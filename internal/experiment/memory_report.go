package experiment

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"zdx-ban/internal/measurement"
)

func WriteMemoryRawReport(path string, rows []MemoryRawResult) error {
	a := AggregateMemoryRows(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# BAN Pass 5 Memory Validation\n\nRaw observations: %d  \n\n", a.Rows)
	b.WriteString("## Outcome Distribution\n\n")
	for _, c := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
		x := a.ByCondition[c]
		fmt.Fprintf(&b, "- %s: supported=%d contradicted=%d inconclusive=%d unsupported=%d unknown=%d failed=%d\n", c, x[EpistemicSupported], x[EpistemicContradicted], x[EpistemicInconclusive], x[EpistemicUnsupported], x[EpistemicUnknown], x[EpistemicFailed])
	}
	b.WriteString("\n## Memory Utility and Harm\n\n")
	fmt.Fprintf(&b, "Retrieved records: %d  \nHelpful: %d  \nHarmful: %d  \nRecovered from harm: %d  \nRejected memories: %d  \nAuthoritative overrides: %d  \n", a.Retrievals, a.Helpful, a.Harmful, a.Recovered, a.IrrelevantRejected, a.StaleOverrides)
	den := func(n int) float64 {
		if a.RetrievalObservations == 0 {
			return 0
		}
		return float64(n) * 100 / float64(a.RetrievalObservations)
	}
	precision := 0.0
	if a.Retrievals > 0 {
		precision = float64(a.RelevantRetrievals) * 100 / float64(a.Retrievals)
	}
	fmt.Fprintf(&b, "Retrieval precision (score >= 0.20): %.2f%%  \nRetrieved but unused: %d  \n", precision, a.RetrievedUnused)
	fmt.Fprintf(&b, "Retrieval usefulness: %.2f%%  \nHarmful retrieval rate: %.2f%%  \nHarm recovery rate: %.2f%%  \nMeasurement-contract compliant: %d/%d  \nNovel candidates preserved: %d  \nBranches avoided: %d  \nModel calls avoided: %d  \n", den(a.Helpful), den(a.Harmful), den(a.Recovered), a.ContractCompliant, a.Rows, a.NovelCandidatesPreserved, a.BranchesAvoided, a.ModelCallsAvoided)
	b.WriteString("\n## Comparisons\n\n")
	for _, p := range [][2]MemoryCondition{{BaselineCondition, ColdCondition}, {ColdCondition, MemoryConditionEnabled}, {MemoryConditionEnabled, MisleadingCondition}} {
		c := CompareMemoryRows(rows, p[0], p[1])
		fmt.Fprintf(&b, "- %s -> %s: paired=%d improved=%d regressed=%d unchanged=%d\n", c.From, c.To, c.Total, c.Improved, c.Regressed, c.Unchanged)
	}
	b.WriteString("\n## Per-Category Results\n\n")
	cats := make([]string, 0, len(a.ByCategory))
	for c := range a.ByCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Fprintf(&b, "### %s\n\n", c)
		for _, cond := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
			x := a.ByCategory[c][cond]
			fmt.Fprintf(&b, "- %s: supported=%d contradicted=%d inconclusive=%d\n", cond, x[EpistemicSupported], x[EpistemicContradicted], x[EpistemicInconclusive])
		}
	}
	b.WriteString("\n## Limitations\n\nMemory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func WriteMemoryReport(path string, r *MemoryExperiment) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# BAN Memory Experiment\n\nDataset: %s  \nDataset hash: `%s`  \nCases: %d  \nMemory schema: %s  \nInitial memory hash: `%s`  \nFinal memory hash: `%s`\n\n", r.DatasetVersion, r.DatasetHash, len(r.Cases), r.MemorySchemaVersion, r.InitialMemoryHash, r.FinalMemoryHash)
	counts := map[MemoryCondition]map[measurement.Outcome]int{}
	for _, c := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
		counts[c] = map[measurement.Outcome]int{}
	}
	metrics := MemoryMetrics{}
	for _, x := range r.Cases {
		counts[BaselineCondition][x.Baseline.Verification.Measurement.Outcome]++
		counts[ColdCondition][x.Cold.Verification.Measurement.Outcome]++
		counts[MemoryConditionEnabled][x.WithMemory.Verification.Measurement.Outcome]++
		counts[MisleadingCondition][x.Misleading.Verification.Measurement.Outcome]++
		metrics.RetrievalCount += x.Metrics.RetrievalCount
		metrics.RelevantHits += x.Metrics.RelevantHits
		metrics.HarmfulRetrievals += x.Metrics.HarmfulRetrievals
		metrics.MemoryInducedErrors += x.Metrics.MemoryInducedErrors
		metrics.ContradictionEvents += x.Metrics.ContradictionEvents
		metrics.SupersessionEvents += x.Metrics.SupersessionEvents
		metrics.EpisodicWrites += x.Metrics.EpisodicWrites
		metrics.ConsolidationEvents += x.Metrics.ConsolidationEvents
		metrics.MemoryLeverageBranches += x.Metrics.MemoryLeverageBranches
		metrics.MemoryLeverageModelCalls += x.Metrics.MemoryLeverageModelCalls
		metrics.GravityWells += x.Metrics.GravityWells
		metrics.GravityEvidence += x.Metrics.GravityEvidence
		metrics.GravityRoutedBranches += x.Metrics.GravityRoutedBranches
		if x.Metrics.MaxGravityStrength > metrics.MaxGravityStrength {
			metrics.MaxGravityStrength = x.Metrics.MaxGravityStrength
		}
	}
	b.WriteString("## Measurement outcomes\n\n")
	for _, c := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
		fmt.Fprintf(&b, "- %s: supported=%d contradicted=%d inconclusive=%d errors=%d\n", c, counts[c][measurement.Supported], counts[c][measurement.Contradicted], counts[c][measurement.Inconclusive], counts[c][measurement.Error])
	}
	fmt.Fprintf(&b, "\n## Memory behavior\n\nRetrieved records in memory condition: %d  \nRelevant hits: %d  \nPotentially harmful records retrieved: %d  \nMeasured memory-induced errors: %d  \nContradiction events: %d  \nSupersessions: %d  \nEpisodic writes: %d  \nConsolidations: %d  \nGravity wells routed: %d  \nVerified gravity evidence: %d  \nPeak gravity strength: %.3f  \nBranches routed by gravity: %d  \nMemory leverage (branches avoided; comparable completed pairs only): %d  \nMemory leverage (model calls avoided; comparable completed pairs only): %d\n\n", metrics.RetrievalCount, metrics.RelevantHits, metrics.HarmfulRetrievals, metrics.MemoryInducedErrors, metrics.ContradictionEvents, metrics.SupersessionEvents, metrics.EpisodicWrites, metrics.ConsolidationEvents, metrics.GravityWells, metrics.GravityEvidence, metrics.MaxGravityStrength, metrics.GravityRoutedBranches, metrics.MemoryLeverageBranches, metrics.MemoryLeverageModelCalls)
	if len(r.Cases) > 0 {
		b.WriteString("## Gravity trajectory\n\n")
		for _, x := range r.Cases {
			fmt.Fprintf(&b, "- %s: strength=%.3f evidence=%d wells=%d routed-branches=%d memory-outcome=%s\n", x.CaseID, x.Metrics.MaxGravityStrength, x.Metrics.GravityEvidence, x.Metrics.GravityWells, x.Metrics.GravityRoutedBranches, x.WithMemory.Verification.Measurement.Outcome)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Interpretation\n\nThese are measurement-relative outcomes within a controlled memory intervention. Historical memory is guidance, not current evidence. No universal intelligence or causal claim follows without live paired results and review.\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}
