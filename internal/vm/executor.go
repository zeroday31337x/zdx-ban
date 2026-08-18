// Package vm defines contained execution/simulation observations.
package vm

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Capability string

const (
	Execute  Capability = "EXECUTE"
	Simulate Capability = "SIMULATE"
	Replay   Capability = "REPLAY"
)

type Action struct {
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}
type Bounds struct {
	Timeout             time.Duration `json:"timeout"`
	MaxActions          int           `json:"max_actions"`
	MaxOutputBytes      int           `json:"max_output_bytes"`
	AllowNetwork        bool          `json:"allow_network"`
	AllowHostFilesystem bool          `json:"allow_host_filesystem"`
}
type Request struct {
	ExecutionID, RunID, NodeID, HypothesisID, ThoughtID string
	InitialState                                        json.RawMessage
	Actions                                             []Action
	Bounds                                              Bounds
	Seed                                                *int
}
type Observation struct {
	Kind   string
	Value  json.RawMessage
	Source string
}
type ResourceUsage struct {
	Duration    time.Duration
	Actions     int
	OutputBytes int
}
type Result struct {
	ExecutionID   string
	InitialState  json.RawMessage
	Actions       []Action
	FinalState    json.RawMessage
	Observations  []Observation
	Errors        []string
	Usage         ResourceUsage
	Deterministic bool
	ReplayToken   string
	Environment   string
}
type Executor interface {
	Execute(context.Context, Request) (Result, error)
	Simulate(context.Context, Request) (Result, error)
	Capabilities() map[Capability]bool
}

var (
	ErrUnsupportedAction = errors.New("unsupported VM action")
	ErrResourceLimit     = errors.New("VM resource limit exceeded")
	ErrTimeout           = errors.New("VM execution timeout")
)

// FakeExecutor is a deterministic, non-host-executing test/smoke VM.
type FakeExecutor struct{ Allowed map[string]bool }

func (f FakeExecutor) Capabilities() map[Capability]bool {
	return map[Capability]bool{Execute: true, Simulate: true, Replay: true}
}
func (f FakeExecutor) Execute(ctx context.Context, r Request) (Result, error)  { return f.run(ctx, r) }
func (f FakeExecutor) Simulate(ctx context.Context, r Request) (Result, error) { return f.run(ctx, r) }
func (f FakeExecutor) run(ctx context.Context, r Request) (Result, error) {
	start := time.Now()
	out := Result{ExecutionID: r.ExecutionID, InitialState: r.InitialState, Actions: r.Actions, Deterministic: true, Environment: "bounded-fake-v1"}
	if r.Bounds.AllowNetwork || r.Bounds.AllowHostFilesystem {
		return out, ErrUnsupportedAction
	}
	if r.Bounds.MaxActions > 0 && len(r.Actions) > r.Bounds.MaxActions {
		return out, ErrResourceLimit
	}
	select {
	case <-ctx.Done():
		return out, ErrTimeout
	default:
	}
	state := append(json.RawMessage(nil), r.InitialState...)
	for _, a := range r.Actions {
		if !f.Allowed[a.Operation] {
			return out, ErrUnsupportedAction
		}
		if a.Operation == "SET_STATE" {
			if !json.Valid(a.Arguments) {
				return out, errors.New("invalid VM action arguments")
			}
			state = append(json.RawMessage(nil), a.Arguments...)
		}
	}
	out.FinalState = state
	out.Observations = []Observation{{Kind: "STATE", Value: state, Source: out.Environment}}
	out.Usage = ResourceUsage{Duration: time.Since(start), Actions: len(r.Actions), OutputBytes: len(state)}
	if r.Bounds.MaxOutputBytes > 0 && len(state) > r.Bounds.MaxOutputBytes {
		return out, ErrResourceLimit
	}
	out.ReplayToken = r.ExecutionID
	return out, nil
}
