package experiment

import (
	"context"
	"testing"
	"zdx-ban/internal/measurement"
)

func TestCandidateSupportedFinalContradictedIsFinalFailure(t *testing.T) {
	c := validCase()
	v, _ := NewRegistry().Get("numeric")
	candidate := verify(v, context.Background(), c, "7")
	final := verify(v, context.Background(), c, "8")
	row := PairedResult{Category: c.Category, BANInitial: candidate, BAN: SideResult{Verification: final}, CandidateMeasurements: []measurement.Result{candidate.Measurement}, FinalMeasurements: []measurement.Result{final.Measurement}}
	s := Summarize([]PairedResult{row})
	if !candidate.Passed || final.Passed || s.BAN.Passed != 0 || s.Measurements.CandidateSupportedFinalContradicted != 1 {
		t.Fatalf("candidate leaked into final accuracy: %+v", s)
	}
}
func TestMeasurementErrorIsUnscoredNotModelIncorrect(t *testing.T) {
	v := Verification{Measurement: measurement.Result{Outcome: measurement.Error, Error: &measurement.MeasurementError{Code: "CRASH", Message: "measurement failed"}}}
	s := Summarize([]PairedResult{{Baseline: SideResult{Verification: v}, BAN: SideResult{Verification: v}, BANInitial: v}})
	if s.Baseline.Total != 0 || s.Baseline.Unscored != 1 || s.Baseline.Percentage != 0 {
		t.Fatalf("measurement error scored as model failure: %+v", s.Baseline)
	}
}
func TestMeasurementProvenanceAndZeroPaidCalls(t *testing.T) {
	c := validCase()
	c.Measurement.Contract.Provenance.DatasetHash = "hash"
	c.Measurement.Contract.Provenance.GitCommit = "commit"
	v, _ := NewRegistry().Get("numeric")
	m := measure(v, context.Background(), c, "7")
	if m.Provenance.DatasetHash != "hash" || m.Provenance.GitCommit != "commit" || m.Provenance.Implementation == "" || m.Cost.PaidInferenceCalls != 0 || m.Cost.LocalMeasurementCalls != 1 {
		t.Fatalf("provenance/cost missing: %+v", m)
	}
}
