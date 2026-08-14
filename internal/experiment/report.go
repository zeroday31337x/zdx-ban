package experiment

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func WriteReport(path string, r *ResultFile) error {
	var b strings.Builder
	s := r.Summary
	fmt.Fprintf(&b, "# BAN EXPERIMENT 001\n\n## Dataset\n\nCases: %d paired trials  \nDataset: %s  \nSHA-256: `%s`\n\n", s.TotalPairs, r.Manifest.DatasetVersion, r.Manifest.DatasetSHA256)
	fmt.Fprintf(&b, "## Model and configuration\n\nModel: %s  \nProvider: %s  \nTemperature: %.3g  \nMax tokens: %d  \nRepetitions: %d\n\n", r.Manifest.Configuration.Model, r.Manifest.Configuration.Provider, r.Manifest.Configuration.Temperature, r.Manifest.Configuration.MaxTokens, r.Manifest.Configuration.Repetitions)
	fmt.Fprintf(&b, "## Measurement-grounded correctness\n\nSupported means supported under the case measurement contract; it is not universal truth. Measurement errors and inconclusive results are unscored.\n\n## Benchmark-supported accuracy\n\nBaseline: %d/%d (%.2f%%%%)  \nBAN: %d/%d (%.2f%%%%)  \nBAN initial winners: %d/%d (%.2f%%%%)\n\n", s.Baseline.Passed, s.Baseline.Total, s.Baseline.Percentage, s.BAN.Passed, s.BAN.Total, s.BAN.Percentage, s.BANInitial.Passed, s.BANInitial.Total, s.BANInitial.Percentage)
	fmt.Fprintf(&b, "## Recovery\n\nInitial failures: %d  \nSuccessful recoveries: %d  \nRecovery rate: %.2f%%\n\n", s.RecoveryAttempts, s.SuccessfulRecoveries, s.RecoveryRate)
	fmt.Fprintf(&b, "## Cost\n\nModel calls — baseline: %d, BAN: %d  \nMean latency (ns) — baseline: %.0f, BAN: %.0f  \nVerified successes per 100 calls — baseline: %.2f, BAN: %.2f\n\n", s.BaselineCalls, s.BANCalls, s.BaselineLatency.Mean, s.BANLatency.Mean, s.VerifiedPer100CallsBaseline, s.VerifiedPer100CallsBAN)
	fmt.Fprintf(&b, "## Graph behavior\n\nNodes: %d  \nDuplicates detected: %d  \nConvergences: %d  \nMultiple-parent nodes: %d  \nPruned: %d  \nMaximum depth: %d\n\n", s.Graph.NodesCreated, s.Graph.DuplicatesDetected, s.Graph.Convergences, s.Graph.MultipleParentNodes, s.Graph.BranchesPruned, s.Graph.MaxDepthReached)
	b.WriteString("## Per-category results\n\n")
	keys := make([]string, 0, len(s.Categories))
	for k := range s.Categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := s.Categories[k]
		fmt.Fprintf(&b, "- %s: baseline %.2f%%, BAN %.2f%%, recoveries %d/%d\n", k, c.Baseline.Percentage, c.BAN.Percentage, c.Recoveries, c.RecoveryAttempts)
	}
	b.WriteString(fmt.Sprintf("\n## Measurement basis\n\nSupported: %d  \nContradicted: %d  \nInconclusive: %d  \nErrors: %d  \nCandidate supported, final contradicted: %d  \nDeterministic local measurement calls: %d  \nPaid verification inference calls: %d\n", s.Measurements.Supported, s.Measurements.Contradicted, s.Measurements.Inconclusive, s.Measurements.Errors, s.Measurements.CandidateSupportedFinalContradicted, s.Measurements.DeterministicLocalCalls, s.Measurements.PaidInferenceCalls))
	b.WriteString("\n## Statistical analysis\n\n")
	if s.McNemar == nil {
		b.WriteString("No discordant pairs; McNemar's test is not informative.\n")
	} else {
		fmt.Fprintf(&b, "Continuity-corrected McNemar: χ²=%.4f, p=%.4f. Significant at 0.05: %v.\n", s.McNemar.ChiSquare, s.McNemar.PValue, s.McNemar.Significant)
	}
	b.WriteString("\n## Conclusion\n\n")
	switch {
	case s.TotalPairs == 0:
		b.WriteString("No live paired results exist; no accuracy conclusion can be drawn.\n")
	case s.BAN.Percentage > s.Baseline.Percentage:
		fmt.Fprintf(&b, "BAN improved verified accuracy by %.2f percentage points in this execution. This result is specific to the recorded dataset, model, configuration, and sample size.\n", s.AccuracyGain)
	case s.BAN.Percentage < s.Baseline.Percentage:
		fmt.Fprintf(&b, "BAN performed worse by %.2f percentage points in this execution. The failure records should guide diagnosis before architectural expansion.\n", -s.AccuracyGain)
	default:
		b.WriteString("BAN and baseline had equal verified accuracy in this execution; the result does not demonstrate an accuracy advantage.\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
