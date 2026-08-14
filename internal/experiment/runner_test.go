package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/model"
)

type pairedMock struct {
	calls int
	eval  int
}

func (p *pairedMock) Health(context.Context) error { return nil }
func (p *pairedMock) ModelInfo(context.Context) (model.Info, error) {
	return model.Info{Provider: "mock", Model: "frozen"}, nil
}
func (p *pairedMock) Generate(_ context.Context, _ model.GenerateRequest) (model.GenerateResponse, error) {
	p.calls++
	return model.GenerateResponse{Text: "7", PromptTokens: 1, CompletionTokens: 1, Latency: time.Millisecond}, nil
}
func (p *pairedMock) GenerateStructured(_ context.Context, r model.GenerateRequest, dst any) (model.GenerateResponse, error) {
	p.calls++
	var v any
	switch {
	case contains(r.Prompt, "exactly 5"):
		v = map[string]any{"branches": []map[string]any{{"title": "tempting", "hypothesis": "8", "reasoning_summary": "surface cue", "assumptions": []string{}}, {"title": "correct", "hypothesis": "7", "reasoning_summary": "calculation", "assumptions": []string{}}, {"title": "alt3", "hypothesis": "6", "reasoning_summary": "other", "assumptions": []string{}}, {"title": "alt4", "hypothesis": "9", "reasoning_summary": "other", "assumptions": []string{}}, {"title": "challenge", "hypothesis": "10", "reasoning_summary": "framing", "assumptions": []string{}}}}
	case contains(r.Prompt, "exactly 2"):
		p.eval++
		v = map[string]any{"branches": []map[string]any{{"title": fmt.Sprintf("child-a-%d", p.eval), "hypothesis": "7", "reasoning_summary": "refined", "assumptions": []string{}}, {"title": fmt.Sprintf("child-b-%d", p.eval), "hypothesis": "11", "reasoning_summary": "refined", "assumptions": []string{}}}}
	case contains(r.Prompt, "skeptic"):
		v = map[string]any{"skeptic": "could be wrong", "counterfactual": "check arithmetic"}
	default:
		p.eval++
		score := .5
		if p.eval == 1 {
			score = .99
		} else if p.eval == 2 {
			score = .9
		}
		v = map[string]any{"evidence": []string{}, "contradictions": []string{}, "Confidence": score, "Uncertainty": .1, "EvidenceScore": score, "ConsistencyScore": score, "ContradictionScore": 0, "FeasibilityScore": score, "RiskScore": 0, "UtilityScore": score, "DiversityScore": .5, "hard_constraint_violation": false, "hard_constraint_reason": ""}
	}
	b, _ := json.Marshal(v)
	if err := json.Unmarshal(b, dst); err != nil {
		return model.GenerateResponse{}, err
	}
	return model.GenerateResponse{PromptTokens: 1, CompletionTokens: 1, Latency: time.Millisecond}, nil
}
func mockRunner(t *testing.T, p *pairedMock) Runner {
	c := validCase()
	c.ForcedRecovery = true
	c.TrapDescription = "wrong 8"
	return Runner{Provider: p, Registry: NewRegistry(), Dataset: Dataset{Version: "v1", Path: "memory", SHA256: "abc", Cases: []Case{c}}, Config: RunConfig{Model: "frozen", Provider: "mock", Temperature: .2, MaxTokens: 32, Timeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true}, OutputRoot: t.TempDir(), ExperimentID: "test-run"}
}
func TestPairedPersistenceRecoveryAndResume(t *testing.T) {
	p := &pairedMock{}
	r := mockRunner(t, p)
	result, err := r.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cases) != 1 || !result.Cases[0].Baseline.Verification.Passed || !result.Cases[0].BAN.Verification.Passed {
		t.Fatalf("bad pair %+v", result.Cases)
	}
	if !result.Cases[0].RecoveryAttempted || !result.Cases[0].RecoverySuccessful {
		t.Fatalf("recovery not captured %+v", result.Cases[0])
	}
	calls := p.calls
	if _, err = r.Run(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if p.calls != calls {
		t.Fatal("resume reran completed pair")
	}
}
func TestCancellationLeavesResumableCheckpoint(t *testing.T) {
	p := &pairedMock{}
	r := mockRunner(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := r.Run(ctx, false)
	if err == nil || len(result.Cases) != 0 {
		t.Fatalf("expected safe interruption: %v", err)
	}
	if _, err = r.Run(context.Background(), true); err != nil {
		t.Fatal("resume failed", err)
	}
}
