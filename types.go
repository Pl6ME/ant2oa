package main

import "encoding/json"

// ================= Common =================

type AnthropicContent struct {
	Type string `json:"type"` // "text", "tool_use", "tool_result"

	// Type: text
	Text string `json:"text,omitempty"`

	// Type: tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Type: tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or []Content
	IsError   bool            `json:"is_error,omitempty"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// ================= Anthropic New (/v1/messages) =================

type AnthropicMessagesReq struct {
	Model    string          `json:"model,omitempty"`
	System   json.RawMessage `json:"system,omitempty"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"` // string or []AnthropicContent
	} `json:"messages"`
	MaxTokens     any             `json:"max_tokens"`
	Temperature   any             `json:"temperature,omitempty"`
	TopP          any             `json:"top_p,omitempty"`
	TopK          any             `json:"top_k,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	StopSequences any             `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool `json:"tools,omitempty"`
	ToolChoice    any             `json:"tool_choice,omitempty"`
}

// ================= Anthropic Old (/v1/complete) =================

type AnthropicCompleteReq struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens_to_sample"`
	Temperature float64 `json:"temperature,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

// ================= OpenAI-compatible =================

type OAChatReq struct {
	Model       string      `json:"model"`
	Messages    []OAMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
	Tools       []OATool    `json:"tools,omitempty"`
	ToolChoice  any         `json:"tool_choice,omitempty"`
}

type OAMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []OAToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"` // For role: tool
	Name       string       `json:"name,omitempty"`
}

type OATool struct {
	Type     string     `json:"type"` // "function"
	Function OAFunction `json:"function"`
}

type OAFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // For Tools definition
	Arguments   string          `json:"arguments,omitempty"`  // For Tool Calls (stringified JSON)
}

type OAToolCall struct {
	Index    int        `json:"index,omitempty"`
	ID       string     `json:"id,omitempty"`
	Type     string     `json:"type"` // "function"
	Function OAFunction `json:"function"`
}

// stream chunk
type OAStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string       `json:"content,omitempty"`
			ReasoningContent string       `json:"reasoning_content,omitempty"`
			Reasoning        string       `json:"reasoning,omitempty"` // 兼容某些厂商
			ToolCalls        []OAToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
}

// Anthropic Models API
type AnthropicModel struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

type AnthropicModelsResp struct {
	Data    []AnthropicModel `json:"data"`
	HasMore bool             `json:"has_more"`
}

// OpenAI Models
type OAModel struct {
	ID string `json:"id"`
}

type OAModelsResp struct {
	Data []OAModel `json:"data"`
}

// OpenAI Non-stream Response
type OAChatResp struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string       `json:"content"`
			ReasoningContent string       `json:"reasoning_content"`
			ToolCalls        []OAToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
