package simulation

import (
	"math"
	"sort"
)

// PathSummary is a compact explanation of a complete, modeled scenario path.
// Labels are the ordered outcome descriptions. Scores accumulate by stage.
type PathSummary struct {
	Labels           []string  `json:"labels"`
	Probability      float64   `json:"probability"`
	TotalScore       float64   `json:"total_score"`
	CumulativeScores []float64 `json:"cumulative_scores"`
}

// RiskAnalysis describes the distribution IMPLIED BY THE INPUT assumptions.
// None of these statistics are calibrated forecasts of real outcomes.
type RiskAnalysis struct {
	P10Score                      float64       `json:"p10_score"`
	MedianScore                   float64       `json:"median_score"`
	P90Score                      float64       `json:"p90_score"`
	WorstTenPercentMean           float64       `json:"worst_10_percent_mean"`
	ExpectedScoreByStage          []float64     `json:"expected_score_by_stage"`
	MostLikelyPaths               []PathSummary `json:"most_likely_paths"`
	WorstPaths                    []PathSummary `json:"worst_paths"`
	StressDelta                   float64       `json:"stress_delta,omitempty"`
	PessimisticShiftExpectedScore float64       `json:"pessimistic_shift_expected_score,omitempty"`
}

// analyzeRisk is called only AFTER walk has validated the tree and enforced its
// terminal-path and depth caps. Each stored path is bounded by maxDepth.
func analyzeRisk(branches []Branch, stressDelta float64) RiskAnalysis {
	paths := make([]PathSummary, 0, 16)
	var collect func([]Branch, float64, float64, []string, []float64)
	collect = func(nodes []Branch, p, total float64, labels []string, cumulative []float64) {
		for _, node := range nodes {
			nextP := p * node.Probability
			nextTotal := total + node.Score
			nextLabels := append(append([]string(nil), labels...), node.Label)
			nextScores := append(append([]float64(nil), cumulative...), nextTotal)
			if len(node.Next) > 0 {
				collect(node.Next, nextP, nextTotal, nextLabels, nextScores)
			} else {
				paths = append(paths, PathSummary{
					Labels: nextLabels, Probability: nextP,
					TotalScore: nextTotal, CumulativeScores: nextScores,
				})
			}
		}
	}
	collect(branches, 1, 0, nil, nil)
	risk := RiskAnalysis{}
	sortedScores := append([]PathSummary(nil), paths...)
	sort.SliceStable(sortedScores, func(i, j int) bool {
		return sortedScores[i].TotalScore < sortedScores[j].TotalScore
	})
	risk.P10Score = weightedQuantile(sortedScores, 0.1)
	risk.MedianScore = weightedQuantile(sortedScores, 0.5)
	risk.P90Score = weightedQuantile(sortedScores, 0.9)

	// Conditional lower-tail average, including a fractional boundary path.
	left, weighted := 0.1, 0.0
	for _, path := range sortedScores {
		take := math.Min(left, path.Probability)
		weighted += take * path.TotalScore
		left -= take
		if left <= 1e-12 {
			break
		}
	}
	risk.WorstTenPercentMean = weighted / 0.1

	horizon := 0
	for _, path := range paths {
		if len(path.CumulativeScores) > horizon {
			horizon = len(path.CumulativeScores)
		}
	}
	risk.ExpectedScoreByStage = make([]float64, horizon)
	for _, path := range paths {
		for stage := 0; stage < horizon; stage++ {
			index := stage
			// A terminated scenario retains its terminal value thereafter.
			if index >= len(path.CumulativeScores) {
				index = len(path.CumulativeScores)-1
			}
			risk.ExpectedScoreByStage[stage] += path.Probability * path.CumulativeScores[index]
		}
	}
	for i := 0; i < len(sortedScores) && i < 3; i++ {
		risk.WorstPaths = append(risk.WorstPaths, sortedScores[i])
	}
	sort.SliceStable(paths, func(i, j int) bool {
		if paths[i].Probability == paths[j].Probability {
			return paths[i].TotalScore < paths[j].TotalScore
		}
		return paths[i].Probability > paths[j].Probability
	})
	for i := 0; i < len(paths) && i < 3; i++ {
		risk.MostLikelyPaths = append(risk.MostLikelyPaths, paths[i])
	}
	if stressDelta > 0 {
		risk.StressDelta = stressDelta
		risk.PessimisticShiftExpectedScore = pessimisticShift(branches, stressDelta)
	}
	return risk
}

func weightedQuantile(sorted []PathSummary, q float64) float64 {
	cumulative := 0.0
	for _, path := range sorted {
		cumulative += path.Probability
		if cumulative+1e-12 >= q {
			return path.TotalScore
		}
	}
	return sorted[len(sorted)-1].TotalScore
}

// pessimisticShift is a deterministic ONE-PAIR stress test, not an optimal
// robust bound. At every branching point it moves up to delta probability
// from the locally highest-value child to the lowest-value child.
// Nested values are themselves stressed before the parent comparison.
func pessimisticShift(branches []Branch, delta float64) float64 {
	values := make([]float64, len(branches))
	expectation := 0.0
	for i, node := range branches {
		v := node.Score
		if len(node.Next) > 0 {
			v += pessimisticShift(node.Next, delta)
		}
		values[i] = v
		expectation += node.Probability * v
	}
	if len(branches) < 2 {
		return expectation
	}
	lowest, highest := 0, 0
	for i := range values {
		if values[i] < values[lowest] {
			lowest = i
		}
		if values[i] > values[highest] {
			highest = i
		}
	}
	if lowest == highest || values[highest] <= values[lowest] {
		return expectation
	}
	shift := math.Min(delta, branches[highest].Probability)
	return expectation - shift*(values[highest]-values[lowest])
}
