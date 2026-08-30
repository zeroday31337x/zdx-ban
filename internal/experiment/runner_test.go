package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/inference"
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
		v = map[string]any{"branches": []map[string]any{{"title": "tempting", "answer": "8", "reasoning_summary": "surface cue", "assumptions": []string{}}, {"title": "correct", "answer": "7", "reasoning_summary": "calculation", "assumptions": []string{}}, {"title": "alt3", "answer": "6", "reasoning_summary": "other", "assumptions": []string{}}, {"title": "alt4", "answer": "9", "reasoning_summary": "other", "assumptions": []string{}}, {"title": "challenge", "answer": "10", "reasoning_summary": "framing", "assumptions": []string{}}}}
	case contains(r.Prompt, "exactly 2"):
		p.eval++
		v = map[string]any{"branches": []map[string]any{{"title": fmt.Sprintf("child-a-%d", p.eval), "answer": "7", "reasoning_summary": "refined", "assumptions": []string{}}, {"title": fmt.Sprintf("child-b-%d", p.eval), "answer": "11", "reasoning_summary": "refined", "assumptions": []string{}}}}
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

type failureProvider struct {
	err   error
	delay time.Duration
}

func (p failureProvider) Generate(ctx context.Context, r model.GenerateRequest) (model.GenerateResponse, error) {
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return model.GenerateResponse{}, ctx.Err()
		}
	}
	return model.GenerateResponse{}, p.err
}
func (p failureProvider) GenerateStructured(ctx context.Context, r model.GenerateRequest, d any) (model.GenerateResponse, error) {
	return p.Generate(ctx, r)
}

func TestProviderFailureClassificationAndAccounting(t *testing.T) {
	c := validCase()
	tests := []struct {
		name     string
		provider failureProvider
		timeout  time.Duration
		code     inference.FailureCode
	}{
		{"connection", failureProvider{err: inference.NewFailure(inference.ProviderConnectionError, errors.New("dial failed"))}, time.Second, inference.ProviderConnectionError},
		{"malformed", failureProvider{err: inference.NewFailure(inference.ModelOutputMalformed, errors.New("bad JSON"))}, time.Second, inference.ModelOutputMalformed},
		{"timeout", failureProvider{delay: 50 * time.Millisecond}, 5 * time.Millisecond, inference.ProviderTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Runner{Provider: tt.provider, Registry: NewRegistry(), Config: RunConfig{Model: "m", Provider: "fake", MaxTokens: 8, Timeout: time.Second, InferenceTimeout: tt.timeout, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true}}
			row := r.runPair(context.Background(), c, 1)
			if row.BAN.Provider.Attempted != 1 || row.BAN.Provider.Failed != 1 {
				t.Fatalf("accounting=%+v", row.BAN.Provider)
			}
			if row.BAN.FailureCategory != string(tt.code) {
				t.Fatalf("category=%s want=%s", row.BAN.FailureCategory, tt.code)
			}
			if tt.code == inference.ProviderTimeout && row.BAN.Provider.TimedOut != 1 {
				t.Fatalf("timeout accounting=%+v", row.BAN.Provider)
			}
		})
	}
}

func TestExecutionErrorIsNotMalformedOrProviderFailure(t *testing.T) {
	err := errors.New("insufficient distinct branches")
	v := errorVerification(context.Background(), err)
	if v.Outcome != ExecutionFailure || classifyBAN(err, &ban.ExecutionTrace{}) != string(inference.ExecutionError) {
		t.Fatalf("execution error misclassified: outcome=%s category=%s", v.Outcome, classifyBAN(err, &ban.ExecutionTrace{}))
	}
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
