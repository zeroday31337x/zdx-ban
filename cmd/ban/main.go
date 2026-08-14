package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/benchmark"
	"zdx-ban/internal/experiment"
	"zdx-ban/internal/model"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ban:", err)
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	mode := "run"
	if len(args) > 0 && (args[0] == "run" || args[0] == "baseline" || args[0] == "benchmark" || args[0] == "experiment" || args[0] == "memory") {
		mode = args[0]
		args = args[1:]
	}
	if mode == "memory" {
		return memoryCommand(args)
	}
	modelName := env("BAN_MODEL", "qwen2.5:1.5b")
	baseURL := env("OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	if mode == "experiment" {
		p := model.NewOllama(baseURL, modelName, 5*time.Minute)
		return experiment.Command(args, p, modelName)
	}
	fs := flag.NewFlagSet("ban", flag.ContinueOnError)
	branches := fs.Int("branches", 5, "initial semantic branches")
	retain := fs.Int("retain", 2, "leading branches to expand")
	depth := fs.Int("depth", 2, "maximum graph depth")
	nodes := fs.Int("nodes", 20, "maximum graph nodes")
	concurrency := fs.Int("concurrency", 1, "bounded evaluation concurrency")
	timeout := fs.Duration("timeout", 5*time.Minute, "run timeout")
	dataset := fs.String("dataset", "datasets/initial.json", "benchmark dataset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := model.NewOllama(baseURL, modelName, *timeout)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	remaining := fs.Args()
	if mode == "baseline" {
		if len(remaining) == 0 {
			return fmt.Errorf("problem is required")
		}
		r, err := benchmark.Baseline(ctx, p, strings.Join(remaining, " "), .2, 1024)
		if err == nil {
			fmt.Println(r.Answer)
		}
		return err
	}
	cfg := ban.DefaultConfig()
	cfg.InitialBranches = *branches
	cfg.RetainBranches = *retain
	cfg.MaxDepth = *depth
	cfg.MaxNodes = *nodes
	cfg.MaxConcurrentEvaluations = *concurrency
	cfg.MaxConcurrentModelCalls = *concurrency
	e := ban.NewEngine(p, cfg)
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
	fmt.Printf("\ntrace: traces/%s.json\n", tr.RunID)
	return nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
