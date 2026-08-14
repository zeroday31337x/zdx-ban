package ban

import "math"

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }
func ApplyEvaluation(s *State, e Evaluation) {
	s.Evidence = e.Evidence
	s.Contradictions = e.Contradictions
	s.Confidence = clamp(e.Confidence)
	s.Uncertainty = clamp(e.Uncertainty)
	s.EvidenceScore = clamp(e.EvidenceScore)
	s.ConsistencyScore = clamp(e.ConsistencyScore)
	s.ContradictionScore = clamp(e.ContradictionScore)
	s.FeasibilityScore = clamp(e.FeasibilityScore)
	s.RiskScore = clamp(e.RiskScore)
	s.UtilityScore = clamp(e.UtilityScore)
	s.DiversityScore = clamp(e.DiversityScore)
	if e.HardConstraintViolation {
		s.AggregateScore = 0
		s.Status = Pruned
		return
	}
	s.AggregateScore = clamp(.25*s.EvidenceScore + .20*s.ConsistencyScore + .15*s.FeasibilityScore + .15*s.Confidence + .10*s.UtilityScore + .05*s.DiversityScore - .06*s.ContradictionScore - .04*s.RiskScore - .05*s.Uncertainty)
	s.Status = Evaluated
}
