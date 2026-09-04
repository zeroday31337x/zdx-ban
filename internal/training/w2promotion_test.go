package training

import (
	"testing"
	"time"
)

func w1FromRun(id, run, input, output string, at time.Time) Candidate {
	c := candidate()
	c.ID = id
	c.SourceRun = run
	c.Input = input
	c.ModelOutput = output
	c.Target = W1Candidate
	c.ValidationState = W1Eligible
	c.Timestamp = at
	return c
}

func TestW2PromotionCandidatesRequiresIndependentRuns(t *testing.T) {
	base := time.Unix(1000, 0).UTC()
	repeated := []Candidate{
		w1FromRun("tc-1", "run-1", "why does it leak memory", "the increment was never applied", base),
		w1FromRun("tc-2", "run-2", "why does it leak memory", "the increment was never applied", base.Add(time.Hour)),
		w1FromRun("tc-3", "run-3", "why does it leak memory", "the increment was never applied", base.Add(2*time.Hour)),
	}
	onceOnly := w1FromRun("tc-4", "run-1", "why does the queue drop messages", "consumer acked early", base)

	out := W2PromotionCandidates(append(repeated, onceOnly), 3, base.Add(3*time.Hour))
	if len(out) != 1 {
		t.Fatalf("expected exactly one promoted input, got %d: %+v", len(out), out)
	}
	promoted := out[0]
	if promoted.Input != "why does it leak memory" {
		t.Fatalf("unexpected promoted input: %+v", promoted)
	}
	if promoted.RepetitionCount != 3 {
		t.Fatalf("expected repetition count 3, got %d", promoted.RepetitionCount)
	}
	if promoted.Target != W2Candidate || promoted.ValidationState != W2Eligible {
		t.Fatalf("expected W2Candidate/W2Eligible classification, got target=%s state=%s", promoted.Target, promoted.ValidationState)
	}
	if len(promoted.PromotedFrom) != 3 {
		t.Fatalf("expected all 3 underlying candidate ids traced, got %v", promoted.PromotedFrom)
	}
	if err := promoted.Validate(); err != nil {
		t.Fatalf("promoted candidate should validate: %v", err)
	}
}

func TestW2PromotionCandidatesIgnoresNonW1Targets(t *testing.T) {
	memoryOnly := candidate()
	memoryOnly.Input = "never confirmed"
	memoryOnly.Target = MemoryOnly
	rejected := candidate()
	rejected.ID = "r1"
	rejected.Input = "never confirmed"
	rejected.Target = Rejected

	out := W2PromotionCandidates([]Candidate{memoryOnly, rejected}, 1, time.Now().UTC())
	if len(out) != 0 {
		t.Fatalf("expected no promotions from non-W1 candidates, got %+v", out)
	}
}

func TestW2PromotionCandidatesDoesNotDoubleCountSameRun(t *testing.T) {
	base := time.Unix(2000, 0).UTC()
	sameRunTwice := []Candidate{
		w1FromRun("tc-a", "run-1", "same input, two nodes", "answer", base),
		w1FromRun("tc-b", "run-1", "same input, two nodes", "answer", base.Add(time.Minute)),
	}
	out := W2PromotionCandidates(sameRunTwice, 2, base.Add(time.Hour))
	if len(out) != 0 {
		t.Fatalf("two candidates from the same run must not satisfy a threshold of 2 distinct runs, got %+v", out)
	}
}

func TestW2PromotionCandidatesThresholdBelowOneClampsToOne(t *testing.T) {
	base := time.Unix(3000, 0).UTC()
	single := w1FromRun("tc-1", "run-1", "single confirmation", "answer", base)
	out := W2PromotionCandidates([]Candidate{single}, 0, base.Add(time.Hour))
	if len(out) != 1 || out[0].RepetitionCount != 1 {
		t.Fatalf("expected threshold<1 to clamp to 1, got %+v", out)
	}
}

func TestW2PromotionCandidatesIsDeterministic(t *testing.T) {
	base := time.Unix(4000, 0).UTC()
	candidates := []Candidate{
		w1FromRun("tc-1", "run-1", "b input", "answer-b", base),
		w1FromRun("tc-2", "run-2", "b input", "answer-b", base),
		w1FromRun("tc-3", "run-1", "a input", "answer-a", base),
		w1FromRun("tc-4", "run-2", "a input", "answer-a", base),
	}
	now := base.Add(time.Hour)
	first := W2PromotionCandidates(candidates, 2, now)
	second := W2PromotionCandidates(candidates, 2, now)
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected both inputs promoted, got %d and %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Fatalf("expected deterministic ids across calls: %+v vs %+v", first, second)
	}
	if first[0].Input != "a input" || first[1].Input != "b input" {
		t.Fatalf("expected sorted input order, got %s then %s", first[0].Input, first[1].Input)
	}
	// Re-running with the same input set should reproduce the same
	// promotion id, since the id is a pure function of the input text.
	rerunID := W2PromotionCandidates(candidates, 2, now.Add(time.Hour))[0].ID
	if rerunID != first[0].ID {
		t.Fatalf("promotion id should be stable across reruns regardless of `now`: %s vs %s", rerunID, first[0].ID)
	}
}
