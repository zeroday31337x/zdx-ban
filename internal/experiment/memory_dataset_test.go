package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/memory"
)

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
	r := MemoryRunner{Provider: failureProvider{err: inference.NewFailure(inference.ProviderConnectionError, errors.New("offline"))}, Registry: NewRegistry(), Config: RunConfig{Model: "m", Provider: "fake", MaxTokens: 8, Timeout: time.Second, InferenceTimeout: time.Second, Repetitions: 1, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true}, Retrieval: memory.DefaultRetrievalConfig(), WritePolicy: "writable"}
	row, err := r.runCase(context.Background(), c, 1)
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
