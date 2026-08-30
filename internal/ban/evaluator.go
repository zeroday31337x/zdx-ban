package ban

import (
	"context"
	"fmt"
	"zdx-ban/internal/inference"
)

type Evaluator struct {
	Provider           inference.Engine
	Temperature        float64
	MaxTokens          int
	ChallengeMaxTokens int
}

func (e Evaluator) Evaluate(ctx context.Context, goal string, s *State) (Evaluation, inference.Result, error) {
	var out Evaluation
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn only the requested JSON object. evidence and contradictions must be arrays of strings. Scores confidence, uncertainty, evidence_score, consistency_score, contradiction_score, feasibility_score, risk_score, utility_score, and diversity_score must be numbers in [0,1]. hard_constraint_violation must be boolean and hard_constraint_reason must be a string. Do not add fields. Utility is not evidence.", goal, s.Title, s.Hypothesis)
	resp, err := inference.GenerateStructured(ctx, e.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: e.Temperature, MaxTokens: e.MaxTokens, StructuredSchema: evaluationSchema()}, &out)
	return out, resp, err
}
func (e Evaluator) Challenge(ctx context.Context, goal string, s *State) (Challenge, inference.Result, error) {
	var out Challenge
	prompt := fmt.Sprintf("Goal: %s\nCandidate: %s — %s\nReturn strict JSON with skeptic and counterfactual.", goal, s.Title, s.Hypothesis)
	maxTokens := e.ChallengeMaxTokens
	if maxTokens <= 0 {
		maxTokens = e.MaxTokens
	}
	resp, err := inference.GenerateStructured(ctx, e.Provider, inference.Request{Goal: goal, Prompt: prompt, Temperature: e.Temperature, MaxTokens: maxTokens, StructuredSchema: challengeSchema()}, &out)
	return out, resp, err
}

func evaluationSchema() map[string]any {
	text := map[string]any{"type": "string", "maxLength": 512}
	strings := map[string]any{"type": "array", "items": text, "maxItems": 8}
	number := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	properties := map[string]any{
		"evidence": strings, "contradictions": strings,
		"confidence": number, "uncertainty": number, "evidence_score": number,
		"consistency_score": number, "contradiction_score": number, "feasibility_score": number,
		"risk_score": number, "utility_score": number, "diversity_score": number,
		"hard_constraint_violation": map[string]any{"type": "boolean"}, "hard_constraint_reason": text,
	}
	required := []string{"evidence", "contradictions", "confidence", "uncertainty", "evidence_score", "consistency_score", "contradiction_score", "feasibility_score", "risk_score", "utility_score", "diversity_score", "hard_constraint_violation", "hard_constraint_reason"}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func challengeSchema() map[string]any {
	text := map[string]any{"type": "string", "maxLength": 512}
	return map[string]any{"type": "object", "properties": map[string]any{"skeptic": text, "counterfactual": text}, "required": []string{"skeptic", "counterfactual"}, "additionalProperties": false}
}
