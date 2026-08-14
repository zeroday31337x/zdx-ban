package experiment

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
)

func MemoryCommand(action string, args []string, p model.Provider, modelName string) error {
	fs := flag.NewFlagSet("experiment "+action, flag.ContinueOnError)
	datasetDefault := "datasets/ban-memory-experiment-001.jsonl"
	if action == "memory-smoke" {
		datasetDefault = "datasets/ban-memory-experiment-001-smoke.jsonl"
	}
	dataset := fs.String("dataset", datasetDefault, "memory experiment JSONL")
	reps := fs.Int("repetitions", 1, "repetitions")
	output := fs.String("output", "results", "result root")
	dry := fs.Bool("dry-run", false, "validate without model calls")
	resume := fs.String("resume", "", "experiment ID")
	maxRecords := fs.Int("memory-records", 8, "working memory record bound")
	maxChars := fs.Int("memory-chars", 6000, "working memory character bound")
	writePolicy := fs.String("memory-write", "writable", "writable or read-only")
	timeout := fs.Duration("timeout", 5*time.Minute, "timeout")
	seed := fs.Int("seed", 42, "seed")
	if e := fs.Parse(args); e != nil {
		return e
	}
	registry := NewRegistry()
	data, e := LoadMemoryDataset(*dataset, registry)
	if e != nil {
		return e
	}
	retrieval := memory.DefaultRetrievalConfig()
	retrieval.MaxRecords = *maxRecords
	retrieval.MaxChars = *maxChars
	cfg := RunConfig{Model: modelName, Provider: "ollama", Temperature: .2, Seed: seed, MaxTokens: 1024, Timeout: *timeout, Repetitions: *reps, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true, MemoryEnabled: true, MemoryRetrieval: retrieval, MemoryConsolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, MemoryWritePolicy: *writePolicy}
	runner := MemoryRunner{Provider: p, Registry: registry, Dataset: data, Config: cfg, OutputRoot: *output, ExperimentID: *resume, Retrieval: retrieval, Consolidation: cfg.MemoryConsolidation, WritePolicy: *writePolicy}
	if *dry {
		if e = runner.DryRun(); e != nil {
			return e
		}
		fmt.Printf("memory dry-run valid: %d cases, dataset=%s, sha256=%s, memory-schema=%s\n", len(data.Cases), data.Version, data.SHA256, memory.SchemaVersion)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner.Progress = func(x MemoryCaseResult, n, total int) {
		fmt.Printf("[%02d/%02d] %s cold=%s memory=%s misleading=%s retrieved=%d\n", n, total, x.CaseID, outcomeLabel(x.Cold.Verification), outcomeLabel(x.WithMemory.Verification), outcomeLabel(x.Misleading.Verification), x.Metrics.RetrievalCount)
	}
	result, e := runner.Run(ctx, *resume != "")
	fmt.Printf("memory experiment=%s cases=%d initial-memory=%s final-memory=%s\n", result.ExperimentID, len(result.Cases), result.InitialMemoryHash, result.FinalMemoryHash)
	return e
}
