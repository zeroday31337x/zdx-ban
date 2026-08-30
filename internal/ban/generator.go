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
	Branches []proposalOutput `json:"branches"`
}

type proposalOutput struct {
	Title            string   `json:"title"`
	Answer           string   `json:"answer"`
	ReasoningSummary string   `json:"reasoning_summary"`
	Assumptions      []string `json:"assumptions"`
}

func (g Generator) Generate(ctx context.Context, goal string, count int, parents []*State) ([]Proposal, inference.Result, error) {
	contextSummary := "none"
	if len(parents) > 0 {
		contextSummary = "Existing paths that must not be duplicated; generate different method names, reasoning summaries, assumptions, and literal answers:\n"
		for _, p := range parents {
			status := "existing"
			if len(p.VerificationResults) > 0 && !p.VerificationResults[len(p.VerificationResults)-1].Passed {
				status = "rejected by the current deterministic verifier; recompute independently and do not repeat this answer"
			}
			contextSummary += fmt.Sprintf("- %s: %s (%s)\n", p.Title, p.Hypothesis, status)
		}
	}
	prompt := fmt.Sprintf("Goal: %s\n%s\n%s\nReturn only JSON {\"branches\":[...]}, with exactly %d semantic approaches. Each branch has exactly title (string), answer (string), reasoning_summary (string), and assumptions (array of strings). Do not add fields. Keep each title under 8 words, each reasoning_summary under 30 words, and assumptions to at most 3 short strings. The answer is the literal candidate response submitted to the goal verifier. Obey output-only instructions exactly: for 'return only the name', use a bare name such as 'Agent-01', never a sentence such as 'Agent-01 has the key'; for 'return only the number', use only digits; for JSON, use only the requested JSON value; for a canonical defect phrase, use only the conventional lowercase 1-to-4-word label with no sentence, punctuation, or added words such as 'error' or 'defect'. Distinct reasoning approaches may produce the same answer. Put every explanation in reasoning_summary, never in answer. Use distinct causal mechanisms, avoid paraphrases, and express any framing challenge inside an ordinary branch rather than as another field.", goal, g.MemoryGuide, contextSummary, count)
	var out proposalEnvelope
	resp, err := inference.GenerateStructured(ctx, g.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: g.Temperature, MaxTokens: g.MaxTokens, StructuredSchema: proposalSchema(count)}, &out)
	if err != nil {
		return nil, resp, err
	}
	if len(out.Branches) != count {
		return nil, resp, fmt.Errorf("expected %d branches, got %d", count, len(out.Branches))
	}
	proposals := make([]Proposal, 0, len(out.Branches))
	for _, p := range out.Branches {
		if p.Title == "" || p.Answer == "" {
			return nil, resp, errors.New("empty proposal fields")
		}
		proposals = append(proposals, Proposal{Title: p.Title, Hypothesis: p.Answer, ReasoningSummary: p.ReasoningSummary, Assumptions: p.Assumptions})
	}
	return proposals, resp, nil
}

func proposalSchema(count int) map[string]any {
	text := func(maxLength int) map[string]any { return map[string]any{"type": "string", "maxLength": maxLength} }
	branch := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":             text(96),
			"answer":            text(512),
			"reasoning_summary": text(320),
			"assumptions":       map[string]any{"type": "array", "items": text(160), "maxItems": 3},
		},
		"required":             []string{"title", "answer", "reasoning_summary", "assumptions"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"branches": map[string]any{"type": "array", "items": branch, "minItems": count, "maxItems": count},
		},
		"required":             []string{"branches"},
		"additionalProperties": false,
	}
}
