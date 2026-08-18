package thought

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type CompilerCapability string

const (
	SemanticCompile CompilerCapability = "SEMANTIC_COMPILE"
	ModelRender     CompilerCapability = "MODEL_RENDER"
	OutputParse     CompilerCapability = "OUTPUT_PARSE"
	VisualRender    CompilerCapability = "VISUAL_RENDER"
	VMRender        CompilerCapability = "VM_RENDER"
)

type CompileInput struct{ ID, Goal, Text string }
type Rendered struct {
	MediaType string
	Data      []byte
}
type Compiler interface {
	Compile(context.Context, CompileInput) (IR, error)
	Render(context.Context, IR) (Rendered, error)
	Parse(context.Context, []byte) (IR, *Mutation, error)
	Capabilities() map[CompilerCapability]bool
}

// CanonicalCompiler is deterministic semantic plumbing. It does not claim to
// reproduce the external raster ThoughtCompiler algorithm.
type CanonicalCompiler struct{}

func (CanonicalCompiler) Compile(_ context.Context, in CompileInput) (IR, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = strings.TrimSpace(in.Text)
	}
	id := in.ID
	if id == "" {
		id = StableID(goal)
	}
	v := IR{Version: Version, ID: id, Goal: goal}
	return v, v.Validate()
}
func (CanonicalCompiler) Render(_ context.Context, v IR) (Rendered, error) {
	b, e := v.MarshalDeterministic()
	return Rendered{MediaType: "application/vnd.zdx.thoughtir+json", Data: b}, e
}
func (CanonicalCompiler) Parse(_ context.Context, b []byte) (IR, *Mutation, error) {
	v, e := Parse(b)
	if e == nil {
		return v, nil, nil
	}
	var m Mutation
	if json.Unmarshal(b, &m) == nil && m.Version == Version && m.ThoughtID != "" {
		return IR{}, &m, nil
	}
	return IR{}, nil, errors.New("model output is neither valid ThoughtIR v1 nor mutation")
}
func (CanonicalCompiler) Capabilities() map[CompilerCapability]bool {
	return map[CompilerCapability]bool{SemanticCompile: true, ModelRender: true, OutputParse: true}
}
