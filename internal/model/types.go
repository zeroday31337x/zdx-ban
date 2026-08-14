package model

import "time"

type GenerateRequest struct {
	Prompt      string  `json:"prompt"`
	System      string  `json:"system,omitempty"`
	Temperature float64 `json:"temperature"`
	Seed        *int    `json:"seed,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

type GenerateResponse struct {
	Text             string        `json:"text"`
	PromptTokens     int           `json:"prompt_tokens,omitempty"`
	CompletionTokens int           `json:"completion_tokens,omitempty"`
	Latency          time.Duration `json:"latency"`
}

type Info struct{ Provider, Model, Version string }
