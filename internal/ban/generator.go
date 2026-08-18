package ban

import (
	"context"
	"errors"
	"fmt"
	"zdx-ban/internal/inference"
)

type Generator struct {
	Provider    inference.Engine
	Temperature float64
	MaxTokens   int
	MemoryGuide string
}
type proposalEnvelope struct {
	Branches []Proposal `json:"branches"`
}

func (g Generator) Generate(ctx context.Context, goal string, count int, parents []*State) ([]Proposal, inference.Result, error) {
	contextSummary := "none"
	if len(parents) > 0 {
		contextSummary = "Refine retained approaches:\n"
		for _, p := range parents {
			contextSummary += fmt.Sprintf("- %s: %s\n", p.Title, p.Hypothesis)
		}
	}
	prompt := fmt.Sprintf("Goal: %s\n%s\n%s\nReturn strict JSON {\"branches\":[...]}, exactly %d semantic approaches. Each branch requires title, hypothesis, reasoning_summary, assumptions. The hypothesis must be the candidate final answer in the exact format the goal requests; place explanation only in reasoning_summary. Use distinct causal mechanisms, avoid paraphrases, and include one framing-challenging alternative when appropriate.", goal, g.MemoryGuide, contextSummary, count)
	var out proposalEnvelope
	resp, err := inference.GenerateStructured(ctx, g.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: g.Temperature, MaxTokens: g.MaxTokens}, &out)
	if err != nil {
		return nil, resp, err
	}
	if len(out.Branches) != count {
		return nil, resp, fmt.Errorf("expected %d branches, got %d", count, len(out.Branches))
	}
	for _, p := range out.Branches {
		if p.Title == "" || p.Hypothesis == "" {
			return nil, resp, errors.New("empty proposal fields")
		}
	}
	return out.Branches, resp, nil
}
