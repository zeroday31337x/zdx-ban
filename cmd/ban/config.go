package main

import (
	"fmt"
	"os"
	"strings"

	"zdx-ban/internal/appconfig"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/experiment"
	"zdx-ban/internal/inference/ollama"
)

func loadAppConfig(args []string) (appconfig.Config, []string, error) {
	path := os.Getenv("BAN_CONFIG")
	if path == "" {
		path = "ban.config"
	}
	remaining := args
	if len(remaining) > 0 {
		switch {
		case remaining[0] == "--config":
			if len(remaining) < 2 || strings.TrimSpace(remaining[1]) == "" {
				return appconfig.Config{}, nil, fmt.Errorf("--config requires a path")
			}
			path = remaining[1]
			remaining = remaining[2:]
		case strings.HasPrefix(remaining[0], "--config="):
			path = strings.TrimPrefix(remaining[0], "--config=")
			if strings.TrimSpace(path) == "" {
				return appconfig.Config{}, nil, fmt.Errorf("--config requires a path")
			}
			remaining = remaining[1:]
		}
	}
	config, err := appconfig.Load(path)
	if err != nil {
		return appconfig.Config{}, nil, err
	}
	if err = config.ApplyEnvironment(os.Getenv); err != nil {
		return appconfig.Config{}, nil, fmt.Errorf("apply BAN environment overrides: %w", err)
	}
	return config, remaining, nil
}

func newOllama(config appconfig.Config) *ollama.Engine {
	provider := ollama.New(config.Model.BaseURL, config.Model.Name, config.Model.ConnectTimeout.Duration)
	provider.Retries = config.Model.Retries
	provider.MaxBytes = config.Model.MaxResponseBytes
	return provider
}

func configureEngine(engine *ban.Engine, config appconfig.Config) {
	engine.Generator.Temperature = config.Generation.Proposal.Temperature
	engine.Generator.MaxTokens = config.Generation.Proposal.MaxTokens
	engine.Evaluator.Temperature = config.Generation.Evaluation.Temperature
	engine.Evaluator.MaxTokens = config.Generation.Evaluation.MaxTokens
	engine.Evaluator.ChallengeMaxTokens = config.Generation.Challenge.MaxTokens
	engine.Temperature = config.Generation.Final.Temperature
	engine.MaxTokens = config.Generation.Final.MaxTokens
	engine.TraceDir = config.Runtime.TraceDir
}

func experimentDefaults(config appconfig.Config) experiment.CommandDefaults {
	return experiment.CommandDefaults{
		Provider:             config.Model.Provider,
		FoundationID:         config.Model.FoundationID,
		Temperature:          config.Experiment.Temperature,
		MemoryTemperature:    config.Experiment.MemoryTemperature,
		MaxTokens:            config.Experiment.MaxTokens,
		ProposalMaxTokens:    config.Experiment.ProposalMaxTokens,
		EvaluationMaxTokens:  config.Experiment.EvaluationMaxTokens,
		ChallengeMaxTokens:   config.Experiment.ChallengeMaxTokens,
		FinalAnswerMaxTokens: config.Experiment.FinalMaxTokens,
		Timeout:              config.Experiment.Timeout.Duration,
		InferenceTimeout:     config.Experiment.InferenceTimeout.Duration,
		CaseTimeout:          config.Experiment.CaseTimeout.Duration,
		RunTimeout:           config.Experiment.RunTimeout.Duration,
		Repetitions:          config.Experiment.Repetitions,
		Seed:                 config.Experiment.Seed,
		BAN:                  config.BANConfig(),
	}
}
