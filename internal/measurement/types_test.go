package measurement

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOutcomesRemainDistinct(t *testing.T) {
	values := []Outcome{Supported, Contradicted, Inconclusive, Error, Unsupported, NotMeasured}
	seen := map[Outcome]bool{}
	for _, v := range values {
		if seen[v] {
			t.Fatal("duplicate outcome")
		}
		seen[v] = true
	}
	if (Result{Outcome: Inconclusive}).Passed() || (Result{Outcome: Error}).Passed() || !(Result{Outcome: Supported}).Passed() {
		t.Fatal("compatibility pass semantics invalid")
	}
}
func TestVerificationClassesSerialize(t *testing.T) {
	classes := []VerificationClass{BenchmarkVerified, FormallyVerified, ExecutionVerified, MeasurementSupported, CausallySupported, Unverified}
	for _, c := range classes {
		b, err := json.Marshal(c)
		if err != nil || len(b) < 3 {
			t.Fatalf("class %s: %v", c, err)
		}
	}
}
func TestIndependenceCannotBeInvented(t *testing.T) {
	c := Contract{ID: "x", Claim: "claim", Observable: "o", Method: "model", Authority: ModelEstimate, Independence: Independent}
	if ValidateContract(c) == nil {
		t.Fatal("model estimate mislabeled independent")
	}
}
func TestConservativeAggregation(t *testing.T) {
	support := Result{Outcome: Supported, Authority: Observational}
	contradict := Result{Outcome: Contradicted, Authority: Formal}
	a := AggregateResults([]Result{support, contradict})
	if a.Outcome != Contradicted {
		t.Fatalf("authoritative contradiction lost: %s", a.Outcome)
	}
	inc := AggregateResults([]Result{{Outcome: Inconclusive}, {Outcome: Error}})
	if inc.Outcome != Inconclusive {
		t.Fatalf("unknown became %s", inc.Outcome)
	}
	none := AggregateResults(nil)
	if none.Outcome != NotMeasured {
		t.Fatal("missing measurement became success")
	}
}
func TestCorrelatedConvergenceNotIndependentVotes(t *testing.T) {
	p := Provenance{Timestamp: time.Now()}
	e := func(src string) Evidence {
		return Evidence{Source: src, Independent: true, CorrelationGroup: "same-model-path", Provenance: p}
	}
	a := AggregateResults([]Result{{Outcome: Supported, Evidence: []Evidence{e("path-a"), e("path-b")}}})
	if a.IndependentSources != 1 {
		t.Fatalf("correlated evidence counted %d times", a.IndependentSources)
	}
}
