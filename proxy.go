package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ================= Core Forward + Streaming FSM =================

// ================= Core Forward + Streaming FSM =================

var (
	// Connection Pooling & Timeout
	HttpClient = &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Rate Limiting
	limiter          chan struct{}
	rateLimitEnabled bool
)

func forwardOAMap(w http.ResponseWriter, r *http.Request, base, auth string, oaReqMap map[string]any, stream bool) {
	// Rate Limit Check
	if rateLimitEnabled && limiter != nil {
		select {
		case <-limiter:
			// Go ahead
		case <-r.Context().Done():
			http.Error(w, "client disconnected waiting for rate limit", 499)
			return
		}
	}

	apiURL := strings.TrimSuffix(base, "/")
	// Gemini API uses /v1beta instead of /v1
	if strings.Contains(apiURL, "generativelanguage.googleapis.com") {
		if !strings.HasSuffix(apiURL, "/v1beta") {
			apiURL += "/v1beta"
		}
	} else {
		if !strings.HasSuffix(apiURL, "/v1") {
			apiURL += "/v1"
		}
	}
	apiURL += "/chat/completions"

	buf, err := json.Marshal(oaReqMap)
	if err != nil {
		log.Printf("Request Marshal Error: %v", err)
		http.Error(w, "error processing request", 500)
		return
	}

	var resp *http.Response
	maxRetries := 3

	for i := 0; i <= maxRetries; i++ {
		or, err := http.NewRequestWithContext(r.Context(), "POST", apiURL, bytes.NewReader(buf))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		or.Header.Set("Authorization", auth)
		or.Header.Set("Content-Type", "application/json")

		resp, err = HttpClient.Do(or)
		if err != nil {
			log.Printf("Upstream Request Error: %v", err)
			http.Error(w, err.Error(), 502)
			return
		}

		if resp.StatusCode != 429 {
			break
		}

		// Close body before retrying
		resp.Body.Close()

		if i < maxRetries {
			waitTime := time.Duration(1<<i) * time.Second
			log.Printf("Upstream 429 Too Many Requests. Retrying in %v...", waitTime)
			select {
			case <-time.After(waitTime):
				continue
			case <-r.Context().Done():
				http.Error(w, "request canceled during retry", 499)
				return
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		rb, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading error response body: %v", err)
		}
		if ctype := resp.Header.Get("Content-Type"); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(rb)
		return
	}

	if !stream {
		w.Header().Set("Content-Type", "application/json")
		var oaResp OAChatResp
		if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
			http.Error(w, "upstream decode error", 502)
			return
		}
		if len(oaResp.Choices) == 0 {
			http.Error(w, "empty choices", 502)
			return
		}

		choice := oaResp.Choices[0]
		blocks := make([]map[string]any, 0)

		// 1. Thinking
		if choice.Message.ReasoningContent != "" {
			blocks = append(blocks, map[string]any{
				"type":     "thinking",
				"thinking": choice.Message.ReasoningContent,
			})
		}

		// 2. Text Content (Parse <think>)
		rawContent := choice.Message.Content
		parsedBlocks := parseContentWithThinkTags(rawContent) // Helper below
		blocks = append(blocks, parsedBlocks...)

		// 3. Tool Calls
		for _, tc := range choice.Message.ToolCalls {
			args := "{}"
			if len(tc.Function.Parameters) > 0 {
				args = string(tc.Function.Parameters)
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": json.RawMessage(args),
			})
		}

		if len(blocks) == 0 {
			blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		}

		stopReason := "end_turn"
		if choice.FinishReason == "tool_calls" {
			stopReason = "tool_use"
		}

		anthResp := map[string]any{
			"id":            "msg_" + oaResp.ID,
			"type":          "message",
			"role":          "assistant",
			"model":         oaResp.Model,
			"content":       blocks,
			"stop_reason":   stopReason,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  oaResp.Usage.PromptTokens,
				"output_tokens": oaResp.Usage.CompletionTokens,
			},
		}
		json.NewEncoder(w).Encode(anthResp)
		return
	}

	// STREAMING
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	startedMessage := false

	// FSM State
	currentBlockType := "" // "thinking", "text", "tool_use"
	currentBlockIdx := -1

	// Buffers
	contentBuffer := "" // for text <think> parsing

	// Tool State
	currentToolIndex := -1
	// currentToolID := ""
	// currentToolName := ""

	emitDelta := func(text string) {
		if text == "" {
			return
		}
		if currentBlockType == "" {
			currentBlockIdx++
			currentBlockType = "text"
			w.Write([]byte(fmt.Sprintf("event: content_block_start\ndata: {\"type\": \"content_block_start\", \"index\": %d, \"content_block\": {\"type\": \"text\", \"text\": \"\"}}\n\n", currentBlockIdx)))
		}

		switch currentBlockType {
		case "thinking":
			evt, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": currentBlockIdx,
				"delta": map[string]string{"type": "thinking_delta", "thinking": text},
			})
			w.Write([]byte("event: content_block_delta\ndata: " + string(evt) + "\n\n"))
		case "text":
			evt, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": currentBlockIdx,
				"delta": map[string]string{"type": "text_delta", "text": text},
			})
			w.Write([]byte("event: content_block_delta\ndata: " + string(evt) + "\n\n"))
		}
	}

	closeBlock := func() {
		if currentBlockIdx >= 0 {
			w.Write([]byte(fmt.Sprintf("event: content_block_stop\ndata: {\"type\": \"content_block_stop\", \"index\": %d}\n\n", currentBlockIdx)))
		}
		currentBlockType = ""
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			closeBlock()
			w.Write([]byte("event: message_delta\ndata: {\"type\": \"message_delta\", \"delta\": {\"stop_reason\": \"end_turn\", \"stop_sequence\": null}, \"usage\": {\"output_tokens\": 0}}\n\n"))
			w.Write([]byte("event: message_stop\ndata: {\"type\": \"message_stop\"}\n\n"))
			flusher.Flush()
			return
		}

		var chunk OAStreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}

		if !startedMessage {
			w.Write([]byte("event: message_start\ndata: {\"type\": \"message_start\", \"message\": {\"id\": \"msg_proxy\", \"type\": \"message\", \"role\": \"assistant\", \"content\": [], \"model\": \"proxy\", \"stop_reason\": null, \"stop_sequence\": null, \"usage\": {\"input_tokens\": 0, \"output_tokens\": 0}}}\n\n"))
			startedMessage = true
		}

		delta := chunk.Choices[0].Delta

		// 1. Handle Reasoning (deepseek style or reasoning_content)
		rContent := delta.ReasoningContent
		if rContent == "" {
			rContent = delta.Reasoning
		}
		if rContent != "" {
			if currentBlockType != "thinking" {
				closeBlock()
				currentBlockIdx++
				currentBlockType = "thinking"
				w.Write([]byte(fmt.Sprintf("event: content_block_start\ndata: {\"type\": \"content_block_start\", \"index\": %d, \"content_block\": {\"type\": \"thinking\", \"thinking\": \"\"}}\n\n", currentBlockIdx)))
			}
			emitDelta(rContent)
		}

		// 2. Handle Text (parsed for <think>)
		if delta.Content != "" {
			// If we were in tool mode, close it
			if currentBlockType == "tool_use" {
				closeBlock()
			}

			contentBuffer += delta.Content
			// Parse logic similar to before
			for {
				startTag := strings.Index(contentBuffer, "<think>")
				endTag := strings.Index(contentBuffer, "</think>")

				if startTag == -1 && endTag == -1 {
					// Safe partial check
					cutoff := len(contentBuffer)
					if len(contentBuffer) > 20 {
						lastOpen := strings.LastIndexByte(contentBuffer[len(contentBuffer)-20:], '<')
						if lastOpen != -1 {
							cutoff = (len(contentBuffer) - 20) + lastOpen
						}
					} else {
						if strings.Contains(contentBuffer, "<") {
							cutoff = strings.LastIndexByte(contentBuffer, '<')
						}
					}

					safe := contentBuffer[:cutoff]
					contentBuffer = contentBuffer[cutoff:]

					// Determine if we are in thinking or text based on history
					// If strictly <think> started it, we are in thinking.
					// Implementation quirk: currentBlockType tracks this.

					if safe != "" {
						emitDelta(safe)
					}
					break
				}

				// Found tag
				tagIdx := -1
				isStart := false
				if startTag != -1 && (endTag == -1 || startTag < endTag) {
					tagIdx = startTag
					isStart = true
				} else {
					tagIdx = endTag
					isStart = false
				}

				pre := contentBuffer[:tagIdx]
				if pre != "" {
					emitDelta(pre)
				}

				if isStart {
					// <think>
					if currentBlockType == "text" {
						closeBlock()
					}
					if currentBlockType != "thinking" {
						currentBlockIdx++
						currentBlockType = "thinking"
						w.Write([]byte(fmt.Sprintf("event: content_block_start\ndata: {\"type\": \"content_block_start\", \"index\": %d, \"content_block\": {\"type\": \"thinking\", \"thinking\": \"\"}}\n\n", currentBlockIdx)))
					}
					contentBuffer = contentBuffer[tagIdx+7:]
				} else {
					// </think>
					if currentBlockType == "thinking" {
						closeBlock()
					}
					if currentBlockType != "text" {
						// Don't auto open text unless content follows?
						// Actually emitDelta will open text if needed.
					}
					contentBuffer = contentBuffer[tagIdx+8:]
				}
			}
		}

		// 3. Handle Tool Calls
		if len(delta.ToolCalls) > 0 {
			if currentBlockType == "text" || currentBlockType == "thinking" {
				closeBlock()
			}

			tc := delta.ToolCalls[0]

			// Check if new tool call started (index changed or ID present)
			if tc.Index != currentToolIndex || tc.ID != "" {
				if currentBlockType == "tool_use" && tc.Index != currentToolIndex {
					closeBlock()
				}

				if tc.ID != "" {
					currentToolIndex = tc.Index
					// Start new tool_use block
					currentBlockIdx++
					currentBlockType = "tool_use"

					startJson, _ := json.Marshal(map[string]any{
						"type":  "content_block_start",
						"index": currentBlockIdx,
						"content_block": map[string]string{
							"type": "tool_use",
							"id":   tc.ID,
							"name": tc.Function.Name,
						},
					})
					w.Write([]byte("event: content_block_start\ndata: " + string(startJson) + "\n\n"))
				}
			}

			// Streaming arguments
			if tc.Function.Arguments != "" {
				// Ensure we are in tool_use block (sometimes ID comes first, args later)
				if currentBlockType != "tool_use" {
					// Should ideally not happen if ID comes first.
					// If it happens (packet split weirdly), we rely on previous state?
					// Assume ID always sent at start of tool index.
				}

				deltaJson, _ := json.Marshal(map[string]any{
					"type":  "content_block_delta",
					"index": currentBlockIdx,
					"delta": map[string]string{
						"type":         "input_json_delta",
						"partial_json": tc.Function.Arguments,
					},
				})
				w.Write([]byte("event: content_block_delta\ndata: " + string(deltaJson) + "\n\n"))
			}
		}

		flusher.Flush()
	}
}
