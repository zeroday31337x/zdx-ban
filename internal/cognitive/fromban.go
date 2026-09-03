package cognitive

import (
	"zdx-ban/internal/ban"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/training"
)

// CandidatesFromBANTrace turns one completed internal/ban search run into an
// observational record of training candidates, one per graph node visited
// during that run. It performs no I/O and never mutates the trace.
//
// Eligibility is deliberately conservative: a node is only ever classified as
// W1Candidate when the run's own deterministic verifier produced an
// authoritative Supported measurement for it (see internal/ban.ApplyMeasurement
// and internal/measurement.AggregateResults). The default no-constraint
// AcceptVerifier never records a measurement, so an ordinary `ban run` yields
// MemoryOnly candidates only — this mirrors internal/cognitive.Loop, which
// makes the same distinction for its own (VM-executor) candidate source, and
// keeps this bridge from overclaiming evidentiary strength that a plain
// accept-verifier run never produced.
func CandidatesFromBANTrace(t *ban.ExecutionTrace, modelStateID string) []training.Candidate {
	if t == nil {
		return nil
	}
	out := make([]training.Candidate, 0, len(t.Nodes))
	for _, node := range t.Nodes {
		if node == nil {
			continue
		}
		agg := measurement.AggregateResults(node.Measurements)
		target := training.MemoryOnly
		state := training.Recorded
		switch {
		case agg.Outcome == measurement.Contradicted:
			target = training.Rejected
			state = training.PromotionContradicted
		case agg.Outcome == measurement.Error:
			target = training.Rejected
			state = training.PromotionRejected
		case node.Status == ban.Selected && agg.Outcome == measurement.Supported:
			target = training.W1Candidate
			state = training.W1Eligible
		}
		measurementIDs := make([]string, 0, len(node.Measurements))
		for _, m := range node.Measurements {
			measurementIDs = append(measurementIDs, m.ID)
		}
		timestamp := node.UpdatedAt
		if timestamp.IsZero() {
			timestamp = t.FinishedAt
		}
		retrieved := append([]string(nil), t.Memory.RetrievedMemoryIDs...)
		out = append(out, training.Candidate{
			SchemaVersion:      1,
			ID:                 training.StableID(t.RunID, node.ID, t.Problem),
			Timestamp:          timestamp,
			SourceRun:          t.RunID,
			SourceGraphNode:    node.ID,
			Input:              t.Problem,
			ModelOutput:        node.Hypothesis,
			MemoryAttribution:  retrieved,
			MeasurementOutcome: agg.Outcome,
			Confidence:         node.Confidence,
			Novelty:            node.DiversityScore,
			Usefulness:         node.UtilityScore,
			Contradictions:     append([]string(nil), node.Contradictions...),
			Gravity:            training.Gravity{Value: clamp01(node.AggregateScore), Source: "ban-engine-aggregate-score", WellID: node.GravityWellID},
			Target:             target,
			ValidationState:    state,
			Provenance: training.Provenance{
				RunID:          t.RunID,
				GraphNodeID:    node.ID,
				ModelStateID:   modelStateID,
				MeasurementIDs: measurementIDs,
				MemoryIDs:      retrieved,
			},
		})
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
