package experiment

import (
	"math"
	"sort"
	"time"
	"zdx-ban/internal/measurement"
)

type McNemarResult struct {
	BaselineOnly, BANOnly int
	ChiSquare, PValue     float64
	Significant           bool
}

func Summarize(rows []PairedResult) Summary {
	s := Summary{TotalPairs: len(rows), Categories: map[string]CategorySummary{}, GeneratedAt: time.Now().UTC()}
	var bl, ba []float64
	known := true
	bt, nt := 0, 0
	for _, r := range rows {
		addAccuracy(&s.Baseline, r.Baseline.Verification)
		addAccuracy(&s.BAN, r.BAN.Verification)
		addAccuracy(&s.BANInitial, r.BANInitial)
		if r.RecoveryAttempted {
			s.RecoveryAttempts++
		}
		if r.RecoverySuccessful {
			s.SuccessfulRecoveries++
		}
		s.BaselineCalls += r.Baseline.ModelCalls
		s.BANCalls += r.BAN.ModelCalls
		bl = append(bl, float64(r.Baseline.Latency))
		ba = append(ba, float64(r.BAN.Latency))
		if r.Baseline.Tokens == nil || r.BAN.Tokens == nil {
			known = false
		} else {
			bt += *r.Baseline.Tokens
			nt += *r.BAN.Tokens
		}
		addGraph(&s.Graph, r.Graph)
		c := s.Categories[r.Category]
		addAccuracy(&c.Baseline, r.Baseline.Verification)
		addAccuracy(&c.BAN, r.BAN.Verification)
		if r.RecoveryAttempted {
			c.RecoveryAttempts++
		}
		if r.RecoverySuccessful {
			c.Recoveries++
		}
		s.Categories[r.Category] = c
		for _, m := range append(append([]measurement.Result{}, r.CandidateMeasurements...), r.FinalMeasurements...) {
			addMeasurement(&s.Measurements, m)
		}
		for _, m := range r.Baseline.Measurements {
			addMeasurement(&s.Measurements, m)
		}
		if r.BANInitial.Measurement.Outcome == measurement.Supported && r.BAN.Verification.Measurement.Outcome == measurement.Contradicted {
			s.Measurements.CandidateSupportedFinalContradicted++
		}
	}
	finishAcc(&s.Baseline)
	finishAcc(&s.BAN)
	finishAcc(&s.BANInitial)
	for k, c := range s.Categories {
		finishAcc(&c.Baseline)
		finishAcc(&c.BAN)
		s.Categories[k] = c
	}
	if s.RecoveryAttempts > 0 {
		s.RecoveryRate = 100 * float64(s.SuccessfulRecoveries) / float64(s.RecoveryAttempts)
	}
	s.BaselineLatency = distribution(bl)
	s.BANLatency = distribution(ba)
	if known {
		s.BaselineTokens = &bt
		s.BANTokens = &nt
	}
	if s.BaselineCalls > 0 {
		s.VerifiedPer100CallsBaseline = 100 * float64(s.Baseline.Passed) / float64(s.BaselineCalls)
	}
	if s.BANCalls > 0 {
		s.VerifiedPer100CallsBAN = 100 * float64(s.BAN.Passed) / float64(s.BANCalls)
	}
	s.AccuracyGain = s.BAN.Percentage - s.Baseline.Percentage
	extra := s.BANCalls - s.BaselineCalls
	if extra > 0 {
		s.AccuracyGainPerAdditionalCall = s.AccuracyGain / float64(extra)
		s.RecoveryPerAdditionalCall = float64(s.SuccessfulRecoveries) / float64(extra)
	}
	s.McNemar = McNemar(rows)
	return s
}
func addAccuracy(a *Accuracy, v Verification) {
	switch v.Measurement.Outcome {
	case measurement.Supported:
		a.Total++
		a.Passed++
	case measurement.Contradicted:
		a.Total++
	default:
		a.Unscored++
	}
}
func finishAcc(a *Accuracy) {
	if a.Total > 0 {
		a.Percentage = 100 * float64(a.Passed) / float64(a.Total)
	}
}
func addMeasurement(s *MeasurementSummary, m measurement.Result) {
	switch m.Outcome {
	case measurement.Supported:
		s.Supported++
	case measurement.Contradicted:
		s.Contradicted++
	case measurement.Inconclusive, measurement.Unsupported:
		s.Inconclusive++
	case measurement.Error:
		s.Errors++
	case measurement.NotMeasured:
		s.NotMeasured++
	}
	s.DeterministicLocalCalls += m.Cost.LocalMeasurementCalls
	s.PaidInferenceCalls += m.Cost.PaidInferenceCalls
	s.Latency += m.Cost.Latency
}
func addGraph(a *GraphMetrics, b GraphMetrics) {
	a.CandidateProposals += b.CandidateProposals
	a.NodesCreated += b.NodesCreated
	a.DuplicateProposals += b.DuplicateProposals
	a.DuplicatesDetected += b.DuplicatesDetected
	a.AnswerConvergences += b.AnswerConvergences
	a.DiversityRegenerations += b.DiversityRegenerations
	a.Convergences += b.Convergences
	a.MultipleParentNodes += b.MultipleParentNodes
	a.BranchesPruned += b.BranchesPruned
	a.PrunedCandidates += b.PrunedCandidates
	a.ProviderEvaluations += b.ProviderEvaluations
	a.GravityRoutedBranches += b.GravityRoutedBranches
	a.GravityWellHits += b.GravityWellHits
	a.GravityRecoveryAttempts += b.GravityRecoveryAttempts
	a.GravityRecoveries += b.GravityRecoveries
	if b.MaxDepthReached > a.MaxDepthReached {
		a.MaxDepthReached = b.MaxDepthReached
	}
	if b.PeakActiveBranches > a.PeakActiveBranches {
		a.PeakActiveBranches = b.PeakActiveBranches
	}
}
func distribution(v []float64) Distribution {
	d := Distribution{Count: len(v)}
	if len(v) == 0 {
		return d
	}
	sort.Float64s(v)
	d.Min = v[0]
	d.Max = v[len(v)-1]
	for _, x := range v {
		d.Mean += x
	}
	d.Mean /= float64(len(v))
	if len(v)%2 == 1 {
		d.Median = v[len(v)/2]
	} else {
		d.Median = (v[len(v)/2-1] + v[len(v)/2]) / 2
	}
	for _, x := range v {
		d.StdDev += (x - d.Mean) * (x - d.Mean)
	}
	d.StdDev = math.Sqrt(d.StdDev / float64(len(v)))
	return d
}
func McNemar(rows []PairedResult) *McNemarResult {
	b, c := 0, 0
	for _, r := range rows {
		bo, ba := r.Baseline.Verification.Measurement.Outcome, r.BAN.Verification.Measurement.Outcome
		if bo != measurement.Supported && bo != measurement.Contradicted || ba != measurement.Supported && ba != measurement.Contradicted {
			continue
		}
		if bo == measurement.Supported && ba == measurement.Contradicted {
			b++
		}
		if bo == measurement.Contradicted && ba == measurement.Supported {
			c++
		}
	}
	if b+c == 0 {
		return nil
	}
	chi := math.Pow(math.Abs(float64(b-c))-1, 2) / float64(b+c)
	p := math.Erfc(math.Sqrt(chi / 2))
	return &McNemarResult{b, c, chi, p, p < .05}
}
