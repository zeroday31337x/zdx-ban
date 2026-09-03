package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zdx-ban/internal/appconfig"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/benchmark"
	"zdx-ban/internal/cognitive"
	"zdx-ban/internal/experiment"
	"zdx-ban/internal/modelstate"
	"zdx-ban/internal/training"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ban:", err)
		os.Exit(1)
	}
}
func run() error {
	config, args, err := loadAppConfig(os.Args[1:])
	if err != nil {
		return err
	}
	mode := "run"
	if len(args) > 0 && (args[0] == "run" || args[0] == "baseline" || args[0] == "benchmark" || args[0] == "experiment" || args[0] == "memory" || args[0] == "model" || args[0] == "config" || args[0] == "thought" || args[0] == "vm" || args[0] == "runtime" || args[0] == "training") {
		mode = args[0]
		args = args[1:]
	}
	if mode == "memory" {
		return memoryCommand(args, config.Runtime.MemoryFile)
	}
	if mode == "config" {
		if len(args) != 1 || (args[0] != "show" && args[0] != "validate") {
			return fmt.Errorf("config requires show or validate")
		}
		if args[0] == "validate" {
			fmt.Println("ban.config valid")
			return nil
		}
		return printJSON(config)
	}
	if mode == "model" || mode == "thought" || mode == "vm" || mode == "runtime" || mode == "training" {
		return runtimeCommand(mode, args, config)
	}
	if mode == "experiment" {
		p := newOllama(config)
		return experiment.Command(args, p, config.Model.Name, experimentDefaults(config))
	}
	fs := flag.NewFlagSet("ban", flag.ContinueOnError)
	branches := fs.Int("branches", config.Search.InitialBranches, "initial semantic branches")
	retain := fs.Int("retain", config.Search.RetainBranches, "leading branches to expand")
	depth := fs.Int("depth", config.Search.MaxDepth, "maximum graph depth")
	nodes := fs.Int("nodes", config.Search.MaxNodes, "maximum graph nodes")
	concurrency := fs.Int("concurrency", 0, "override both model and evaluation concurrency")
	modelConcurrency := fs.Int("model-concurrency", config.Search.MaxConcurrentModelCalls, "bounded model-call concurrency")
	evaluationConcurrency := fs.Int("evaluation-concurrency", config.Search.MaxConcurrentEvaluations, "bounded evaluation concurrency")
	timeout := fs.Duration("timeout", config.Runtime.RunTimeout.Duration, "run timeout")
	dataset := fs.String("dataset", config.Runtime.BenchmarkDataset, "benchmark dataset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *concurrency < 0 || *modelConcurrency < 1 || *evaluationConcurrency < 1 {
		return fmt.Errorf("concurrency values must be positive; --concurrency may be zero when unused")
	}
	if *branches < 1 || *retain < 1 || *retain > *branches || *depth < 1 || *nodes < *branches {
		return fmt.Errorf("invalid search bounds: require branches >= retain >= 1, depth >= 1, and nodes >= branches")
	}
	p := newOllama(config)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	remaining := fs.Args()
	if mode == "baseline" {
		if len(remaining) == 0 {
			return fmt.Errorf("problem is required")
		}
		r, err := benchmark.Baseline(ctx, p, strings.Join(remaining, " "), config.Generation.Baseline.Temperature, config.Generation.Baseline.MaxTokens)
		if err == nil {
			fmt.Println(r.Answer)
		}
		return err
	}
	cfg := config.BANConfig()
	cfg.InitialBranches = *branches
	cfg.RetainBranches = *retain
	cfg.MaxDepth = *depth
	cfg.MaxNodes = *nodes
	cfg.MaxConcurrentModelCalls = *modelConcurrency
	cfg.MaxConcurrentEvaluations = *evaluationConcurrency
	if *concurrency > 0 {
		cfg.MaxConcurrentModelCalls = *concurrency
		cfg.MaxConcurrentEvaluations = *concurrency
	}
	e := ban.NewEngine(p, cfg)
	configureEngine(e, config)
	e.Logf = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	if mode == "benchmark" {
		items, err := benchmark.Load(*dataset)
		if err != nil {
			return err
		}
		out, err := benchmark.Run(ctx, p, e, items)
		if err != nil {
			return err
		}
		for _, r := range out {
			fmt.Printf("%s baseline=%s BAN=%s nodes=%d recovered=%v\n", r.Item.ID, r.Baseline.Latency, r.BANAnswer, r.BANNodes, r.Recovered)
		}
		return nil
	}
	if len(remaining) == 0 {
		return fmt.Errorf("problem is required")
	}
	result, tr, err := e.Run(ctx, strings.Join(remaining, " "))
	if err != nil {
		return err
	}
	fmt.Println("\n" + result.Answer)
	fmt.Printf("\ntrace: %s\n", filepath.Join(config.Runtime.TraceDir, tr.RunID+".json"))
	writeTrainingCandidates(config, tr)
	return nil
}

// writeTrainingCandidates records an observational training-candidate JSONL
// file alongside the run's trace. It is a side artifact, not core BAN
// functionality: a failure here is reported but never fails the run.
func writeTrainingCandidates(config appconfig.Config, tr *ban.ExecutionTrace) {
	if config.Runtime.TraceDir == "" || tr == nil {
		return
	}
	modelStateID, err := modelstate.DeclaredW0(config.Model.FoundationID, config.Model.Name).ID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ban: warning: could not compute model-state id for training candidates:", err)
		return
	}
	candidates := cognitive.CandidatesFromBANTrace(tr, modelStateID)
	path, err := training.WriteCandidatesJSONL(config.Runtime.TraceDir, tr.RunID, candidates)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ban: warning: could not write training candidates:", err)
		return
	}
	if path != "" {
		fmt.Printf("candidates: %s\n", path)
	}
}
