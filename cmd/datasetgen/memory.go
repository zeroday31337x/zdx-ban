package main

import (
	"encoding/json"
	"fmt"
	"os"
	"zdx-ban/internal/experiment"
	"zdx-ban/internal/memory"
)

func writeMemory(path, version string, perCategory int) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	evals := cases(version+"-evaluation", perCategory)
	for i, evaluation := range evals {
		behaviors := []string{"USEFUL_MEMORY_LEVERAGE", "IRRELEVANT_MEMORY_REJECTION", "STALE_MEMORY_OVERRIDE", "MISLEADING_MEMORY_RECOVERY", "UNKNOWN_NOVEL_PRESERVATION", "REPEATED_PROBLEM_EFFICIENCY", "FAILURE_MEMORY_LEVERAGE", "CONFLICTING_HISTORICAL_OBSERVATIONS"}
		behavior := behaviors[i%len(behaviors)]
		good := experiment.SeedMemory{ID: fmt.Sprintf("memory-good-%03d", i+1), Title: "Prior measured strategy", Content: memoryStrategy(evaluation.Category), Category: evaluation.Category, StrategyType: evaluation.Category + "-strategy", CorrelationGroup: fmt.Sprintf("exposure-%03d", i+1), Tier: memory.Episodic, Kind: memory.Strategy, Status: memory.Supported, Tags: evaluation.Tags, SourceClass: memory.MemoryGuidance}
		bad := experiment.SeedMemory{ID: fmt.Sprintf("memory-bad-%03d", i+1), Title: "Stale superficially similar strategy", Content: misleadingStrategy(evaluation.Category), Category: evaluation.Category, StrategyType: evaluation.Category + "-strategy", CorrelationGroup: fmt.Sprintf("stale-%03d", i+1), Tier: memory.Domain, Kind: memory.FailurePattern, Status: memory.Contradicted, Tags: evaluation.Tags, SourceClass: memory.ModelAssertion}
		if behavior == "IRRELEVANT_MEMORY_REJECTION" {
			good.Category = "unrelated_historical_context"
			good.Tags = []string{"irrelevant"}
			good.Content = "A historically valid formatting strategy for a materially different task category."
		}
		if behavior == "FAILURE_MEMORY_LEVERAGE" {
			good.Kind = memory.FailurePattern
			good.Status = memory.Contradicted
			good.Content = "A prior route failed under its recorded context; use it as avoidance guidance, not a universal prohibition."
		}
		if behavior == "CONFLICTING_HISTORICAL_OBSERVATIONS" {
			good.Content = "Prior observations disagreed across contexts; inspect provenance and use current measurement rather than majority memory."
		}
		c := experiment.MemoryCase{DatasetVersion: version, ID: fmt.Sprintf("memory-%03d", i+1), Version: "2", Category: evaluation.Category, Relationship: "structurally related strategy transfer without evaluation answer", Origin: "deterministic local generator cmd/datasetgen", Difficulty: "smoke-to-moderate", NoveltyClass: behavior, Behavior: behavior, ExpectedObservables: []string{"final measurement outcome", "retrieval events", "condition delta"}, AuthorityInformation: []string{"current deterministic benchmark measurement outranks memory"}, Exposure: []experiment.SeedMemory{good}, Evaluation: evaluation, Misleading: []experiment.SeedMemory{bad}, Metadata: map[string]any{"exposureDoesNotContainAnswer": true, "importedAt": "deterministic-generation"}}
		if e = enc.Encode(c); e != nil {
			return e
		}
	}
	return f.Sync()
}
func memoryStrategy(c string) string {
	switch c {
	case "arithmetic_constraints":
		return "Respect explicit operation structure, compute grouped multiplication before subtraction, then check the final numeric form."
	case "logic_deduction":
		return "Translate each stated truth condition literally and test consistency before selecting the named entity."
	case "structured_transformation":
		return "Construct only requested fields, preserve declared types, and validate the resulting JSON shape."
	case "coding_debugging":
		return "Identify the runtime or language invariant violated by the snippet before naming the defect."
	default:
		return "Separate outdated surface cues from signed state-changing events and calculate from recorded transitions."
	}
}
func misleadingStrategy(c string) string {
	switch c {
	case "arithmetic_constraints":
		return "Apply subtraction before multiplication because operations should be read strictly left to right."
	case "logic_deduction":
		return "Prefer the first named entity without checking whether all truth conditions remain consistent."
	case "structured_transformation":
		return "Add explanatory fields and convert every value to text even when the schema says otherwise."
	case "coding_debugging":
		return "Assume every failure is a nil pointer and ignore the specific language invariant."
	default:
		return "Treat the visible label as current and ignore later signed audit transitions."
	}
}
