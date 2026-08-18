package ban

import (
	"context"
	"fmt"
	"zdx-ban/internal/inference"
)

type Evaluator struct {
	Provider    inference.Engine
	Temperature float64
	MaxTokens   int
}

func (e Evaluator) Evaluate(ctx context.Context, goal string, s *State) (Evaluation, inference.Result, error) {
	var out Evaluation
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn strict JSON with evidence, contradictions, scores confidence, uncertainty, evidenceScore, consistencyScore, contradictionScore, feasibilityScore, riskScore, utilityScore, diversityScore in [0,1], hardConstraintViolation and hardConstraintReason. Utility is not evidence.", goal, s.Title, s.Hypothesis)
	resp, err := inference.GenerateStructured(ctx, e.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: e.Temperature, MaxTokens: e.MaxTokens}, &out)
	return out, resp, err
}
func (e Evaluator) Challenge(ctx context.Context, goal string, s *State) (Challenge, inference.Result, error) {
	var out Challenge
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn strict JSON with skeptic and counterfactual.", goal, s.Title, s.Hypothesis)
	resp, err := inference.GenerateStructured(ctx, e.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: e.Temperature, MaxTokens: e.MaxTokens}, &out)
	return out, resp, err
}
