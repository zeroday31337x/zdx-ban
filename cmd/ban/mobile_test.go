package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"zdx-ban/internal/ban"
)

func TestMobileProfileEndToEnd(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"test-model"}]}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/generate" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		calls = append(calls, req)
		mu.Unlock()
		prompt, _ := req["Prompt"].(string)
		var response string
		if strings.Contains(prompt, "exactly 2 semantic approaches") {
			response = `{"branches":[{"title":"Arithmetic","answer":"12","reasoning_summary":"add values","assumptions":[]},{"title":"Estimation","answer":"13","reasoning_summary":"estimate sum","assumptions":[]}]}`
		} else if strings.Contains(prompt, "Scores confidence") {
			response = `{"evidence":[],"contradictions":[],"confidence":0.7,"uncertainty":0.2,"evidence_score":0.5,"consistency_score":0.7,"contradiction_score":0.1,"feasibility_score":0.8,"risk_score":0.1,"utility_score":0.7,"diversity_score":0.5,"hard_constraint_violation":false,"hard_constraint_reason":""}`
		} else {
			http.Error(w, "unexpected model call", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(map[string]any{"response": response, "done": true, "prompt_eval_count": 12, "eval_count": 30})
	}))
	defer server.Close()

	raw, err := os.ReadFile("../../ban.config")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["model"].(map[string]any)["base_url"] = server.URL
	traces := t.TempDir()
	cfg["runtime"].(map[string]any)["trace_dir"] = traces
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "ban.config")
	if err := os.WriteFile(file, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	previous := os.Args
	os.Args = []string{"ban", "--config", file, "--mobile", "What is 7 plus 5? Return only the number."}
	defer func() { os.Args = previous }()
	t.Setenv("BAN_MODEL", "")
	t.Setenv("OLLAMA_BASE_URL", "")
	if err := run(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	captured := append([]map[string]any(nil), calls...)
	mu.Unlock()
	if len(captured) != 3 {
		t.Fatalf("mobile should make one proposal and two evaluation calls, got %d", len(captured))
	}
	for i, req := range captured {
		options, ok := req["options"].(map[string]any)
		if !ok {
			t.Fatalf("call %d has no options: %v", i, req)
		}
		limit, _ := options["num_predict"].(float64)
		if i == 0 && limit > 384 || i > 0 && limit > 192 {
			t.Fatalf("call %d token limit too high: %v", i, limit)
		}
	}
	files, err := filepath.Glob(filepath.Join(traces, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("trace files=%v err=%v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var trace ban.ExecutionTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.Config.MaxDepth != 0 || trace.Config.InitialBranches != 2 || trace.Config.GravityRecoveryBranches != 0 || trace.Metrics.ModelCalls != 3 {
		t.Fatalf("mobile profile was not applied: config=%+v metrics=%+v", trace.Config, trace.Metrics)
	}
	if trace.SelectedBranch == "" || trace.Result.Answer != "12" || trace.Metrics.Latency < 0*time.Second {
		t.Fatalf("mobile did not select expected branch: %+v", trace.Result)
	}
}
