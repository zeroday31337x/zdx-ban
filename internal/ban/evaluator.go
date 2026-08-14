package ban

import (
	"context"
	"fmt"
	"zdx-ban/internal/model"
)

type Evaluator struct {
	Provider    model.Provider
	Temperature float64
	MaxTokens   int
}

func (e Evaluator) Evaluate(ctx context.Context, goal string, s *State) (Evaluation, model.GenerateResponse, error) {
	var out Evaluation
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn strict JSON with evidence, contradictions, scores confidence, uncertainty, evidenceScore, consistencyScore, contradictionScore, feasibilityScore, riskScore, utilityScore, diversityScore in [0,1], hardConstraintViolation and hardConstraintReason. Utility is not evidence.", goal, s.Title, s.Hypothesis)
	resp, err := e.Provider.GenerateStructured(ctx, model.GenerateRequest{Prompt: prompt, Temperature: e.Temperature, MaxTokens: e.MaxTokens}, &out)
	return out, resp, err
}
func (e Evaluator) Challenge(ctx context.Context, goal string, s *State) (Challenge, model.GenerateResponse, error) {
	var out Challenge
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn strict JSON with skeptic and counterfactual.", goal, s.Title, s.Hypothesis)
	resp, err := e.Provider.GenerateStructured(ctx, model.GenerateRequest{Prompt: prompt, Temperature: e.Temperature, MaxTokens: e.MaxTokens}, &out)
	return out, resp, err
}
