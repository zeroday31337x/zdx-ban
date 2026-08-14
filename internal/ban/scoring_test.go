package ban

import "testing"

func TestScoringConstraintsAndSeparation(t *testing.T) {
	a := NewState("a", Proposal{}, 0)
	ApplyEvaluation(a, Evaluation{EvidenceScore: .2, ConsistencyScore: .8, FeasibilityScore: .8, Confidence: .5, UtilityScore: 1})
	b := NewState("b", Proposal{}, 0)
	ApplyEvaluation(b, Evaluation{EvidenceScore: .9, ConsistencyScore: .8, FeasibilityScore: .8, Confidence: .8, UtilityScore: .2})
	if a.UtilityScore != 1 || a.EvidenceScore != .2 {
		t.Fatal("utility contaminated evidence")
	}
	if b.AggregateScore <= a.AggregateScore {
		t.Fatal("evidence should dominate utility")
	}
	bad := NewState("x", Proposal{}, 0)
	ApplyEvaluation(bad, Evaluation{HardConstraintViolation: true, UtilityScore: 1, EvidenceScore: 1})
	if bad.Status != Pruned || bad.AggregateScore != 0 {
		t.Fatal("hard constraint did not reject")
	}
	again := NewState("c", Proposal{}, 0)
	ApplyEvaluation(again, Evaluation{EvidenceScore: .9, ConsistencyScore: .8, FeasibilityScore: .8, Confidence: .8, UtilityScore: .2})
	if again.AggregateScore != b.AggregateScore {
		t.Fatal("score nondeterministic")
	}
}
