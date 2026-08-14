package experiment

import (
	"math"
	"sort"
	"time"
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
		s.Baseline.Total++
		s.BAN.Total++
		s.BANInitial.Total++
		if r.Baseline.Verification.Passed {
			s.Baseline.Passed++
		}
		if r.BAN.Verification.Passed {
			s.BAN.Passed++
		}
		if r.BANInitial.Passed {
			s.BANInitial.Passed++
		}
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
		c.Baseline.Total++
		c.BAN.Total++
		if r.Baseline.Verification.Passed {
			c.Baseline.Passed++
		}
		if r.BAN.Verification.Passed {
			c.BAN.Passed++
		}
		if r.RecoveryAttempted {
			c.RecoveryAttempts++
		}
		if r.RecoverySuccessful {
			c.Recoveries++
		}
		s.Categories[r.Category] = c
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
func finishAcc(a *Accuracy) {
	if a.Total > 0 {
		a.Percentage = 100 * float64(a.Passed) / float64(a.Total)
	}
}
func addGraph(a *GraphMetrics, b GraphMetrics) {
	a.NodesCreated += b.NodesCreated
	a.DuplicatesDetected += b.DuplicatesDetected
	a.Convergences += b.Convergences
	a.MultipleParentNodes += b.MultipleParentNodes
	a.BranchesPruned += b.BranchesPruned
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
		if r.Baseline.Verification.Passed && !r.BAN.Verification.Passed {
			b++
		}
		if !r.Baseline.Verification.Passed && r.BAN.Verification.Passed {
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
