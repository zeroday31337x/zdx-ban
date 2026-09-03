package cognitive

import (
	"testing"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/training"
)

func measured(outcome measurement.Outcome, authority measurement.Authority) measurement.Result {
	return measurement.Result{ID: "m-" + string(outcome), Outcome: outcome, Authority: authority}
}

func TestCandidatesFromBANTraceNil(t *testing.T) {
	if got := CandidatesFromBANTrace(nil, "model-state"); got != nil {
		t.Fatalf("expected nil for nil trace, got %v", got)
	}
}

func TestCandidatesFromBANTraceClassification(t *testing.T) {
	winner := &ban.State{ID: "b01", Hypothesis: "the answer", Status: ban.Selected, AggregateScore: 0.87, GravityWellID: "well-1", Measurements: []measurement.Result{measured(measurement.Supported, measurement.DeterministicRuntime)}}
	contradicted := &ban.State{ID: "b02", Hypothesis: "wrong answer", Status: ban.Failed, AggregateScore: 0.1, Measurements: []measurement.Result{measured(measurement.Contradicted, measurement.DeterministicRuntime)}}
	errored := &ban.State{ID: "b03", Hypothesis: "broken", Status: ban.Evaluated, Measurements: []measurement.Result{measured(measurement.Error, measurement.DeterministicRuntime)}}
	unmeasured := &ban.State{ID: "b04", Hypothesis: "unverified", Status: ban.Pruned, AggregateScore: 0.4}
	acceptedButUnmeasured := &ban.State{ID: "b05", Hypothesis: "accepted, no real verifier", Status: ban.Selected, AggregateScore: 0.6}

	trace := &ban.ExecutionTrace{
		RunID:      "run-1",
		Problem:    "why does it leak memory",
		FinishedAt: time.Unix(1000, 0).UTC(),
		Nodes:      []*ban.State{winner, contradicted, errored, unmeasured, acceptedButUnmeasured},
		Memory:     ban.MemoryInteraction{RetrievedMemoryIDs: []string{"mem-1", "mem-2"}},
	}

	got := CandidatesFromBANTrace(trace, "model-state-id")
	if len(got) != 5 {
		t.Fatalf("expected 5 candidates, got %d", len(got))
	}
	byNode := map[string]training.Candidate{}
	for _, c := range got {
		byNode[c.SourceGraphNode] = c
		if err := c.Validate(); err != nil {
			t.Fatalf("candidate %s invalid: %v", c.SourceGraphNode, err)
		}
		if c.Input != trace.Problem || c.SourceRun != trace.RunID {
			t.Fatalf("provenance mismatch: %+v", c)
		}
		if len(c.MemoryAttribution) != 2 || c.Provenance.ModelStateID != "model-state-id" {
			t.Fatalf("memory/model-state attribution missing: %+v", c)
		}
	}

	w := byNode["b01"]
	if w.Target != training.W1Candidate || w.ValidationState != training.W1Eligible {
		t.Fatalf("authoritative supported winner should be W1-eligible: %+v", w)
	}
	if w.Gravity.Value != 0.87 || w.Gravity.WellID != "well-1" {
		t.Fatalf("gravity not carried from aggregate score/well id: %+v", w.Gravity)
	}

	c := byNode["b02"]
	if c.Target != training.Rejected || c.ValidationState != training.PromotionContradicted {
		t.Fatalf("contradicted node should be rejected: %+v", c)
	}

	e := byNode["b03"]
	if e.Target != training.Rejected || e.ValidationState != training.PromotionRejected {
		t.Fatalf("errored measurement should be rejected: %+v", e)
	}

	u := byNode["b04"]
	if u.Target != training.MemoryOnly || u.ValidationState != training.Recorded {
		t.Fatalf("unmeasured node should stay memory-only: %+v", u)
	}

	a := byNode["b05"]
	if a.Target != training.MemoryOnly || a.ValidationState != training.Recorded {
		t.Fatalf("selected-but-unmeasured node must not be promoted to W1: %+v", a)
	}
}

func TestCandidatesFromBANTraceClampsOutOfRangeGravity(t *testing.T) {
	node := &ban.State{ID: "b01", Hypothesis: "h", Status: ban.Evaluated, AggregateScore: 1.5}
	trace := &ban.ExecutionTrace{RunID: "run-2", Problem: "p", FinishedAt: time.Unix(2000, 0).UTC(), Nodes: []*ban.State{node}}
	got := CandidatesFromBANTrace(trace, "")
	if len(got) != 1 || got[0].Gravity.Value != 1 {
		t.Fatalf("expected gravity clamped to 1, got %+v", got)
	}
	if err := got[0].Validate(); err != nil {
		t.Fatalf("clamped candidate should validate: %v", err)
	}
}
