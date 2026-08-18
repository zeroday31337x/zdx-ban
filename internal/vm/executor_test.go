package vm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestFakeExecutorDeterministicAndContained(t *testing.T) {
	f := FakeExecutor{Allowed: map[string]bool{"SET_STATE": true}}
	r := Request{ExecutionID: "e", InitialState: json.RawMessage(`{"x":0}`), Actions: []Action{{Operation: "SET_STATE", Arguments: json.RawMessage(`{"x":1}`)}}, Bounds: Bounds{MaxActions: 1, MaxOutputBytes: 100}}
	a, e := f.Execute(context.Background(), r)
	if e != nil || string(a.FinalState) != `{"x":1}` || !a.Deterministic {
		t.Fatalf("bad execution: %+v %v", a, e)
	}
	r.Bounds.AllowNetwork = true
	if _, e = f.Execute(context.Background(), r); !errors.Is(e, ErrUnsupportedAction) {
		t.Fatal("network request was not rejected")
	}
}
func TestFakeExecutorFailures(t *testing.T) {
	f := FakeExecutor{Allowed: map[string]bool{}}
	if _, e := f.Execute(context.Background(), Request{Actions: []Action{{Operation: "SHELL"}}}); !errors.Is(e, ErrUnsupportedAction) {
		t.Fatal("shell action accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := f.Execute(ctx, Request{}); !errors.Is(e, ErrTimeout) {
		t.Fatalf("canceled execution: %v", e)
	}
}
