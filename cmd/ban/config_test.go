package main

import (
	"path/filepath"
	"testing"

	"zdx-ban/internal/appconfig"
	"zdx-ban/internal/ban"
)

func rootConfig(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "ban.config"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppConfigPrecedenceAndArguments(t *testing.T) {
	t.Setenv("BAN_CONFIG", "/does/not/exist")
	t.Setenv("BAN_MODEL", "test-model")
	t.Setenv("OLLAMA_BASE_URL", "http://127.0.0.1:9999")
	config, args, err := loadAppConfig([]string{"--config", rootConfig(t), "model", "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Model.Name != "test-model" || config.Model.BaseURL != "http://127.0.0.1:9999" {
		t.Fatalf("environment did not override file: %+v", config.Model)
	}
	if len(args) != 2 || args[0] != "model" || args[1] != "inspect" {
		t.Fatalf("unexpected remaining arguments: %v", args)
	}
}

func TestLoadAppConfigRejectsEmptyPath(t *testing.T) {
	if _, _, err := loadAppConfig([]string{"--config="}); err == nil {
		t.Fatal("expected empty config path to fail")
	}
}

func TestProviderAndEngineUseFileSettings(t *testing.T) {
	config, err := appconfig.Load(rootConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	provider := newOllama(config)
	if provider.Model != config.Model.Name || provider.Retries != config.Model.Retries || provider.MaxBytes != config.Model.MaxResponseBytes {
		t.Fatalf("provider was not configured: %+v", provider)
	}
	engine := ban.NewEngine(provider, config.BANConfig())
	configureEngine(engine, config)
	if engine.Generator.MaxTokens != config.Generation.Proposal.MaxTokens ||
		engine.Evaluator.Temperature != config.Generation.Evaluation.Temperature ||
		engine.TraceDir != config.Runtime.TraceDir {
		t.Fatalf("engine was not configured: %+v", engine)
	}
}
