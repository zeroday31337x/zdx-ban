package ban

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"zdx-ban/internal/model"
)

type scriptedProvider struct{ eval int }

func (p *scriptedProvider) Health(context.Context) error { return nil }
func (p *scriptedProvider) ModelInfo(context.Context) (model.Info, error) {
	return model.Info{Provider: "mock", Model: "frozen"}, nil
}
func (p *scriptedProvider) Generate(context.Context, model.GenerateRequest) (model.GenerateResponse, error) {
	return model.GenerateResponse{Text: "answer from recovered branch", CompletionTokens: 5}, nil
}
func (p *scriptedProvider) GenerateStructured(_ context.Context, r model.GenerateRequest, dst any) (model.GenerateResponse, error) {
	var v any
	switch {
	case strings.Contains(r.Prompt, "exactly 5"):
		v = map[string]any{"branches": []map[string]any{{"title": "Obvious", "hypothesis": "wrong obvious cause", "reasoning_summary": "looks likely", "assumptions": []string{}}, {"title": "Minority", "hypothesis": "correct alternative", "reasoning_summary": "testable", "assumptions": []string{}}, {"title": "Third", "hypothesis": "third cause", "reasoning_summary": "third", "assumptions": []string{}}, {"title": "Fourth", "hypothesis": "fourth cause", "reasoning_summary": "fourth", "assumptions": []string{}}, {"title": "Framing", "hypothesis": "measurement error", "reasoning_summary": "challenge framing", "assumptions": []string{}}}}
	case strings.Contains(r.Prompt, "exactly 2"):
		v = map[string]any{"branches": []map[string]any{{"title": "Shared A", "hypothesis": "convergent refinement A", "reasoning_summary": "refined", "assumptions": []string{}}, {"title": "Shared B", "hypothesis": "convergent refinement B", "reasoning_summary": "refined", "assumptions": []string{}}}}
	case strings.Contains(r.Prompt, "skeptic"):
		v = map[string]any{"skeptic": "hidden assumption", "counterfactual": "new contrary evidence"}
	default:
		p.eval++
		score := .6
		if p.eval == 1 {
			score = .99
		} else if p.eval == 2 {
			score = .9
		}
		v = map[string]any{"evidence": []string{"e"}, "contradictions": []string{}, "Confidence": score, "Uncertainty": .1, "EvidenceScore": score, "ConsistencyScore": score, "ContradictionScore": 0, "FeasibilityScore": score, "RiskScore": .1, "UtilityScore": score, "DiversityScore": .8, "hard_constraint_violation": false, "hard_constraint_reason": ""}
	}
	b, _ := json.Marshal(v)
	if err := json.Unmarshal(b, dst); err != nil {
		return model.GenerateResponse{}, err
	}
	return model.GenerateResponse{CompletionTokens: 2, Latency: time.Millisecond}, nil
}

type failObvious struct{}

func (failObvious) Name() string { return "forced" }
func (failObvious) Verify(_ context.Context, _ string, s *State) VerificationResult {
	pass := s.ID != "b01"
	detail := "verified"
	if !pass {
		detail = "obvious branch disproved"
	}
	return VerificationResult{Verifier: "forced", Passed: pass, Details: detail, At: time.Now().UTC()}
}
func TestEngineRecoversAndConverges(t *testing.T) {
	p := &scriptedProvider{}
	e := NewEngine(p, DefaultConfig())
	e.Verifier = failObvious{}
	e.TraceDir = t.TempDir()
	res, tr, err := e.Run(context.Background(), "forced recovery problem")
	if err != nil {
		t.Fatal(err)
	}
	if tr.InitialTopBranch != "b01" || tr.SelectedBranch == "b01" || !tr.RecoveredFromWrongBranch {
		t.Fatalf("recovery: initial=%s final=%s recovered=%v", tr.InitialTopBranch, tr.SelectedBranch, tr.RecoveredFromWrongBranch)
	}
	if res.Selected.Status != Selected || tr.ReasonForSwitch == "" {
		t.Fatal("selection/reason missing")
	}
	if len(tr.Nodes) != 7 || tr.Metrics.ModelCalls == 0 {
		t.Fatalf("trace nodes=%d calls=%d", len(tr.Nodes), tr.Metrics.ModelCalls)
	}
	parents := 0
	for _, n := range tr.Nodes {
		if len(n.ParentIDs) == 2 {
			parents++
		}
	}
	if parents != 2 {
		t.Fatalf("expected convergent nodes, got %d", parents)
	}
	_ = fmt.Sprintf("%v", tr)
}
