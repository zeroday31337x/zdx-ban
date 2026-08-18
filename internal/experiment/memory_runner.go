package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
	tr "zdx-ban/internal/trace"
)

type MemoryCondition string

const (
	BaselineCondition      MemoryCondition = "BASELINE"
	ColdCondition          MemoryCondition = "BAN_COLD"
	MemoryConditionEnabled MemoryCondition = "BAN_MEMORY"
	MisleadingCondition    MemoryCondition = "BAN_MISLEADING_MEMORY"
)

type MemoryMetrics struct {
	RetrievalCount, RelevantHits, RetrievedUnused, HarmfulRetrievals, MemoryInducedErrors, ContradictionEvents, SupersessionEvents, WorkingMemoryChars, EpisodicWrites, ConsolidationEvents int
	MemoryLeverageBranches                                                                                                                                                                  int
	MemoryLeverageModelCalls                                                                                                                                                                int
}
type MemoryCaseResult struct {
	CaseID, Category, Behavior                             string
	Repetition                                             int
	RunID                                                  string
	Baseline, Cold, WithMemory, Misleading                 SideResult
	ColdPair, MemoryPair, MisleadingPair                   PairedResult
	InitialMemoryHash, FinalMemoryHash                     string
	MisleadingInitialMemoryHash, MisleadingFinalMemoryHash string
	MemoryRetrievalDuration, MisleadingRetrievalDuration   time.Duration
	Metrics                                                MemoryMetrics
	MemoryAttribution, MisleadingAttribution               MemoryAttribution
	MemoryEvents                                           []memory.UpdateEvent
	EpisodeID                                              string
	Consolidations                                         []memory.ConsolidationRecord
	CompletedAt                                            time.Time
}
type MemoryExperiment struct {
	Experiment, SchemaVersion, TraceSchemaVersion, MemorySchemaVersion, ExperimentID string
	DatasetVersion, DatasetHash                                                      string
	StartingCommit, GitRemote                                                        string
	GitDirty                                                                         bool
	Config                                                                           RunConfig
	ConditionOrder                                                                   []MemoryCondition
	InitialMemoryHash, FinalMemoryHash                                               string
	Cases                                                                            []MemoryCaseResult
	StartedAt                                                                        time.Time
	FinishedAt                                                                       *time.Time `json:"finishedAt,omitempty"`
}
type MemoryRunner struct {
	Provider                 model.Provider
	Registry                 *Registry
	Dataset                  MemoryDataset
	Config                   RunConfig
	OutputRoot, ExperimentID string
	Retrieval                memory.RetrievalConfig
	Consolidation            memory.ConsolidationConfig
	WritePolicy              string
	Progress                 func(MemoryCaseResult, int, int)
}

func (r *MemoryRunner) Validate() error {
	if r.Provider == nil || r.Registry == nil {
		return fmt.Errorf("provider and registry required")
	}
	if len(r.Dataset.Cases) == 0 {
		return fmt.Errorf("memory dataset empty")
	}
	if r.Config.Repetitions < 1 {
		return fmt.Errorf("repetitions required")
	}
	if r.Retrieval.MaxRecords <= 0 || r.Retrieval.MaxChars <= 0 {
		return fmt.Errorf("bounded retrieval required")
	}
	for _, c := range r.Dataset.Cases {
		if e := ValidateCase(c.Evaluation, r.Registry); e != nil {
			return e
		}
	}
	return nil
}
func (r *MemoryRunner) DryRun() error { return r.Validate() }
func (r *MemoryRunner) Run(ctx context.Context, resume bool) (MemoryExperiment, error) {
	if e := r.Validate(); e != nil {
		return MemoryExperiment{}, e
	}
	if r.ExperimentID == "" {
		r.ExperimentID = newID() + "-MEMORY"
	}
	dir := filepath.Join(r.OutputRoot, r.ExperimentID)
	if e := os.MkdirAll(dir, 0755); e != nil {
		return MemoryExperiment{}, e
	}
	state := MemoryExperiment{Experiment: "BAN-MEMORY-EXPERIMENT-001", SchemaVersion: SchemaVersion, TraceSchemaVersion: ban.TraceSchemaVersion, MemorySchemaVersion: memory.SchemaVersion, ExperimentID: r.ExperimentID, DatasetVersion: r.Dataset.Version, DatasetHash: r.Dataset.SHA256, StartingCommit: gitCommit(), GitRemote: gitRemote(), GitDirty: gitDirty(), Config: r.Config, StartedAt: time.Now().UTC()}
	state.ConditionOrder = []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition}
	allSeeds := []SeedMemory{}
	for _, c := range r.Dataset.Cases {
		allSeeds = append(allSeeds, c.Exposure...)
	}
	global, _ := SeedStore(ctx, allSeeds, false)
	state.InitialMemoryHash, _ = global.SnapshotHash(ctx)
	if resume {
		var prior MemoryExperiment
		if e := readJSON(filepath.Join(dir, "memory-summary.json"), &prior); e != nil {
			return state, e
		}
		if prior.DatasetHash != state.DatasetHash || configHash(prior.Config) != configHash(state.Config) || prior.InitialMemoryHash != state.InitialMemoryHash {
			return state, fmt.Errorf("resume memory state/configuration drift")
		}
		state = prior
	}
	done := map[string]bool{}
	for _, x := range state.Cases {
		done[pairKey(x.CaseID, x.Repetition)] = true
	}
	total := len(r.Dataset.Cases) * r.Config.Repetitions
	for rep := 1; rep <= r.Config.Repetitions; rep++ {
		for _, c := range r.Dataset.Cases {
			if done[pairKey(c.ID, rep)] {
				continue
			}
			select {
			case <-ctx.Done():
				_ = persistMemory(dir, &state)
				return state, ctx.Err()
			default:
			}
			caseCtx := ctx
			cancelCase := func() {}
			if r.Config.CaseTimeout > 0 {
				caseCtx, cancelCase = context.WithTimeout(ctx, r.Config.CaseTimeout)
			}
			row, e := r.runCase(caseCtx, c, rep)
			cancelCase()
			if e != nil {
				return state, e
			}
			state.Cases = append(state.Cases, row)
			if e = persistMemory(dir, &state); e != nil {
				return state, e
			}
			if r.Progress != nil {
				r.Progress(row, len(state.Cases), total)
			}
		}
	}
	state.FinalMemoryHash = memoryFinalHash(state.Cases)
	now := time.Now().UTC()
	state.FinishedAt = &now
	if e := persistMemory(dir, &state); e != nil {
		return state, e
	}
	return state, nil
}
func (r *MemoryRunner) runCase(ctx context.Context, c MemoryCase, rep int) (MemoryCaseResult, error) {
	baseRunner := Runner{Provider: r.Provider, Registry: r.Registry, Config: r.Config}
	cold := baseRunner.runPair(ctx, c.Evaluation, rep)
	row := MemoryCaseResult{CaseID: c.ID, Category: c.Category, Behavior: c.Behavior, Repetition: rep, RunID: newID(), Baseline: cold.Baseline, Cold: cold.BAN, ColdPair: cold, CompletedAt: time.Now().UTC()}
	seeded, e := SeedStore(ctx, c.Exposure, false)
	if e != nil {
		return row, e
	}
	row.InitialMemoryHash, _ = seeded.SnapshotHash(ctx)
	req := memory.RetrievalRequest{Query: c.Evaluation.Prompt, Category: c.Category, Tags: c.Evaluation.Tags, Now: time.Now().UTC(), Config: r.Retrieval, ExcludeCaseID: c.Evaluation.ID, ExcludeExactAnswer: fmt.Sprint(c.Evaluation.Expected)}
	retrievalStart := time.Now()
	retrieved, e := memory.Retrieve(ctx, seeded, req)
	row.MemoryRetrievalDuration = time.Since(retrievalStart)
	if e != nil {
		return row, e
	}
	memRunner := baseRunner
	memRunner.SkipBaseline = true
	memRunner.MemoryRetrieval = &retrieved
	withMemory := memRunner.runPair(ctx, c.Evaluation, rep)
	row.WithMemory = withMemory.BAN
	row.WithMemory.MemoryRetrievalDuration = row.MemoryRetrievalDuration
	row.MemoryPair = withMemory
	row.Metrics.RetrievalCount = len(retrieved.Records)
	row.Metrics.WorkingMemoryChars = retrieved.ApproxChars
	for _, x := range retrieved.Records {
		row.MemoryAttribution.MemoryIDs = append(row.MemoryAttribution.MemoryIDs, x.Record.ID)
		if row.MemoryAttribution.MemoryRelevance == nil {
			row.MemoryAttribution.MemoryRelevance = map[string]float64{}
		}
		row.MemoryAttribution.MemoryRelevance[x.Record.ID] = x.Reason.Score
		if x.Reason.CategoryMatch || x.Reason.TokenOverlap > 0 {
			row.Metrics.RelevantHits++
		}
	}
	row.MemoryAttribution.MemoryRetrieved = len(retrieved.Records) > 0
	row.MemoryAttribution.ObservableBasis = "retrieval events plus measured cold/memory outcome delta; use is a behavioral proxy, not chain-of-thought"
	misSeeds := append(append([]SeedMemory{}, c.Exposure...), c.Misleading...)
	misStore, _ := SeedStore(ctx, misSeeds, false)
	row.MisleadingInitialMemoryHash, _ = misStore.SnapshotHash(ctx)
	misRetrievalStart := time.Now()
	misRetrieved, _ := memory.Retrieve(ctx, misStore, req)
	row.MisleadingRetrievalDuration = time.Since(misRetrievalStart)
	misRunner := baseRunner
	misRunner.SkipBaseline = true
	misRunner.MemoryRetrieval = &misRetrieved
	mis := misRunner.runPair(ctx, c.Evaluation, rep)
	row.Misleading = mis.BAN
	row.Misleading.MemoryRetrievalDuration = row.MisleadingRetrievalDuration
	row.MisleadingPair = mis
	for _, x := range misRetrieved.Records {
		row.MisleadingAttribution.MemoryIDs = append(row.MisleadingAttribution.MemoryIDs, x.Record.ID)
		if row.MisleadingAttribution.MemoryRelevance == nil {
			row.MisleadingAttribution.MemoryRelevance = map[string]float64{}
		}
		row.MisleadingAttribution.MemoryRelevance[x.Record.ID] = x.Reason.Score
	}
	row.MisleadingAttribution.MemoryRetrieved = len(misRetrieved.Records) > 0
	row.MisleadingAttribution.ObservableBasis = "retrieval, current measurement, recovery, and condition delta"
	if withMemory.BAN.Verification.Measurement.Outcome == measurement.Contradicted && cold.BAN.Verification.Measurement.Outcome == measurement.Supported {
		row.Metrics.MemoryInducedErrors++
	}
	for _, x := range misRetrieved.Records {
		if x.Record.Status == memory.Contradicted || x.Record.Status == memory.Stale {
			row.Metrics.HarmfulRetrievals++
			if mis.BAN.Verification.Measurement.Authoritative() {
				row.MisleadingAttribution.MemoryConflicted = append(row.MisleadingAttribution.MemoryConflicted, x.Record.ID)
				row.MisleadingAttribution.EvidenceOverride = true
				row.MisleadingAttribution.MemoryConflictResolution = "current authoritative measurement outranked historical guidance"
				current := mis.BAN.Verification.Measurement
				if current.Outcome == measurement.Supported {
					current.Outcome = measurement.Contradicted
					current.ID += "-memory-conflict"
				}
				ev, _ := memory.ApplyCurrentMeasurement(ctx, misStore, x.Record.ID, current)
				if ev.NewStatus == memory.Contradicted {
					row.Metrics.ContradictionEvents++
				}
			}
		}
	}
	episode := episodeFromPair(c, withMemory)
	if r.WritePolicy != "read-only" && len(withMemory.BAN.Measurements) > 0 && withMemory.BAN.Verification.Measurement.Outcome != measurement.Error {
		if episodeRecord, episodeErr := memory.RecordEpisode(ctx, seeded, episode, memory.Provenance{Source: "BAN memory experiment", ExperimentID: r.ExperimentID, CaseID: c.ID, TraceID: withMemory.TraceRunID, DatasetVersion: r.Dataset.Version, DatasetHash: r.Dataset.SHA256, GitCommit: gitCommit(), SourceClass: memory.MemoryGuidance, Authority: withMemory.BAN.Verification.Measurement.Authority, Independence: measurement.PartiallyIndependent, CorrelationGroup: c.ID}); episodeErr == nil {
			row.EpisodeID = episodeRecord.ID
			row.MemoryEvents = append(row.MemoryEvents, memory.UpdateEvent{RecordID: episodeRecord.ID, Action: "APPEND_EPISODE", Reason: "measured BAN condition outcome", NewStatus: episodeRecord.Status, At: episodeRecord.CreatedAt})
			row.Metrics.EpisodicWrites++
			events, _ := memory.Consolidate(ctx, seeded, r.Consolidation)
			row.Consolidations = append(row.Consolidations, events...)
			for _, ev := range events {
				if ev.CreatedRecordID != "" {
					row.Metrics.ConsolidationEvents++
				}
			}
		}
	}
	row.FinalMemoryHash, _ = seeded.SnapshotHash(ctx)
	row.MisleadingFinalMemoryHash, _ = misStore.SnapshotHash(ctx)
	row.Metrics.MemoryLeverageBranches = cold.Graph.NodesCreated - withMemory.Graph.NodesCreated
	row.Metrics.MemoryLeverageModelCalls = cold.BAN.ModelCalls - withMemory.BAN.ModelCalls
	classifyMemoryEffects(&row)
	return row, nil
}
func classifyMemoryEffects(row *MemoryCaseResult) {
	if classifyEpistemic(row.Cold.Verification) == EpistemicFailed || classifyEpistemic(row.WithMemory.Verification) == EpistemicFailed || classifyEpistemic(row.Misleading.Verification) == EpistemicFailed {
		row.MemoryAttribution.MemoryEffect = EffectUnknown
		row.MisleadingAttribution.MemoryEffect = EffectUnknown
		return
	}
	cold, mem, misleading := classifyEpistemic(row.Cold.Verification), classifyEpistemic(row.WithMemory.Verification), classifyEpistemic(row.Misleading.Verification)
	row.MemoryAttribution.MemoryUsed = row.MemoryAttribution.MemoryRetrieved && (cold != mem || row.Metrics.MemoryLeverageBranches != 0 || row.Metrics.MemoryLeverageModelCalls != 0)
	switch {
	case cold != EpistemicSupported && mem == EpistemicSupported:
		row.MemoryAttribution.MemoryEffect = EffectHelpful
	case cold == EpistemicSupported && mem != EpistemicSupported:
		row.MemoryAttribution.MemoryEffect = EffectHarmful
	case !row.MemoryAttribution.MemoryRetrieved:
		row.MemoryAttribution.MemoryEffect = EffectNone
	default:
		row.MemoryAttribution.MemoryEffect = EffectNeutral
	}
	row.MisleadingAttribution.MemoryUsed = row.MisleadingAttribution.MemoryRetrieved && (mem != misleading || row.MisleadingPair.RecoveryAttempted)
	switch {
	case row.MisleadingPair.RecoverySuccessful && misleading == EpistemicSupported:
		row.MisleadingAttribution.MemoryEffect = EffectRecovered
	case mem == EpistemicSupported && misleading != EpistemicSupported:
		row.MisleadingAttribution.MemoryEffect = EffectHarmful
	case !row.MisleadingAttribution.MemoryRetrieved:
		row.MisleadingAttribution.MemoryEffect = EffectNone
	default:
		row.MisleadingAttribution.MemoryEffect = EffectNeutral
	}
	if row.MisleadingAttribution.EvidenceOverride {
		row.MisleadingAttribution.MemoryRejected = append(row.MisleadingAttribution.MemoryRejected, row.MisleadingAttribution.MemoryConflicted...)
	}
}
func episodeFromPair(c MemoryCase, p PairedResult) memory.Episode {
	return memory.Episode{Problem: c.Evaluation.Prompt, TaskSignature: c.Relationship, Category: c.Category, Strategies: []string{p.FinalWinnerID}, CandidateMeasurements: p.CandidateMeasurements, FinalMeasurements: p.FinalMeasurements, Outcome: string(p.BAN.Verification.Measurement.Outcome), FailureClassification: p.BAN.FailureCategory, Recovered: p.RecoverySuccessful, RecoveryEvents: []string{p.InitialWinnerID + "->" + p.FinalWinnerID}, FailedStrategies: []string{p.InitialWinnerID}, Latency: p.BAN.Latency, ModelCalls: p.BAN.ModelCalls, Timestamp: time.Now().UTC()}
}
func persistMemory(dir string, s *MemoryExperiment) error {
	_, e := tr.WriteAtomic(dir, "memory-summary", s)
	if e != nil {
		return e
	}
	rows := memoryRows(s)
	if e = writeMemoryRowsAtomic(filepath.Join(dir, "memory-results.jsonl"), rows); e != nil {
		return e
	}
	if e = WriteMemoryRawReport(filepath.Join(dir, "pass5-report.md"), rows); e != nil {
		return e
	}
	return WriteMemoryReport(filepath.Join(dir, "memory-report.md"), s)
}
func readJSON(path string, dst any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, dst)
}
func memoryFinalHash(rows []MemoryCaseResult) string {
	hashes := make([]string, 0, len(rows))
	for _, r := range rows {
		hashes = append(hashes, r.FinalMemoryHash)
	}
	return configHash(hashes)
}
