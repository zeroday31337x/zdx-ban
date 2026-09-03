package experiment

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
)

func MemoryCommand(action string, args []string, p model.Provider, modelName string, defaults CommandDefaults) error {
	fs := flag.NewFlagSet("experiment "+action, flag.ContinueOnError)
	datasetDefault := "datasets/ban-memory-experiment-001.jsonl"
	if action == "memory-smoke" {
		datasetDefault = "datasets/ban-memory-experiment-001-smoke.jsonl"
	}
	dataset := fs.String("dataset", datasetDefault, "memory experiment JSONL")
	foundationPath := fs.String("foundation-memory", "datasets/foundation-memory-v1.jsonl", "versioned foundation memory JSONL")
	reps := fs.Int("repetitions", defaults.Repetitions, "repetitions")
	output := fs.String("output", "results", "result root")
	dry := fs.Bool("dry-run", false, "validate without model calls")
	resume := fs.String("resume", "", "experiment ID")
	maxRecords := fs.Int("memory-records", 8, "working memory record bound")
	maxChars := fs.Int("memory-chars", 6000, "working memory character bound")
	writePolicy := fs.String("memory-write", "writable", "writable or read-only")
	gravityRouter := fs.Bool("gravity-router", true, "route memory and branches using evidence-backed gravity wells")
	onlineLearning := fs.Bool("online-memory-learning", true, "carry verified condition memory forward across prompts")
	timeout := fs.Duration("timeout", defaults.Timeout, "legacy inference timeout")
	inferenceTimeout := fs.Duration("inference-timeout", defaults.InferenceTimeout, "per provider inference deadline")
	caseTimeout := fs.Duration("case-timeout", defaults.CaseTimeout, "whole case deadline")
	runTimeout := fs.Duration("run-timeout", defaults.RunTimeout, "whole experiment deadline")
	caseID := fs.String("case-id", "", "run only this case ID")
	proposalTokens := fs.Int("proposal-max-tokens", defaults.ProposalMaxTokens, "per proposal-generation call limit")
	evaluationTokens := fs.Int("evaluation-max-tokens", defaults.EvaluationMaxTokens, "per branch-evaluation call limit")
	challengeTokens := fs.Int("challenge-max-tokens", defaults.ChallengeMaxTokens, "per challenge call limit")
	finalTokens := fs.Int("final-max-tokens", defaults.FinalAnswerMaxTokens, "final-answer call limit")
	temperature := fs.Float64("temperature", defaults.MemoryTemperature, "generation temperature")
	seed := fs.Int("seed", defaults.Seed, "seed")
	limit := fs.Int("limit", 0, "maximum cases; zero means all")
	live := fs.Bool("live", false, "explicitly permit live provider calls")
	if e := fs.Parse(args); e != nil {
		return e
	}
	registry := NewRegistry()
	data, e := LoadMemoryDataset(*dataset, registry)
	if e != nil {
		return e
	}
	foundation, e := LoadFoundationMemory(*foundationPath)
	if e != nil {
		return e
	}
	if *caseID != "" {
		filtered := data.Cases[:0]
		for _, c := range data.Cases {
			if c.ID == *caseID {
				filtered = append(filtered, c)
			}
		}
		data.Cases = filtered
		if len(data.Cases) == 0 {
			return fmt.Errorf("case ID %q not found", *caseID)
		}
	}
	if *limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if *limit > 0 && *limit < len(data.Cases) {
		data.Cases = data.Cases[:*limit]
	}
	retrieval := memory.DefaultRetrievalConfig()
	retrieval.MaxRecords = *maxRecords
	retrieval.MaxChars = *maxChars
	retrieval.Gravity.Enabled = *gravityRouter
	cfg := RunConfig{Model: modelName, Provider: defaults.Provider, FoundationID: defaults.FoundationID, Temperature: *temperature, Seed: seed, MaxTokens: defaults.MaxTokens, ProposalMaxTokens: *proposalTokens, EvaluationMaxTokens: *evaluationTokens, ChallengeMaxTokens: *challengeTokens, FinalAnswerMaxTokens: *finalTokens, Timeout: *timeout, InferenceTimeout: *inferenceTimeout, CaseTimeout: *caseTimeout, RunTimeout: *runTimeout, Streaming: true, Repetitions: *reps, BAN: defaults.BAN, RequireObjectiveVerification: true, MemoryEnabled: true, MemoryRetrieval: retrieval, MemoryConsolidation: memory.ConsolidationConfig{MinDistinctEpisodes: 2}, MemoryWritePolicy: *writePolicy, MemoryOnlineLearning: *onlineLearning}
	cfg.CaseLimit = *limit
	runner := MemoryRunner{Provider: p, Registry: registry, Dataset: data, Config: cfg, OutputRoot: *output, ExperimentID: *resume, Retrieval: retrieval, Consolidation: cfg.MemoryConsolidation, WritePolicy: *writePolicy, Foundation: foundation}
	banCalls := 1 + cfg.BAN.InitialBranches + cfg.BAN.RetainBranches + cfg.BAN.RetainBranches*2 + 2 + 1 + 1 + cfg.BAN.GravityRecoveryBranches
	providerCallsUpper := len(data.Cases) * *reps * (1 + 3*banCalls)
	providerEnvelope := time.Duration(providerCallsUpper) * *inferenceTimeout
	caseEnvelope := time.Duration(len(data.Cases)**reps) * *caseTimeout
	boundedEnvelope := providerEnvelope
	if caseEnvelope < boundedEnvelope {
		boundedEnvelope = caseEnvelope
	}
	if *runTimeout < boundedEnvelope {
		boundedEnvelope = *runTimeout
	}
	fmt.Printf("execution envelope: cases=%d repetitions=%d conditions=4 provider-calls-upper=%d inference=%s case=%s run=%s bounded-upper=%s\n", len(data.Cases), *reps, providerCallsUpper, *inferenceTimeout, *caseTimeout, *runTimeout, boundedEnvelope)
	if boundedEnvelope >= 24*time.Hour {
		fmt.Printf("warning: unusually large worst-case execution envelope: %s\n", boundedEnvelope)
	}
	if *dry {
		if e = runner.DryRun(); e != nil {
			return e
		}
		fmt.Printf("memory dry-run valid: %d cases, dataset=%s, sha256=%s, memory-schema=%s\n", len(data.Cases), data.Version, data.SHA256, memory.SchemaVersion)
		return nil
	}
	if !*live {
		return fmt.Errorf("live provider execution requires explicit --live (deterministic orchestration is exercised by tests)")
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancelRun := context.WithTimeout(signalCtx, *runTimeout)
	defer cancelRun()
	runner.Progress = func(x MemoryCaseResult, n, total int) {
		fmt.Printf("[%02d/%02d] %s cold=%s memory=%s misleading=%s retrieved=%d gravity=%.3f evidence=%d routed=%d\n", n, total, x.CaseID, outcomeLabel(x.Cold.Verification), outcomeLabel(x.WithMemory.Verification), outcomeLabel(x.Misleading.Verification), x.Metrics.RetrievalCount, x.Metrics.MaxGravityStrength, x.Metrics.GravityEvidence, x.Metrics.GravityRoutedBranches)
	}
	result, e := runner.Run(ctx, *resume != "")
	fmt.Printf("memory experiment=%s cases=%d initial-memory=%s final-memory=%s\n", result.ExperimentID, len(result.Cases), result.InitialMemoryHash, result.FinalMemoryHash)
	return e
}

func MemoryArtifactCommand(action string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s requires memory-results.jsonl", action)
	}
	rows, err := LoadMemoryRows(args[0])
	if err != nil {
		return err
	}
	switch action {
	case "memory-report":
		out := "memory-pass5-report.md"
		if len(args) > 1 {
			out = args[1]
		}
		return WriteMemoryRawReport(out, rows)
	case "memory-inspect":
		if len(args) < 2 {
			return fmt.Errorf("memory-inspect requires case ID")
		}
		found := false
		for _, r := range rows {
			if r.CaseID == args[1] || r.RunID == args[1] || r.AttemptID == args[1] {
				b, _ := json.MarshalIndent(r, "", "  ")
				fmt.Println(string(b))
				found = true
			}
		}
		if !found {
			return fmt.Errorf("no matching case, run, or attempt")
		}
		return nil
	case "memory-compare":
		if len(args) != 3 {
			return fmt.Errorf("memory-compare requires FROM TO conditions")
		}
		c := CompareMemoryRows(rows, MemoryCondition(args[1]), MemoryCondition(args[2]))
		fmt.Printf("%s -> %s paired=%d improved=%d regressed=%d unchanged=%d\n", c.From, c.To, c.Total, c.Improved, c.Regressed, c.Unchanged)
		return nil
	default:
		return fmt.Errorf("unknown memory artifact action")
	}
}
