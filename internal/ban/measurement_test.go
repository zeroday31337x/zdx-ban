package ban

import (
	"testing"
	"zdx-ban/internal/measurement"
)

func TestAuthoritativeMeasurementOverridesUtility(t *testing.T) {
	a := NewState("a", Proposal{}, 0)
	a.UtilityScore = .99
	ApplyMeasurement(a, measurement.Result{Outcome: measurement.Contradicted, Authority: measurement.Formal})
	b := NewState("b", Proposal{}, 0)
	b.UtilityScore = .55
	ApplyMeasurement(b, measurement.Result{Outcome: measurement.Supported, Authority: measurement.Formal})
	if FactuallySelectable(a) || !FactuallySelectable(b) || a.Status != Failed {
		t.Fatal("utility overrode measured evidence")
	}
}
func TestInconclusiveAndErrorPreserveBranch(t *testing.T) {
	for _, o := range []measurement.Outcome{measurement.Inconclusive, measurement.Error, measurement.NotMeasured} {
		s := NewState(string(o), Proposal{}, 0)
		ApplyMeasurement(s, measurement.Result{Outcome: o, Authority: measurement.UnknownAuthority})
		if s.Status == Failed {
			t.Fatalf("%s incorrectly failed reasoning", o)
		}
	}
}
