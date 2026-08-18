package cognitive

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/modelstate"
	"zdx-ban/internal/thought"
	"zdx-ban/internal/training"
	"zdx-ban/internal/vm"
)

type fakeInference struct{ output string }

func (f fakeInference) Generate(context.Context, inference.Request) (inference.Result, error) {
	return inference.Result{RequestID: "run-inference", Output: f.output, Text: f.output}, nil
}
func (f fakeInference) GenerateStructured(c context.Context, r inference.Request, v any) (inference.Result, error) {
	return f.Generate(c, r)
}
func ir(effect string, action string) string {
	v := thought.IR{Version: thought.Version, ID: "prediction", Goal: "set x", CandidateActions: []thought.Action{{ID: "a", Operation: action, Parameters: json.RawMessage(`{"x":1}`)}}}
	if effect != "" {
		v.PredictedEffects = []thought.PredictedEffect{{ActionID: "a", Effect: effect}}
	}
	b, _ := v.MarshalDeterministic()
	return string(b)
}
func run(t *testing.T, output string) (Result, *memory.MemoryStore, *training.MemoryStore, error) {
	t.Helper()
	ms := memory.NewMemoryStore()
	ts := &training.MemoryStore{}
	seed := 7
	l := Loop{Compiler: thought.CanonicalCompiler{}, Inference: fakeInference{output}, VM: vm.FakeExecutor{Allowed: map[string]bool{"SET_STATE": true}}, Memory: ms, Candidates: ts, Now: func() time.Time { return time.Unix(10, 0) }}
	r, e := l.Run(context.Background(), Request{RunID: "run", NodeID: "node", Goal: "set x", Input: "set x", InitialState: json.RawMessage(`{"x":0}`), Seed: &seed, ModelState: modelstate.DeclaredW0("qwen2.5:1.5b", "bin/ban")})
	return r, ms, ts, e
}
func TestFullSmokeProvenanceSupported(t *testing.T) {
	r, ms, ts, e := run(t, ir(`{"x":1}`, "SET_STATE"))
	if e != nil {
		t.Fatal(e)
	}
	if r.Measurement.Outcome != measurement.Supported || !r.Deterministic || len(r.Events) != 6 {
		t.Fatalf("bad loop: %+v", r)
	}
	records, _ := ms.List(context.Background())
	if len(records) != 1 || records[0].Provenance.TraceID != "run" || len(ts.Items) != 1 || ts.Items[0].VMExecutionID != r.Execution.ExecutionID || ts.Items[0].Target != training.MemoryOnly {
		t.Fatal("attribution chain incomplete")
	}
}
func TestLoopContradictedInconclusiveAndFailed(t *testing.T) {
	r, _, ts, e := run(t, ir(`{"x":2}`, "SET_STATE"))
	if e != nil || r.Measurement.Outcome != measurement.Contradicted || ts.Items[0].Target != training.Rejected {
		t.Fatal("contradiction not recorded")
	}
	r, _, _, e = run(t, ir("", "SET_STATE"))
	if e != nil || r.Measurement.Outcome != measurement.Inconclusive {
		t.Fatal("inconclusive not recorded")
	}
	r, _, ts, e = run(t, ir(`{"x":1}`, "SHELL"))
	if e != nil || r.Measurement.Outcome != measurement.Error || ts.Items[0].Target != training.Rejected {
		t.Fatal("unsupported action was not classified and rejected")
	}
}
