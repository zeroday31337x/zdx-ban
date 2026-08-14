package experiment

import (
	"fmt"
	"os"
	"strings"
	"zdx-ban/internal/measurement"
)

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
	}
	b.WriteString("## Measurement outcomes\n\n")
	for _, c := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
		fmt.Fprintf(&b, "- %s: supported=%d contradicted=%d inconclusive=%d errors=%d\n", c, counts[c][measurement.Supported], counts[c][measurement.Contradicted], counts[c][measurement.Inconclusive], counts[c][measurement.Error])
	}
	fmt.Fprintf(&b, "\n## Memory behavior\n\nRetrievals: %d  \nRelevant hits: %d  \nHarmful retrievals: %d  \nMemory-induced errors: %d  \nContradiction events: %d  \nSupersessions: %d  \nEpisodic writes: %d  \nConsolidations: %d  \nMemory leverage (branches avoided): %d  \nMemory leverage (model calls avoided): %d\n\n", metrics.RetrievalCount, metrics.RelevantHits, metrics.HarmfulRetrievals, metrics.MemoryInducedErrors, metrics.ContradictionEvents, metrics.SupersessionEvents, metrics.EpisodicWrites, metrics.ConsolidationEvents, metrics.MemoryLeverageBranches, metrics.MemoryLeverageModelCalls)
	b.WriteString("## Interpretation\n\nThese are measurement-relative outcomes within a controlled memory intervention. Historical memory is guidance, not current evidence. No universal intelligence or causal claim follows without live paired results and review.\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}
