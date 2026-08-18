package training

import (
	"bytes"
	"context"
	"testing"
	"time"
	"zdx-ban/internal/measurement"
)

func candidate() Candidate {
	return Candidate{SchemaVersion: 1, ID: "c", Timestamp: time.Unix(1, 0).UTC(), SourceRun: "r", Input: "i", MeasurementOutcome: measurement.Supported, Target: MemoryOnly, ValidationState: Recorded, Gravity: Gravity{Value: .5, Source: "test"}}
}
func TestCandidateStableProvenanceAndNoPromotion(t *testing.T) {
	if StableID("r", "n", "i") != StableID("r", "n", "i") {
		t.Fatal("unstable id")
	}
	c := candidate()
	c.Provenance = Provenance{ExecutionID: "e", MeasurementIDs: []string{"m"}}
	s := &MemoryStore{}
	if e := s.Append(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	if s.Items[0].Provenance.ExecutionID != "e" {
		t.Fatal("provenance lost")
	}
	if _, e := (DisabledPolicy{}).Next(c, Recorded); e == nil {
		t.Fatal("automatic promotion enabled")
	}
	c.ValidationState = Promoted
	if e := c.Validate(); e == nil {
		t.Fatal("auto-promoted candidate accepted")
	}
}
func TestExportDeterministicAndContradictedRejected(t *testing.T) {
	c := candidate()
	c.MeasurementOutcome = measurement.Contradicted
	c.Target = Rejected
	c.ValidationState = PromotionContradicted
	var a, b bytes.Buffer
	ma, e := Export(context.Background(), &a, []Candidate{c})
	if e != nil {
		t.Fatal(e)
	}
	mb, e := Export(context.Background(), &b, []Candidate{c})
	if e != nil || a.String() != b.String() || ma.CandidatesSHA256 != mb.CandidatesSHA256 || ma.TargetWeightDistribution[Rejected] != 1 {
		t.Fatal("non-deterministic or mislabeled export")
	}
}
