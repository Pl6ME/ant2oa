package main

import (
	"encoding/json"
	"strings"
)

// ================= Helpers =================

func parseComplexContent(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []AnthropicContent
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}
	return ""
}

func parseContentWithThinkTags(raw string) []map[string]any {
	blocks := make([]map[string]any, 0)

	// Just a simplified non-stream parser
	parts := strings.Split(raw, "<think>")
	for i, part := range parts {
		if i == 0 {
			if part != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": part})
			}
			continue
		}

		// Substring after <think>
		subs := strings.Split(part, "</think>")
		if len(subs) == 2 {
			// subs[0] is thinking, subs[1] is text
			blocks = append(blocks, map[string]any{"type": "thinking", "thinking": subs[0]})
			if subs[1] != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": subs[1]})
			}
		} else {
			// Unclosed or other weirdness, just treat as text or thinking?
			// Treat entire part as thinking if no closing tag found?
			blocks = append(blocks, map[string]any{"type": "thinking", "thinking": part})
		}
	}
	return blocks
}
