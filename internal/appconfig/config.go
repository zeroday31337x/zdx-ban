// Package appconfig loads the strict project-level ban.config file.
package appconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zdx-ban/internal/ban"
)

const SchemaVersion = 1

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("duration must be a string such as 10s, 5m, or 1h")
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

type ModelConfig struct {
	Provider         string   `json:"provider"`
	Name             string   `json:"name"`
	FoundationID     string   `json:"foundation_id"`
	BaseURL          string   `json:"base_url"`
	ConnectTimeout   Duration `json:"connect_timeout"`
	Retries          int      `json:"retries"`
	MaxResponseBytes int      `json:"max_response_bytes"`
}

type CallConfig struct {
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type GenerationConfig struct {
	Baseline   CallConfig `json:"baseline"`
	Proposal   CallConfig `json:"proposal"`
	Evaluation CallConfig `json:"evaluation"`
	Challenge  CallConfig `json:"challenge"`
	Final      CallConfig `json:"final"`
}

type SearchConfig struct {
	InitialBranches          int `json:"initial_branches"`
	RetainBranches           int `json:"retain_branches"`
	MaxDepth                 int `json:"max_depth"`
	MaxNodes                 int `json:"max_nodes"`
	MaxConcurrentModelCalls  int `json:"max_concurrent_model_calls"`
	MaxConcurrentEvaluations int `json:"max_concurrent_evaluations"`
	MaxActiveBranches        int `json:"max_active_branches"`
	GravityRecoveryBranches  int `json:"gravity_recovery_branches"`
}

type RuntimeConfig struct {
	RunTimeout       Duration `json:"run_timeout"`
	TraceDir         string   `json:"trace_dir"`
	BenchmarkDataset string   `json:"benchmark_dataset"`
	MemoryFile       string   `json:"memory_file"`
}

type ExperimentConfig struct {
	Temperature         float64  `json:"temperature"`
	MemoryTemperature   float64  `json:"memory_temperature"`
	MaxTokens           int      `json:"max_tokens"`
	ProposalMaxTokens   int      `json:"proposal_max_tokens"`
	EvaluationMaxTokens int      `json:"evaluation_max_tokens"`
	ChallengeMaxTokens  int      `json:"challenge_max_tokens"`
	FinalMaxTokens      int      `json:"final_max_tokens"`
	Timeout             Duration `json:"timeout"`
	InferenceTimeout    Duration `json:"inference_timeout"`
	CaseTimeout         Duration `json:"case_timeout"`
	RunTimeout          Duration `json:"run_timeout"`
	Repetitions         int      `json:"repetitions"`
	Seed                int      `json:"seed"`
}

type Config struct {
	SchemaVersion int              `json:"schema_version"`
	Model         ModelConfig      `json:"model"`
	Generation    GenerationConfig `json:"generation"`
	Search        SearchConfig     `json:"search"`
	Runtime       RuntimeConfig    `json:"runtime"`
	Experiment    ExperimentConfig `json:"experiment"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read BAN config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config Config
	if err = decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode BAN config %s: %w", path, err)
	}
	if trailingErr := decoder.Decode(&struct{}{}); !errors.Is(trailingErr, io.EOF) {
		if trailingErr == nil {
			return Config{}, fmt.Errorf("decode BAN config %s: multiple JSON values", path)
		}
		return Config{}, fmt.Errorf("decode BAN config %s: trailing data: %w", path, trailingErr)
	}
	if err = config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate BAN config %s: %w", path, err)
	}
	return config, nil
}

func (c *Config) ApplyEnvironment(getenv func(string) string) error {
	if value := getenv("BAN_MODEL"); value != "" {
		c.Model.Name = value
	}
	if value := getenv("OLLAMA_BASE_URL"); value != "" {
		c.Model.BaseURL = value
	}
	if value := getenv("BAN_MEMORY_FILE"); value != "" {
		c.Runtime.MemoryFile = value
	}
	return c.Validate()
}

func (c Config) BANConfig() ban.Config {
	return ban.Config{
		InitialBranches:          c.Search.InitialBranches,
		RetainBranches:           c.Search.RetainBranches,
		MaxDepth:                 c.Search.MaxDepth,
		MaxNodes:                 c.Search.MaxNodes,
		MaxConcurrentModelCalls:  c.Search.MaxConcurrentModelCalls,
		MaxConcurrentEvaluations: c.Search.MaxConcurrentEvaluations,
		MaxActiveBranches:        c.Search.MaxActiveBranches,
		GravityRecoveryBranches:  c.Search.GravityRecoveryBranches,
	}
}

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", c.SchemaVersion)
	}
	if c.Model.Provider != "ollama" {
		return fmt.Errorf("model.provider must be ollama; got %q", c.Model.Provider)
	}
	for name, value := range map[string]string{
		"model.name":          c.Model.Name,
		"model.foundation_id": c.Model.FoundationID,
		"runtime.trace_dir":   c.Runtime.TraceDir,
		"runtime.dataset":     c.Runtime.BenchmarkDataset,
		"runtime.memory_file": c.Runtime.MemoryFile,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be empty", name)
		}
	}
	parsed, err := url.Parse(c.Model.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("model.base_url must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("model.base_url may not contain credentials, a query, or a fragment")
	}
	if c.Model.ConnectTimeout.Duration <= 0 {
		return fmt.Errorf("model.connect_timeout must be positive")
	}
	if c.Model.Retries < 0 || c.Model.Retries > 10 {
		return fmt.Errorf("model.retries must be between 0 and 10")
	}
	if c.Model.MaxResponseBytes < 1024 || c.Model.MaxResponseBytes > 64<<20 {
		return fmt.Errorf("model.max_response_bytes must be between 1024 and 67108864")
	}
	for name, call := range map[string]CallConfig{
		"baseline": c.Generation.Baseline, "proposal": c.Generation.Proposal,
		"evaluation": c.Generation.Evaluation, "challenge": c.Generation.Challenge,
		"final": c.Generation.Final,
	} {
		if err = validateCall("generation."+name, call); err != nil {
			return err
		}
	}
	if c.Search.InitialBranches < 1 || c.Search.RetainBranches < 1 || c.Search.MaxDepth < 1 ||
		c.Search.MaxNodes < 1 || c.Search.MaxConcurrentModelCalls < 1 ||
		c.Search.MaxConcurrentEvaluations < 1 || c.Search.MaxActiveBranches < 1 ||
		c.Search.GravityRecoveryBranches < 0 {
		return fmt.Errorf("search counts must be positive except gravity_recovery_branches, which may be zero")
	}
	if c.Search.RetainBranches > c.Search.InitialBranches {
		return fmt.Errorf("search.retain_branches may not exceed initial_branches")
	}
	if c.Search.MaxActiveBranches < c.Search.RetainBranches {
		return fmt.Errorf("search.max_active_branches may not be less than retain_branches")
	}
	if c.Search.MaxNodes < c.Search.InitialBranches {
		return fmt.Errorf("search.max_nodes may not be less than initial_branches")
	}
	if c.Runtime.RunTimeout.Duration <= 0 {
		return fmt.Errorf("runtime.run_timeout must be positive")
	}
	if filepath.IsAbs(c.Runtime.TraceDir) && filepath.Clean(c.Runtime.TraceDir) == string(filepath.Separator) {
		return fmt.Errorf("runtime.trace_dir may not be the filesystem root")
	}
	if err = validateExperiment(c.Experiment); err != nil {
		return err
	}
	return nil
}

func validateCall(name string, call CallConfig) error {
	if call.Temperature < 0 || call.Temperature > 2 {
		return fmt.Errorf("%s.temperature must be between 0 and 2", name)
	}
	if call.MaxTokens < 1 || call.MaxTokens > 1_000_000 {
		return fmt.Errorf("%s.max_tokens must be between 1 and 1000000", name)
	}
	return nil
}

func validateExperiment(config ExperimentConfig) error {
	if config.Temperature < 0 || config.Temperature > 2 || config.MemoryTemperature < 0 || config.MemoryTemperature > 2 {
		return fmt.Errorf("experiment temperatures must be between 0 and 2")
	}
	for name, value := range map[string]int{
		"max_tokens": config.MaxTokens, "proposal_max_tokens": config.ProposalMaxTokens,
		"evaluation_max_tokens": config.EvaluationMaxTokens, "challenge_max_tokens": config.ChallengeMaxTokens,
		"final_max_tokens": config.FinalMaxTokens, "repetitions": config.Repetitions,
	} {
		if value < 1 {
			return fmt.Errorf("experiment.%s must be positive", name)
		}
	}
	for name, value := range map[string]time.Duration{
		"timeout": config.Timeout.Duration, "inference_timeout": config.InferenceTimeout.Duration,
		"case_timeout": config.CaseTimeout.Duration, "run_timeout": config.RunTimeout.Duration,
	} {
		if value <= 0 {
			return fmt.Errorf("experiment.%s must be positive", name)
		}
	}
	return nil
}
