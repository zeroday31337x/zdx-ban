// Package cognitive wires the measurable learning substrate without training.
package cognitive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/modelstate"
	"zdx-ban/internal/thought"
	"zdx-ban/internal/training"
	"zdx-ban/internal/vm"
)

type Capability string

const (
	Inference       Capability = "INFERENCE"
	Memory          Capability = "MEMORY"
	ThoughtCompiler Capability = "THOUGHT_COMPILER"
	VMExecution     Capability = "VM_EXECUTION"
	Tools           Capability = "TOOLS"
	Measurement     Capability = "MEASUREMENT"
)

type Event struct {
	Capability                     Capability
	Why, RunID, NodeID, ArtifactID string
	At                             time.Time
}
type Request struct {
	RunID, NodeID, Goal, Input string
	InitialState               json.RawMessage
	Seed                       *int
	ModelState                 modelstate.Manifest
	MemorySnapshotHash         string
}
type Result struct {
	RunID                                             string
	ThoughtBefore, ThoughtAfter                       thought.IR
	Inference                                         inference.Result
	Execution                                         vm.Result
	Measurement                                       measurement.Result
	MemoryRecordID, TrainingCandidateID, ModelStateID string
	Events                                            []Event
	Deterministic                                     bool
}
type Loop struct {
	Compiler   thought.Compiler
	Inference  inference.Engine
	VM         vm.Executor
	Memory     memory.Store
	Candidates training.Store
	Now        func() time.Time
}

func (l Loop) Run(ctx context.Context, r Request) (Result, error) {
	out := Result{RunID: r.RunID}
	now := l.Now
	if now == nil {
		now = time.Now
	}
	at := func() time.Time { return now().UTC() }
	emit := func(c Capability, why, id string) {
		out.Events = append(out.Events, Event{Capability: c, Why: why, RunID: r.RunID, NodeID: r.NodeID, ArtifactID: id, At: at()})
	}
	if r.RunID == "" || r.Goal == "" {
		return out, errors.New("cognitive run id and goal are required")
	}
	mid, e := r.ModelState.ID()
	if e != nil {
		return out, e
	}
	out.ModelStateID = mid
	before, e := l.Compiler.Compile(ctx, thought.CompileInput{Goal: r.Goal, Text: r.Input})
	if e != nil {
		return out, e
	}
	out.ThoughtBefore = before
	emit(ThoughtCompiler, "compile semantic input", before.ID)
	rendered, e := l.Compiler.Render(ctx, before)
	if e != nil {
		return out, e
	}
	var proposed thought.IR
	inf, e := inference.GenerateStructured(ctx, l.Inference, inference.Request{RequestID: r.RunID + "-inference", Goal: r.Goal, Prompt: "Return one complete ThoughtIR v1 JSON object for this goal. Use only available VM operations; never emit shell commands or implicit tool calls. Input ThoughtIR:\n" + string(rendered.Data), System: "Model output is an untrusted proposal. Emit JSON only; observations must not be fabricated.", ThoughtIR: rendered.Data, RequestedCapabilities: []inference.RequestedCapability{{Name: inference.StructuredGeneration}}, Seed: r.Seed, ModelStateRequirement: mid, Trace: map[string]string{"run_id": r.RunID, "node_id": r.NodeID}}, &proposed)
	out.Inference = inf
	emit(Inference, "obtain candidate prediction", inf.RequestID)
	if e != nil {
		return out, e
	}
	after, _, e := l.Compiler.Parse(ctx, []byte(inf.Output))
	if e != nil {
		return out, e
	}
	out.ThoughtAfter = after
	emit(ThoughtCompiler, "validate predicted ThoughtIR", after.ID)
	actions := make([]vm.Action, 0, len(after.CandidateActions))
	for _, a := range after.CandidateActions {
		actions = append(actions, vm.Action{Operation: a.Operation, Arguments: a.Parameters})
	}
	execID := stable("exec", r.RunID, r.NodeID, after.ID)
	vr, e := l.VM.Execute(ctx, vm.Request{ExecutionID: execID, RunID: r.RunID, NodeID: r.NodeID, ThoughtID: after.ID, InitialState: r.InitialState, Actions: actions, Seed: r.Seed, Bounds: vm.Bounds{Timeout: time.Second, MaxActions: 32, MaxOutputBytes: 1 << 20}})
	out.Execution = vr
	emit(VMExecution, "observe candidate action under bounded executor", execID)
	m := compare(r, after, vr, e, at())
	out.Measurement = m
	emit(Measurement, "compare prediction with execution observation", m.ID)
	memID := stable("memory", r.RunID, r.NodeID, m.ID)
	rec := memory.NewRecord(memID, memory.Episodic, memory.EpisodeKind, "prediction/execution observation", string(mustJSON(map[string]any{"prediction": after.PredictedEffects, "observation": vr.FinalState, "measurement": m, "execution_id": execID, "model_state_id": mid})), memory.Provenance{Source: "cognitive-loop-v1", TraceID: r.RunID, BranchID: r.NodeID, MeasurementIDs: []string{m.ID}, Authority: m.Authority, Independence: m.Independence, SourceClass: memory.CurrentMeasurement, CreatedAt: at()})
	if e = l.Memory.Append(ctx, rec); e != nil {
		return out, e
	}
	out.MemoryRecordID = memID
	emit(Memory, "persist explicit observation and provenance", memID)
	target := training.MemoryOnly
	state := training.Recorded
	if m.Outcome == measurement.Contradicted || m.Outcome == measurement.Error {
		target = training.Rejected
		state = training.PromotionRejected
		if m.Outcome == measurement.Contradicted { state = training.PromotionContradicted }
	}
	cid := training.StableID(r.RunID, r.NodeID, r.Input)
	c := training.Candidate{SchemaVersion: 1, ID: cid, Timestamp: at(), SourceRun: r.RunID, SourceGraphNode: r.NodeID, Input: r.Input, ThoughtBefore: &before, ModelOutput: inf.Output, ThoughtAfter: &after, MemoryAttribution: []string{memID}, VMExecutionID: execID, MeasurementOutcome: m.Outcome, Target: target, ValidationState: state, Gravity: training.Gravity{Value: 0, Source: "unmeasured"}, Provenance: training.Provenance{RunID: r.RunID, GraphNodeID: r.NodeID, ExecutionID: execID, MemorySnapshotHash: r.MemorySnapshotHash, ModelStateID: mid, MeasurementIDs: []string{m.ID}, MemoryIDs: []string{memID}}}
	if e = l.Candidates.Append(ctx, c); e != nil {
		return out, e
	}
	out.TrainingCandidateID = cid
	out.Deterministic = vr.Deterministic && r.Seed != nil
	return out, nil
}

func compare(r Request, t thought.IR, v vm.Result, execErr error, now time.Time) measurement.Result {
	m := measurement.Result{ID: stable("measurement", r.RunID, r.NodeID, v.ExecutionID), ContractID: "prediction-observation-v1", Claim: "predicted final state matches bounded execution observation", VerificationClass: measurement.ExecutionVerified, Method: "canonical JSON equality", Authority: measurement.DeterministicRuntime, Independence: measurement.Independent, Repeatable: v.Deterministic, Repeatability: measurement.Deterministic, StartedAt: now, FinishedAt: now}
	if execErr != nil {
		m.Outcome = measurement.Error
		m.Error = &measurement.MeasurementError{Code: "VM_EXECUTION_FAILED", Message: execErr.Error()}
		return m
	}
	if len(t.PredictedEffects) == 0 {
		m.Outcome = measurement.Inconclusive
		return m
	}
	expected := json.RawMessage(t.PredictedEffects[0].Effect)
	m.Expected = expected
	m.Observation = v.FinalState
	if !json.Valid(expected) || !json.Valid(v.FinalState) {
		m.Outcome = measurement.Inconclusive
		return m
	}
	var a, b any
	_ = json.Unmarshal(expected, &a)
	_ = json.Unmarshal(v.FinalState, &b)
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ab) == string(bb) {
		m.Outcome = measurement.Supported
	} else {
		m.Outcome = measurement.Contradicted
	}
	return m
}
func stable(prefix string, parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(h.Sum(nil)[:10])
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
