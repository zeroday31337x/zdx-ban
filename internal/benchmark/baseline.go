package benchmark

import (
	"context"
	"time"
	"zdx-ban/internal/model"
)

type BaselineResult struct {
	Answer  string        `json:"answer"`
	Latency time.Duration `json:"latency"`
	Tokens  int           `json:"tokens"`
}

func Baseline(ctx context.Context, p model.Provider, prompt string, temp float64, max int) (BaselineResult, error) {
	start := time.Now()
	r, err := p.Generate(ctx, model.GenerateRequest{Prompt: prompt, Temperature: temp, MaxTokens: max})
	return BaselineResult{r.Text, time.Since(start), r.PromptTokens + r.CompletionTokens}, err
}
