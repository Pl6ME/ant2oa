package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/joho/godotenv"
)

// Get admin password from env, default to "admin"
func getAdminPassword() string {
	pw := os.Getenv("ADMIN_PASSWORD")
	if pw == "" {
		pw = "admin"
	}
	return pw
}

// checkAuth verifies basic authentication
func checkAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return false
	}

	expectedPW := getAdminPassword()
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(decoded, []byte(":"+expectedPW)) == 1
}

func configWebHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="ant2oa"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	f, err := webFS.Open("web/index.html")
	if err != nil {
		http.Error(w, "Web UI not found", 404)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("Error serving web UI: %v", err)
	}
}

// ================= Handlers =================

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"service":   "ant2oa",
			"timestamp": time.Now().Format(time.RFC3339),
		}); err != nil {
			log.Printf("Error encoding health response: %v", err)
		}
	}
}

// enhancedHealthHandler checks both service and upstream health
func enhancedHealthHandler(upstreamBase string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check upstream connectivity
		upstreamStatus := "ok"
		upstreamLatency := int64(0)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Quick HEAD request to upstream
		upstreamURL := strings.TrimSuffix(upstreamBase, "/")
		if strings.Contains(upstreamURL, "generativelanguage.googleapis.com") {
			if !strings.HasSuffix(upstreamURL, "/v1beta") {
				upstreamURL += "/v1beta"
			}
		} else {
			if !strings.HasSuffix(upstreamURL, "/v1") {
				upstreamURL += "/v1"
			}
		}
		upstreamURL += "/models"

		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, "HEAD", upstreamURL, nil)
		if err == nil {
			resp, err := HttpClient.Do(req)
			if err != nil {
				upstreamStatus = "error: " + err.Error()
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 500 {
					upstreamStatus = "error: upstream returned " + resp.Status
				}
			}
			upstreamLatency = time.Since(start).Milliseconds()
		} else {
			upstreamStatus = "error: " + err.Error()
		}

		overallStatus := http.StatusOK
		statusText := "ok"
		if upstreamStatus != "ok" {
			overallStatus = http.StatusServiceUnavailable
			statusText = "degraded"
		}

		w.WriteHeader(overallStatus)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status":              statusText,
			"service":             "ant2oa",
			"timestamp":           time.Now().Format(time.RFC3339),
			"upstream_status":     upstreamStatus,
			"upstream_latency_ms": upstreamLatency,
			"rate_limit_enabled":  rateLimitEnabled,
		}); err != nil {
			log.Printf("Error encoding health response: %v", err)
		}
	}
}

func messagesHandler(base, model string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.Header.Get("x-api-key")
		}
		if auth == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			auth = "Bearer " + auth
		}

		// Key Validation
		bearerToken := strings.TrimPrefix(auth, "Bearer ")
		allowed, _ := validateAPIKey(bearerToken)
		if !allowed {
			http.Error(w, "unauthorized: invalid key or rate limit exceeded", http.StatusUnauthorized)
			return
		}

		var req AnthropicMessagesReq
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error reading request", 400)
			return
		}
		if err := json.Unmarshal(b, &req); err != nil {
			log.Printf("JSON Unmarshal Error: %v", err)
			http.Error(w, "bad request: "+err.Error(), 400)
			return
		}

		// 1. Build OpenAI Tools
		var oaTools []OATool
		if len(req.Tools) > 0 {
			oaTools = make([]OATool, len(req.Tools))
			for i, t := range req.Tools {
				oaTools[i] = OATool{
					Type: "function",
					Function: OAFunction{
						Name:        t.Name,
						Description: t.Description,
						Parameters:  t.InputSchema,
					},
				}
			}
		}

		// 2. Build OpenAI Messages
		finalMessages := buildOpenAIMessages(req)

		// Target Model & Routing
		targetModel := model
		if req.Model != "" {
			targetModel = req.Model
		}

		upstreamBase, upstreamAuthKey := getUpstreamForModel(targetModel, base)
		upstreamAuth := auth
		if upstreamAuthKey != "" {
			upstreamAuth = "Bearer " + upstreamAuthKey
		}

		// Extract numeric parameters with type safety
		maxTokens := extractMaxTokens(req.MaxTokens)
		temp := extractTemperature(req.Temperature)
		stopSequences := req.StopSequences
		toolChoice := req.ToolChoice

		// Build final request map
		oaReqMap := map[string]any{
			"model":    targetModel,
			"messages": finalMessages,
			"stream":   req.Stream,
		}
		if maxTokens > 0 {
			oaReqMap["max_tokens"] = maxTokens
		}
		if temp > 0 {
			oaReqMap["temperature"] = temp
		}
		if stopSequences != nil {
			oaReqMap["stop_sequences"] = stopSequences
		}
		if len(oaTools) > 0 {
			oaReqMap["tools"] = oaTools
		}
		if toolChoice != nil {
			oaReqMap["tool_choice"] = toolChoice
		}

		forwardOAMap(w, r, upstreamBase, upstreamAuth, oaReqMap, req.Stream)
	}
}

// buildOpenAIMessages 构建 OpenAI 兼容的消息格式
func buildOpenAIMessages(req AnthropicMessagesReq) []map[string]any {
	messages := make([]map[string]any, 0)

	// Handle System
	if len(req.System) > 0 {
		sysText := parseComplexContent(req.System)
		if sysText != "" {
			messages = append(messages, map[string]any{"role": "system", "content": sysText})
		}
	}

	for _, m := range req.Messages {
		var parts []AnthropicContent
		if err := json.Unmarshal(m.Content, &parts); err != nil {
			var s string
			if err = json.Unmarshal(m.Content, &s); err != nil {
				s = ""
			}
			parts = []AnthropicContent{{Type: "text", Text: s}}
		}

		role := m.Role
		if role == "" {
			role = "user"
		}

		switch role {
		case "system":
			txt := ""
			for _, p := range parts {
				if p.Type == "text" {
					txt += p.Text
				}
			}
			if txt != "" {
				messages = append(messages, map[string]any{"role": "system", "content": txt})
			}
		case "user":
			var oaParts []OAContentPart
			for _, p := range parts {
				switch p.Type {
				case "text":
					if p.Text != "" {
						oaParts = append(oaParts, OAContentPart{Type: "text", Text: p.Text})
					}
				case "image":
					if p.Source != nil && p.Source.Type == "base64" {
						oaParts = append(oaParts, OAContentPart{
							Type: "image_url",
							ImageURL: &OAImageURL{
								URL: "data:" + p.Source.MediaType + ";base64," + p.Source.Data,
							},
						})
					}
				case "tool_result":
					contentStr := ""
					if len(p.Content) > 0 {
						var s string
						if err := json.Unmarshal(p.Content, &s); err == nil {
							contentStr = s
						} else {
							contentStr = string(p.Content)
						}
					}
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": p.ToolUseID,
						"content":      contentStr,
					})
				}
			}

			if len(oaParts) > 0 {
				if len(oaParts) == 1 && oaParts[0].Type == "text" {
					messages = append(messages, map[string]any{"role": "user", "content": oaParts[0].Text})
				} else {
					messages = append(messages, map[string]any{"role": "user", "content": oaParts})
				}
			} else if len(parts) == 0 {
				// Keep empty message if it was empty
				messages = append(messages, map[string]any{"role": "user", "content": ""})
			}

		case "assistant":
			txt := ""
			var toolCalls []map[string]any

			for _, p := range parts {
				switch p.Type {
				case "text":
					txt += p.Text
				case "tool_use":
					toolCalls = append(toolCalls, map[string]any{
						"id":   p.ID,
						"type": "function",
						"function": map[string]string{
							"name":      p.Name,
							"arguments": string(p.Input),
						},
					})
				}
			}
			msg := map[string]any{
				"role":    "assistant",
				"content": txt,
			}
			if len(toolCalls) > 0 {
				msg["tool_calls"] = toolCalls
			}
			messages = append(messages, msg)
		default:
			txt := ""
			for _, p := range parts {
				if p.Type == "text" {
					txt += p.Text
				}
			}
			messages = append(messages, map[string]any{"role": role, "content": txt})
		}
	}

	return messages
}

// extractMaxTokens 安全提取 max_tokens 参数
func extractMaxTokens(maxTokens any) int {
	if maxTokens == nil {
		return 0
	}
	switch v := maxTokens.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if val, err := strconv.Atoi(v); err == nil {
			return val
		}
	}
	return 0
}

// extractTemperature 安全提取温度参数
func extractTemperature(temp any) float64 {
	if temp == nil {
		return 0
	}
	switch v := temp.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			return val
		}
	}
	return 0
}

func completeHandler(base, model string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.Header.Get("x-api-key")
		}
		if auth == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			auth = "Bearer " + auth
		}

		var req AnthropicCompleteReq
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error reading request", 400)
			return
		}
		if err := json.Unmarshal(b, &req); err != nil {
			log.Printf("complete JSON Unmarshal Error: %v", err)
			http.Error(w, "bad request: "+err.Error(), 400)
			return
		}

		// Target Model - support model override from request
		targetModel := model
		if req.Model != "" {
			targetModel = req.Model
		}

		// Simple mapping
		oaReqMap := map[string]any{
			"model": targetModel,
			"messages": []map[string]any{
				{"role": "user", "content": req.Prompt},
			},
			"stream":     req.Stream,
			"max_tokens": req.MaxTokens,
		}
		if req.Temperature > 0 {
			oaReqMap["temperature"] = req.Temperature
		}

		forwardOAMap(w, r, base, auth, oaReqMap, req.Stream)
	}
}

func modelsHandler(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.Header.Get("x-api-key")
		}

		// Handle Gemini's /v1beta path
		modelURL := strings.TrimSuffix(base, "/")
		if strings.Contains(modelURL, "generativelanguage.googleapis.com") {
			if !strings.HasSuffix(modelURL, "/v1beta") {
				modelURL += "/v1beta"
			}
		} else {
			if !strings.HasSuffix(modelURL, "/v1") {
				modelURL += "/v1"
			}
		}
		modelURL += "/models"

		req, err := http.NewRequestWithContext(r.Context(), "GET", modelURL, nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if auth != "" {
			if !strings.HasPrefix(auth, "Bearer ") {
				auth = "Bearer " + auth
			}
			req.Header.Set("Authorization", auth)
		}

		resp, err := HttpClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()

		var oaResp OAModelsResp
		if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
			log.Printf("modelsHandler upstream decode error: %v", err)
		}

		anthResp := AnthropicModelsResp{
			Data:    make([]AnthropicModel, 0),
			HasMore: false,
		}

		for _, m := range oaResp.Data {
			anthResp.Data = append(anthResp.Data, AnthropicModel{
				Type:        "model",
				ID:          m.ID,
				DisplayName: m.ID,
				CreatedAt:   "2024-01-01T00:00:00Z",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthResp)
	}
}

// ================= Web Config API =================

func configHandler(w http.ResponseWriter, r *http.Request) {
	// Check auth
	if !checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="ant2oa"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == "GET" {
		_ = godotenv.Load()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"listenAddr":     os.Getenv("LISTEN_ADDR"),
			"baseUrl":        os.Getenv("OPENAI_BASE_URL"),
			"model":          os.Getenv("OPENAI_MODEL"),
			"rateLimit":      os.Getenv("RATE_LIMIT"),
			"maxRequestSize": os.Getenv("MAX_REQUEST_SIZE"),
		}); err != nil {
			log.Printf("Error encoding config response: %v", err)
		}
		return
	}

	if r.Method == "POST" {
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}

		// Read existing .env or create default
		envPath := ".env"
		envContent := ""
		if _, err := os.Stat(envPath); err == nil {
			b, _ := os.ReadFile(envPath)
			envContent = string(b)
		} else {
			// Create default .env content
			envContent = "# ant2oa Configuration\n# Generated by Web UI\n"
		}

		// Update values - preserve non-config lines (comments, blank lines)
		updates := map[string]string{
			"LISTEN_ADDR":      data["listenAddr"],
			"OPENAI_BASE_URL":  data["baseUrl"],
			"OPENAI_MODEL":     data["model"],
			"RATE_LIMIT":       data["rateLimit"],
			"MAX_REQUEST_SIZE": data["maxRequestSize"],
			"ADMIN_PASSWORD":   data["adminPassword"],
		}

		// Track which keys we've handled
		handledKeys := make(map[string]bool)

		// Process existing lines
		lines := strings.Split(envContent, "\n")
		newLines := make([]string, 0, len(lines)+len(updates))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				newLines = append(newLines, line)
				continue
			}
			// Check if this line is a config key we want to update
			found := false
			for key := range updates {
				if strings.HasPrefix(trimmed, key+"=") {
					handledKeys[key] = true
					// Only update if value is not empty
					if updates[key] != "" {
						newLines = append(newLines, key+"="+updates[key])
					} else if trimmed != "" {
						// Keep original line if new value is empty
						newLines = append(newLines, line)
					}
					found = true
					break
				}
			}
			if !found {
				newLines = append(newLines, line)
			}
		}

		// Add any missing keys
		for key, val := range updates {
			if !handledKeys[key] && val != "" {
				newLines = append(newLines, key+"="+val)
			}
		}

		if err := os.WriteFile(envPath, []byte(strings.Join(newLines, "\n")), 0600); err != nil {
			log.Printf("Error writing .env file: %v", err)
			http.Error(w, "failed to save config", 500)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	http.Error(w, "method not allowed", 405)
}
