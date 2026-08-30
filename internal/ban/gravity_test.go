package ban

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
	"zdx-ban/internal/model"
)

func TestGravityRoutesRelevantBranchWithoutVerifyingIt(t *testing.T) {
	relevant := NewState("relevant", Proposal{Title: "Grouped multiplication", Hypothesis: "45", ReasoningSummary: "multiply grouped terms before subtraction"}, 0)
	unrelated := NewState("unrelated", Proposal{Title: "Guess", Hypothesis: "45", ReasoningSummary: "select an arbitrary value"}, 0)
	evaluation := Evaluation{Confidence: .5, EvidenceScore: .5, ConsistencyScore: .5, FeasibilityScore: .5, UtilityScore: .5, DiversityScore: .5}
	ApplyEvaluation(relevant, evaluation)
	ApplyEvaluation(unrelated, evaluation)
	well := GravityWell{ID: "well-arithmetic", Strength: .8, SupportingEvidence: 3, Keywords: []string{"grouped", "multiply", "multiplication", "subtraction", "terms"}}
	if !ApplyGravity(relevant, []GravityWell{well}, .2) {
		t.Fatal("relevant branch did not enter gravity well")
	}
	ApplyGravity(unrelated, []GravityWell{well}, .2)
	if relevant.AggregateScore <= unrelated.AggregateScore || relevant.GravityWellID != well.ID || relevant.Status == Verified {
		t.Fatalf("gravity routing invalid: relevant=%+v unrelated=%+v", relevant, unrelated)
	}
}

func TestEffectiveGravityStrengthDecaysAndStaysBounded(t *testing.T) {
	now := time.Unix(10_000, 0).UTC()
	well := GravityWell{Strength: 2, DecayHalfLife: time.Hour, LastUsedAt: now.Add(-time.Hour)}
	if got := well.EffectiveStrength(now); math.Abs(got-.5) > 1e-9 {
		t.Fatalf("one half-life strength = %f, want .5", got)
	}
	well = GravityWell{Strength: .8, ApplicationCount: 4, OutcomeCount: 4, SuccessRate: 0}
	if got := well.EffectiveStrength(now); got <= 0 || got >= .8*.4 {
		t.Fatalf("failed well was not softly weakened: %f", got)
	}
	well = GravityWell{Strength: -1, DecayHalfLife: time.Hour, LastUsedAt: now.Add(time.Hour)}
	if got := well.EffectiveStrength(now); got != 0 {
		t.Fatalf("effective strength escaped lower bound: %f", got)
	}
}

func TestApplyingGravityRecordsUsageImmediately(t *testing.T) {
	now := time.Unix(15_000, 0).UTC()
	state := NewState("routed", Proposal{Title: "verified method", ReasoningSummary: "apply method"}, 0)
	ApplyEvaluation(state, Evaluation{Confidence: .5, EvidenceScore: .5, ConsistencyScore: .5, FeasibilityScore: .5, UtilityScore: .5})
	wells := []GravityWell{{ID: "well-a", Strength: .8, Keywords: []string{"verified", "method"}}}
	if !applyGravityAt(state, wells, .2, now) {
		t.Fatal("gravity was not applied")
	}
	if wells[0].ApplicationCount != 1 || wells[0].OutcomeCount != 0 || !wells[0].LastUsedAt.Equal(now) {
		t.Fatalf("application usage not recorded: %+v", wells[0])
	}
}

func TestGravityOutcomeUpdatesEMAOnlyForResolvedApplications(t *testing.T) {
	now := time.Unix(20_000, 0).UTC()
	wells := []GravityWell{{ID: "well-a", Strength: .8, ApplicationCount: 1, LastUsedAt: now}}
	state := &State{GravityWellID: "well-a", Status: Evaluated}
	if UpdateGravityOutcome(wells, state, now) {
		t.Fatal("inconclusive state updated well")
	}
	state.Status = Failed
	if !UpdateGravityOutcome(wells, state, now) || wells[0].ApplicationCount != 1 || wells[0].OutcomeCount != 1 || wells[0].SuccessRate != 0 || !wells[0].LastUsedAt.Equal(now) {
		t.Fatalf("failed outcome not recorded: %+v", wells[0])
	}
	state.Status = Verified
	if !UpdateGravityOutcome(wells, state, now.Add(time.Minute)) {
		t.Fatal("verified outcome not recorded")
	}
	if wells[0].ApplicationCount != 1 || wells[0].OutcomeCount != 2 || math.Abs(wells[0].SuccessRate-.2) > 1e-9 {
		t.Fatalf("unexpected EMA update: %+v", wells[0])
	}
}

func TestContradictedGravityWellRepelsMatchingBranch(t *testing.T) {
	state := NewState("stale", Proposal{Title: "Left to right", Hypothesis: "40", ReasoningSummary: "always subtract before multiplication"}, 0)
	ApplyEvaluation(state, Evaluation{Confidence: .8, EvidenceScore: .8, ConsistencyScore: .8, FeasibilityScore: .8, UtilityScore: .8})
	before := state.AggregateScore
	well := GravityWell{ID: "well-stale", Strength: .1, Repulsion: .9, Keywords: []string{"always", "subtract", "multiplication"}}
	if !ApplyGravity(state, []GravityWell{well}, .2) || state.AggregateScore >= before || state.GravityRepulsion <= state.InformationGravity {
		t.Fatalf("repulsion was not applied: before=%f state=%+v", before, state)
	}
}

type gravityRecoveryProvider struct{}

func (gravityRecoveryProvider) Health(context.Context) error { return nil }
func (gravityRecoveryProvider) ModelInfo(context.Context) (model.Info, error) {
	return model.Info{Provider: "mock", Model: "gravity"}, nil
}
func (gravityRecoveryProvider) Generate(context.Context, model.GenerateRequest) (model.GenerateResponse, error) {
	return model.GenerateResponse{Text: "correct"}, nil
}
func (gravityRecoveryProvider) GenerateStructured(_ context.Context, request model.GenerateRequest, destination any) (model.GenerateResponse, error) {
	var value any
	switch {
	case strings.Contains(request.Prompt, "rejected by the current deterministic verifier"):
		value = map[string]any{"branches": []map[string]any{{"title": "verified route", "answer": "correct", "reasoning_summary": "apply verified method independently", "assumptions": []string{}}, {"title": "alternate route", "answer": "still-wrong", "reasoning_summary": "check another method", "assumptions": []string{}}}}
	case strings.Contains(request.Prompt, "exactly 5"):
		value = map[string]any{"branches": []map[string]any{{"title": "wrong one", "answer": "wrong-1", "reasoning_summary": "bad method one", "assumptions": []string{}}, {"title": "wrong two", "answer": "wrong-2", "reasoning_summary": "bad method two", "assumptions": []string{}}, {"title": "wrong three", "answer": "wrong-3", "reasoning_summary": "bad method three", "assumptions": []string{}}, {"title": "wrong four", "answer": "wrong-4", "reasoning_summary": "bad method four", "assumptions": []string{}}, {"title": "wrong five", "answer": "wrong-5", "reasoning_summary": "bad method five", "assumptions": []string{}}}}
	case strings.Contains(request.Prompt, "exactly 2"):
		value = map[string]any{"branches": []map[string]any{{"title": "wrong child a", "answer": "wrong-a", "reasoning_summary": "bad refinement a", "assumptions": []string{}}, {"title": "wrong child b", "answer": "wrong-b", "reasoning_summary": "bad refinement b", "assumptions": []string{}}}}
	case strings.Contains(request.Prompt, "skeptic"):
		value = map[string]any{"skeptic": "candidate may be wrong", "counterfactual": "recompute"}
	default:
		value = map[string]any{"evidence": []string{}, "contradictions": []string{}, "confidence": .5, "uncertainty": .5, "evidence_score": .5, "consistency_score": .5, "contradiction_score": .1, "feasibility_score": .5, "risk_score": .1, "utility_score": .5, "diversity_score": .5, "hard_constraint_violation": false, "hard_constraint_reason": ""}
	}
	encoded, _ := json.Marshal(value)
	return model.GenerateResponse{}, json.Unmarshal(encoded, destination)
}

type exactHypothesisVerifier string

func (v exactHypothesisVerifier) Name() string { return "exact hypothesis" }
func (v exactHypothesisVerifier) Verify(_ context.Context, _ string, state *State) VerificationResult {
	return VerificationResult{Verifier: v.Name(), Passed: state.Hypothesis == string(v)}
}

func TestGravityRecoveryIsBoundedAndUsesRejectedAnswers(t *testing.T) {
	engine := NewEngine(gravityRecoveryProvider{}, DefaultConfig())
	engine.Verifier = exactHypothesisVerifier("correct")
	engine.GravityWells = []GravityWell{{ID: "verified-well", Strength: .8, SupportingEvidence: 2, Keywords: []string{"verified", "method"}}}
	engine.TraceDir = ""
	result, trace, err := engine.Run(context.Background(), "find the correct route")
	if err != nil {
		t.Fatal(err)
	}
	if result.Selected == nil || result.Selected.Hypothesis != "correct" || trace.Metrics.GravityRecoveryAttempts != 1 || trace.Metrics.GravityRecoveries != 1 {
		t.Fatalf("bounded gravity recovery failed: result=%+v metrics=%+v", result, trace.Metrics)
	}
	if engine.GravityWells[0].ApplicationCount == 0 || engine.GravityWells[0].OutcomeCount == 0 || engine.GravityWells[0].LastUsedAt.IsZero() {
		t.Fatalf("resolved gravity applications were not recorded: %+v", engine.GravityWells[0])
	}
	if len(trace.Memory.GravityWells) != 1 || trace.Memory.GravityWells[0].ApplicationCount != engine.GravityWells[0].ApplicationCount {
		t.Fatalf("trace did not capture final gravity state: trace=%+v engine=%+v", trace.Memory.GravityWells, engine.GravityWells)
	}
}
