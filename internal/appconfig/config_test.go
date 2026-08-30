package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func projectConfig(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "ban.config"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProjectConfigLoadsAndMapsBANSearch(t *testing.T) {
	config, err := Load(projectConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if config.Model.Name != "qwen2.5:1.5b" || config.Model.ConnectTimeout.Duration != 10*time.Second {
		t.Fatalf("unexpected model config: %+v", config.Model)
	}
	search := config.BANConfig()
	if search.InitialBranches != 5 || search.MaxConcurrentModelCalls != 1 {
		t.Fatalf("unexpected search config: %+v", search)
	}
}

func TestUnknownFieldAndUnsafeValuesAreRejected(t *testing.T) {
	data, err := os.ReadFile(projectConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 1, "surprise": true`, 1)
	path := filepath.Join(t.TempDir(), "unknown.config")
	if err = os.WriteFile(path, []byte(unknown), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}

	badURL := strings.Replace(string(data), "http://127.0.0.1:11434", "file:///tmp/model", 1)
	path = filepath.Join(t.TempDir(), "bad-url.config")
	if err = os.WriteFile(path, []byte(badURL), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil || !strings.Contains(err.Error(), "HTTP(S)") {
		t.Fatalf("expected URL validation error, got %v", err)
	}
}

func TestTrailingDataIsRejected(t *testing.T) {
	data, err := os.ReadFile(projectConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trailing.config")
	if err = os.WriteFile(path, append(data, []byte(" garbage")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("expected trailing data error, got %v", err)
	}
}

func TestEnvironmentOverridesAreValidated(t *testing.T) {
	config, err := Load(projectConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	values := stringMap{
		"BAN_MODEL":       "local-w0:latest",
		"OLLAMA_BASE_URL": "http://10.0.0.2:11434",
		"BAN_MEMORY_FILE": "state/memory.jsonl",
	}
	if err = config.ApplyEnvironment(values.get); err != nil {
		t.Fatal(err)
	}
	if config.Model.Name != values["BAN_MODEL"] || config.Runtime.MemoryFile != values["BAN_MEMORY_FILE"] {
		t.Fatalf("environment overrides not applied: %+v", config)
	}
	values["OLLAMA_BASE_URL"] = "not-a-url"
	if err = config.ApplyEnvironment(values.get); err == nil {
		t.Fatal("expected invalid environment override to fail")
	}
}

type stringMap map[string]string

func (m stringMap) get(key string) string { return m[key] }
