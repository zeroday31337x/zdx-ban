package ban

import (
	"time"
	"zdx-ban/internal/measurement"
)

func ApplyMeasurement(s *State, r measurement.Result) {
	s.Measurements = append(s.Measurements, r)
	s.UpdatedAt = time.Now().UTC()
	agg := measurement.AggregateResults(s.Measurements)
	switch agg.Outcome {
	case measurement.Supported:
		s.EvidenceScore = 1
		if s.Status == Failed {
			s.Status = Evaluated
		}
	case measurement.Contradicted:
		s.ContradictionScore = 1
		s.EvidenceScore = 0
		if r.Authoritative() {
			s.Status = Failed
		}
	case measurement.Inconclusive, measurement.NotMeasured:
	case measurement.Error:
	}
}
func FactuallySelectable(s *State) bool {
	agg := measurement.AggregateResults(s.Measurements)
	return agg.Outcome == measurement.Supported && s.Status != Failed && s.Status != Pruned
}
