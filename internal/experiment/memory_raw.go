package experiment

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func memoryRows(s *MemoryExperiment) []MemoryRawResult {
	rows := make([]MemoryRawResult, 0, len(s.Cases)*4)
	for _, c := range s.Cases {
		seed := s.Config.Seed
		if seed != nil {
			x := *seed + c.Repetition - 1
			seed = &x
		}
		add := func(condition MemoryCondition, side SideResult, graph GraphMetrics, a MemoryAttribution, initialHash, finalHash string) {
			stability := NoMaterialChange
			if a.MemoryEffect == EffectHelpful {
				stability = JustifiedImprovement
			} else if a.MemoryEffect == EffectHarmful {
				stability = HarmfulAnchoring
			} else if a.MemoryEffect == EffectRecovered {
				stability = StabilityRecovery
			} else if a.EvidenceOverride {
				stability = JustifiedCorrection
			}
			rows = append(rows, MemoryRawResult{ExperimentID: s.ExperimentID, RunID: c.RunID, AttemptID: fmt.Sprintf("%s/%s/%d", c.RunID, condition, c.Repetition), CaseID: c.CaseID, DatasetVersion: s.DatasetVersion, DatasetHash: s.DatasetHash, Category: c.Category, Behavior: c.Behavior, Condition: condition, Repetition: c.Repetition, Seed: seed, Provider: s.Config.Provider, Model: s.Config.Model, ConfigurationHash: configHash(s.Config), Result: classifyEpistemic(side.Verification), Verification: side.Verification, Attribution: a, Stability: stability, Graph: graph, ModelCalls: side.ModelCalls, ProviderRuntime: side.Provider, Tokens: side.Tokens, Measurements: len(side.Measurements), Duration: side.Latency, InitialMemoryHash: initialHash, FinalMemoryHash: finalHash, FailureCode: side.FailureCategory, Error: side.FailureCategory, AttemptStartedAt: side.StartedAt, AttemptFinishedAt: side.FinishedAt, RecordPersistedAt: time.Now().UTC(), Timestamp: side.FinishedAt})
		}
		add(BaselineCondition, c.Baseline, GraphMetrics{}, MemoryAttribution{MemoryEffect: EffectNone, ObservableBasis: "memory inaccessible by condition"}, "", "")
		add(ColdCondition, c.Cold, c.ColdPair.Graph, MemoryAttribution{MemoryEffect: EffectNone, ObservableBasis: "empty isolated memory by condition"}, "", "")
		add(MemoryConditionEnabled, c.WithMemory, c.MemoryPair.Graph, c.MemoryAttribution, c.InitialMemoryHash, c.FinalMemoryHash)
		add(MisleadingCondition, c.Misleading, c.MisleadingPair.Graph, c.MisleadingAttribution, c.MisleadingInitialMemoryHash, c.MisleadingFinalMemoryHash)
		rows[len(rows)-2].BranchesAvoided, rows[len(rows)-2].ModelCallsAvoided = c.Metrics.MemoryLeverageBranches, c.Metrics.MemoryLeverageModelCalls
		rows[len(rows)-2].RecoveryAttempted, rows[len(rows)-2].RecoverySuccessful = c.MemoryPair.RecoveryAttempted, c.MemoryPair.RecoverySuccessful
		rows[len(rows)-1].RecoveryAttempted, rows[len(rows)-1].RecoverySuccessful = c.MisleadingPair.RecoveryAttempted, c.MisleadingPair.RecoverySuccessful
	}
	return rows
}
func writeMemoryRowsAtomic(path string, rows []MemoryRawResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".memory-results-*.jsonl")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	w := bufio.NewWriter(f)
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err = w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
