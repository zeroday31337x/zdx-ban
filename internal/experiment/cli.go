package experiment

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/model"
)

func Command(args []string, p model.Provider, modelName string) error {
	if len(args) == 0 {
		return fmt.Errorf("experiment requires smoke, run, or report")
	}
	action := args[0]
	args = args[1:]
	if action == "memory-smoke" || action == "memory-run" {
		return MemoryCommand(action, args, p, modelName)
	}
	if action == "report" {
		if len(args) != 1 {
			return fmt.Errorf("report requires result file")
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var r ResultFile
		if err = json.Unmarshal(b, &r); err != nil {
			return err
		}
		return WriteReport(filepath.Join(filepath.Dir(args[0]), "report.md"), &r)
	}
	if action != "smoke" && action != "run" {
		return fmt.Errorf("unknown experiment action %q", action)
	}
	fs := flag.NewFlagSet("experiment "+action, flag.ContinueOnError)
	defaultData := "datasets/ban-experiment-001.jsonl"
	if action == "smoke" {
		defaultData = "datasets/ban-experiment-001-smoke.jsonl"
	}
	dataset := fs.String("dataset", defaultData, "JSONL dataset")
	reps := fs.Int("repetitions", 1, "paired repetitions")
	output := fs.String("output", "results", "result root")
	dry := fs.Bool("dry-run", false, "validate without model calls")
	resume := fs.String("resume", "", "experiment ID to resume")
	verbose := fs.Bool("verbose", false, "show detailed traces")
	temp := fs.Float64("temperature", .2, "generation temperature")
	tokens := fs.Int("max-tokens", 1024, "per-call generation limit")
	timeout := fs.Duration("timeout", 5*time.Minute, "timeout policy")
	seed := fs.Int("seed", 42, "base seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	registry := NewRegistry()
	data, err := LoadDataset(*dataset, registry)
	if err != nil {
		return err
	}
	cfg := RunConfig{Model: modelName, Provider: "ollama", Temperature: *temp, Seed: seed, MaxTokens: *tokens, Timeout: *timeout, Repetitions: *reps, BAN: ban.DefaultConfig(), RequireObjectiveVerification: true}
	r := Runner{Provider: p, Registry: registry, Dataset: data, Config: cfg, OutputRoot: *output, ExperimentID: *resume, Verbose: *verbose}
	if *dry {
		if err = r.DryRun(); err != nil {
			return err
		}
		fmt.Printf("dry-run valid: %d cases, dataset=%s, sha256=%s, objective verification required\n", len(data.Cases), data.Version, data.SHA256)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r.Progress = func(row PairedResult, n, total int) {
		fmt.Printf("[%02d/%02d] %s baseline=%s BAN=%s recovery=%v\n", n, total, row.CaseID, outcomeLabel(row.Baseline.Verification), outcomeLabel(row.BAN.Verification), row.RecoverySuccessful)
	}
	result, err := r.Run(ctx, *resume != "")
	fmt.Printf("experiment: %s pairs=%d baseline=%.2f%% BAN=%.2f%%\n", result.Manifest.ExperimentID, result.Summary.TotalPairs, result.Summary.Baseline.Percentage, result.Summary.BAN.Percentage)
	return err
}
func outcomeLabel(v Verification) string {
	if v.Passed {
		return "PASS"
	}
	return string(v.Outcome)
}
