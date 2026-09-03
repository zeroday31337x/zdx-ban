package experiment

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/training"
)

func readCandidatesJSONL(path string) ([]training.Candidate, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var out []training.Candidate
	for scanner.Scan() {
		var c training.Candidate
		if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, scanner.Err()
}

func writeMemoryCase(t *testing.T, c MemoryCase) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.jsonl")
	b, _ := json.Marshal(c)
	if e := os.WriteFile(p, append(b, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
	return p
}
func validMemoryCase() MemoryCase {
	c := validCase()
	return MemoryCase{DatasetVersion: "mv1", ID: "group-1", Version: "1", Category: c.Category, Relationship: "strategy transfer", Exposure: []SeedMemory{{ID: "seed-1", Title: "strategy", Content: "use operation precedence without copying answers", Category: c.Category, Tier: memory.Episodic, Kind: memory.Strategy, Status: memory.Supported, SourceClass: memory.MemoryGuidance}}, Evaluation: c, Misleading: []SeedMemory{{ID: "bad-1", Title: "stale", Content: "always read arithmetic left to right", Category: c.Category, Tier: memory.Domain, Kind: memory.FailurePattern, Status: memory.Stale, SourceClass: memory.ModelAssertion}}}
}
func TestMemoryDatasetValidationAndLeakage(t *testing.T) {
	c := validMemoryCase()
	if _, e := LoadMemoryDataset(writeMemoryCase(t, c), NewRegistry()); e != nil {
		t.Fatal(e)
	}
	c.Exposure[0].Content = "the answer is 7"
	if _, e := LoadMemoryDataset(writeMemoryCase(t, c), NewRegistry()); e == nil {
		t.Fatal("exact answer leakage accepted")
	}
	c = validMemoryCase()
	c.Exposure[0].Content = c.Evaluation.Prompt
	if _, e := LoadMemoryDataset(writeMemoryCase(t, c), NewRegistry()); e == nil {
		t.Fatal("duplicate prompt leakage accepted")
	}
}

func TestMemoryDatasetPopulatesMeasurementDatasetHash(t *testing.T) {
	d, err := LoadMemoryDataset("../../datasets/ban-memory-experiment-001-smoke.jsonl", NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.Cases {
		p := c.Evaluation.Measurement.Contract.Provenance
		if p.DatasetHash != d.SHA256 || p.DatasetVersion != d.Version {
			t.Fatalf("case %s provenance=%+v dataset=%s/%s", c.ID, p, d.Version, d.SHA256)
		}
	}
}
func TestMemorySeedHashReproducibleAndReset(t *testing.T) {
	ctx := context.Background()
	c := validMemoryCase()
	a, _ := SeedStore(ctx, c.Exposure, false)
	b, _ := SeedStore(ctx, c.Exposure, false)
	ha, _ := a.SnapshotHash(ctx)
	hb, _ := b.SnapshotHash(ctx)
	if ha != hb {
		t.Fatal("seed hash differs")
	}
	a.Reset(ctx)
	records, _ := a.List(ctx)
	if len(records) != 0 {
		t.Fatal("reset failed")
	}
}
func TestMemoryResumeRejectsStateDrift(t *testing.T) {
	p := &pairedMock{}
	c := validMemoryCase()
	data := MemoryDataset{Version: "mv1", Path: "memory", SHA256: "hash-a", Cases: []MemoryCase{c}}
	cfg := RunConfig{Model: "frozen", Provider: "mock", Temperature: .2, MaxTokens: 32, Timeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryEnabled: true}
	retrieval := memory.DefaultRetrievalConfig()
	r := MemoryRunner{Provider: p, Registry: NewRegistry(), Dataset: data, Config: cfg, OutputRoot: t.TempDir(), ExperimentID: "memory-resume", Retrieval: retrieval, Consolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, WritePolicy: "writable"}
	if _, e := r.Run(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	r.Dataset.Cases[0].Exposure[0].Content = "changed seed state"
	if _, e := r.Run(context.Background(), true); e == nil {
		t.Fatal("resume accepted dataset/memory drift")
	}
}

func TestProviderFailureDoesNotMutateConditionMemory(t *testing.T) {
	c := validMemoryCase()
	r := MemoryRunner{Provider: failureProvider{err: inference.NewFailure(inference.ProviderConnectionError, errors.New("offline"))}, Registry: NewRegistry(), Config: RunConfig{Model: "m", Provider: "fake", MaxTokens: 8, Timeout: time.Second, InferenceTimeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryOnlineLearning: true}, Retrieval: memory.DefaultRetrievalConfig(), WritePolicy: "writable"}
	useful, misleading := memory.NewMemoryStore(), memory.NewMemoryStore()
	row, err := r.runCase(context.Background(), c, 1, useful, misleading)
	if err != nil {
		t.Fatal(err)
	}
	if row.InitialMemoryHash != row.FinalMemoryHash {
		t.Fatalf("provider failure mutated memory: %s -> %s events=%+v", row.InitialMemoryHash, row.FinalMemoryHash, row.MemoryEvents)
	}
	if row.MisleadingInitialMemoryHash != row.MisleadingFinalMemoryHash {
		t.Fatalf("provider failure mutated misleading memory: %s -> %s", row.MisleadingInitialMemoryHash, row.MisleadingFinalMemoryHash)
	}
	if len(row.MemoryEvents) != 0 || row.Metrics.EpisodicWrites != 0 {
		t.Fatalf("unexpected mutation events: %+v", row.MemoryEvents)
	}
	if records, _ := useful.List(context.Background()); len(records) != 0 {
		t.Fatalf("provider failure created gravity evidence: %+v", records)
	}
}

func TestVerifiedOutcomeCreatesCumulativeGravityEvidence(t *testing.T) {
	ctx := context.Background()
	c := validMemoryCase()
	retrieval := memory.DefaultRetrievalConfig()
	r := MemoryRunner{Provider: &pairedMock{}, Registry: NewRegistry(), Dataset: MemoryDataset{Version: "mv1", SHA256: "hash"}, Config: RunConfig{Model: "frozen", Provider: "mock", Temperature: .2, MaxTokens: 32, Timeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryEnabled: true, MemoryOnlineLearning: true, MemoryRetrieval: retrieval}, Retrieval: retrieval, Consolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, WritePolicy: "writable", ExperimentID: "gravity-test"}
	useful, misleading := memory.NewMemoryStore(), memory.NewMemoryStore()
	row, err := r.runCase(ctx, c, 1, useful, misleading)
	if err != nil {
		t.Fatal(err)
	}
	if !row.WithMemory.Verification.Passed || len(row.LearnedMemoryRecords) == 0 {
		t.Fatalf("verified route was not learned: %+v", row)
	}
	records, _ := useful.List(ctx)
	wells := memory.BuildGravityWells(records, time.Now().UTC(), retrieval.Gravity)
	if len(wells) != 1 || wells[0].SupportingEvidence == 0 || wells[0].Strength <= 0 {
		t.Fatalf("verified route did not form gravity well: records=%+v wells=%+v", records, wells)
	}
	if row.LearnedMemoryRecords[0].StrategyType != c.Category || len(row.LearnedMemoryRecords[0].Provenance.MeasurementIDs) == 0 {
		t.Fatalf("gravity evidence lacks semantic route/provenance: %+v", row.LearnedMemoryRecords[0])
	}
}

func TestOnlineLearningRoutesLaterPromptThroughGravityWell(t *testing.T) {
	first := validMemoryCase()
	first.ID = "group-1"
	first.Exposure[0].ID = "seed-1"
	second := validMemoryCase()
	second.ID = "group-2"
	second.Exposure[0].ID = "seed-2"
	second.Evaluation.ID = "x-2"
	second.Evaluation.Expected = 7.5
	second.Evaluation.VerifierConfig["tolerance"] = .5
	second.Evaluation.Measurement.Contract.ID = "c-x-2"
	retrieval := memory.DefaultRetrievalConfig()
	data := MemoryDataset{Version: "mv1", Path: "memory", SHA256: "hash", Cases: []MemoryCase{first, second}}
	cfg := RunConfig{Model: "frozen", Provider: "mock", Temperature: .2, MaxTokens: 32, Timeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryEnabled: true, MemoryOnlineLearning: true, MemoryRetrieval: retrieval}
	runner := MemoryRunner{Provider: &pairedMock{}, Registry: NewRegistry(), Dataset: data, Config: cfg, OutputRoot: t.TempDir(), ExperimentID: "online-gravity", Retrieval: retrieval, Consolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, WritePolicy: "writable"}
	result, err := runner.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cases) != 2 {
		t.Fatalf("cases=%d", len(result.Cases))
	}
	firstMetrics, secondMetrics := result.Cases[0].Metrics, result.Cases[1].Metrics
	if firstMetrics.GravityEvidence != 0 || secondMetrics.GravityEvidence == 0 || secondMetrics.MaxGravityStrength <= firstMetrics.MaxGravityStrength || secondMetrics.GravityRoutedBranches == 0 {
		t.Fatalf("gravity did not rise across prompts: first=%+v second=%+v", firstMetrics, secondMetrics)
	}
	if len(result.Cases[0].LearnedMemoryRecords) == 0 || len(result.Cases[1].MemoryPair.Memory.GravityWells) == 0 {
		t.Fatalf("learned route was not isolated/persisted: first=%+v second-memory=%+v", result.Cases[0].LearnedMemoryRecords, result.Cases[1].MemoryPair.Memory)
	}
}

func TestEpisodeRoutingSummaryDoesNotExposeAnswerOrFreeformReasoning(t *testing.T) {
	c := validMemoryCase()
	pair := PairedResult{SelectedRoute: BranchRoute{Title: "Direct arithmetic", ReasoningSummary: "the result is 130"}, FinalMeasurements: []measurement.Result{{Observation: 130.0, Expected: 130.0, Outcome: measurement.Supported}}, BAN: SideResult{Verification: Verification{Measurement: measurement.Result{Outcome: measurement.Supported}}}}
	episode := episodeFromPair(c, pair)
	visible := strings.Join(episode.UsefulBranches, " ")
	if strings.Contains(visible, "130") || strings.Contains(visible, "the result") || visible != "Direct arithmetic" {
		t.Fatalf("unsafe gravity route summary: %q", visible)
	}
}

func TestDeterministicFourConditionRunPersistsRawRows(t *testing.T) {
	p := &pairedMock{}
	c := validMemoryCase()
	data := MemoryDataset{Version: "mv1", Path: "memory", SHA256: "hash", Cases: []MemoryCase{c}}
	cfg := RunConfig{Model: "frozen", Provider: "deterministic-fake", Temperature: .2, MaxTokens: 32, Timeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryEnabled: true}
	retrieval := memory.DefaultRetrievalConfig()
	root := t.TempDir()
	r := MemoryRunner{Provider: p, Registry: NewRegistry(), Dataset: data, Config: cfg, OutputRoot: root, ExperimentID: "four-condition", Retrieval: retrieval, Consolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, WritePolicy: "read-only"}
	if _, err := r.Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadMemoryRows(filepath.Join(root, "four-condition", "memory-results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("want four isolated observations, got %d", len(rows))
	}
	candidatesPath := filepath.Join(root, "four-condition", "experiment.candidates.jsonl")
	if _, statErr := os.Stat(candidatesPath); statErr != nil {
		t.Fatalf("expected experiment.candidates.jsonl from the memory runner: %v", statErr)
	}
	candidates, err := readCandidatesJSONL(candidatesPath)
	if err != nil {
		t.Fatal(err)
	}
	var sawW1 bool
	for _, c := range candidates {
		if c.Target == training.W1Candidate {
			sawW1 = true
		}
	}
	if !sawW1 {
		t.Fatalf("expected at least one W1-eligible candidate across the four conditions, got %d candidates", len(candidates))
	}
	seen := map[MemoryCondition]bool{}
	for _, row := range rows {
		seen[row.Condition] = true
		if row.AttemptID == "" || row.RunID == "" {
			t.Fatal("missing run identity")
		}
	}
	for _, cond := range []MemoryCondition{BaselineCondition, ColdCondition, MemoryConditionEnabled, MisleadingCondition} {
		if !seen[cond] {
			t.Fatalf("missing %s", cond)
		}
	}
}
