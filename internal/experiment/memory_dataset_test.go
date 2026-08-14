package experiment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"zdx-ban/internal/ban"
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
