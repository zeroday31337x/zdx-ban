// Package inference defines the provider-independent neural inference boundary.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type FailureCode string

const (
	ProviderConnectionError FailureCode = "PROVIDER_CONNECTION_ERROR"
	ProviderTimeout         FailureCode = "PROVIDER_TIMEOUT"
	ProviderCancelled       FailureCode = "PROVIDER_CANCELLED"
	ProviderResponseError   FailureCode = "PROVIDER_RESPONSE_ERROR"
	ModelOutputMalformed    FailureCode = "MODEL_OUTPUT_MALFORMED"
	VerificationError       FailureCode = "VERIFICATION_ERROR"
	ExecutionError          FailureCode = "EXECUTION_ERROR"
)

type Failure struct {
	Code FailureCode
	Err  error
}

func (e *Failure) Error() string { return fmt.Sprintf("%s: %v", e.Code, e.Err) }
func (e *Failure) Unwrap() error { return e.Err }
func NewFailure(code FailureCode, err error) error {
	if err == nil {
		return nil
	}
	var existing *Failure
	if errors.As(err, &existing) {
		return err
	}
	return &Failure{Code: code, Err: err}
}
func FailureCodeOf(err error) FailureCode {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProviderTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ProviderCancelled
	}
	return ExecutionError
}

type CallTelemetry struct {
	StartedAt, RequestInitiatedAt, HeadersReceivedAt, FirstContentAt, CompletedAt time.Time
	FailedAt, TimedOutAt, CancelledAt                                             time.Time
	Duration                                                                      time.Duration
	StreamOpened, StreamProgressed, StreamCompleted                               bool
	Chunks                                                                        int
}

type Capability string

const (
	TextGeneration       Capability = "TEXT_GENERATION"
	StructuredGeneration Capability = "STRUCTURED_GENERATION"
	SeededGeneration     Capability = "SEEDED_GENERATION"
	TokenUsage           Capability = "TOKEN_USAGE"
	ModelInspection      Capability = "MODEL_INSPECTION"
)

type Capabilities map[Capability]bool

func (c Capabilities) Supports(v Capability) bool { return c[v] }

type RequestedCapability struct {
	Name     Capability `json:"name"`
	Required bool       `json:"required"`
}

type Tool struct {
	Name, Description string
	InputSchema       json.RawMessage `json:"input_schema,omitempty"`
}

type Constraints struct {
	Temperature float64  `json:"temperature"`
	Seed        *int     `json:"seed,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

type Request struct {
	RequestID             string                `json:"request_id,omitempty"`
	Goal                  string                `json:"goal,omitempty"`
	Prompt                string                `json:"prompt"`
	System                string                `json:"system,omitempty"`
	ThoughtIR             json.RawMessage       `json:"thought_ir,omitempty"`
	MemoryContext         []string              `json:"memory_context,omitempty"`
	RequestedCapabilities []RequestedCapability `json:"requested_capabilities,omitempty"`
	AvailableTools        []Tool                `json:"available_tools,omitempty"`
	Constraints           Constraints           `json:"constraints"`
	Temperature           float64               `json:"temperature,omitempty"`
	Seed                  *int                  `json:"seed,omitempty"`
	MaxTokens             int                   `json:"max_tokens,omitempty"`
	ModelStateRequirement string                `json:"model_state_requirement,omitempty"`
	Trace                 map[string]string     `json:"trace,omitempty"`
}

type ToolCall struct {
	ID, Name  string
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type Usage struct {
	PromptTokens, CompletionTokens int
	Latency                        time.Duration
	Resource                       map[string]float64 `json:"resource,omitempty"`
}

type Result struct {
	RequestID        string
	Output           string
	Structured       json.RawMessage `json:"structured,omitempty"`
	CandidateActions []string        `json:"candidate_actions,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	StopReason       string
	Usage            Usage
	ModelStateIDs    []string
	Text             string        `json:"text,omitempty"`
	PromptTokens     int           `json:"prompt_tokens,omitempty"`
	CompletionTokens int           `json:"completion_tokens,omitempty"`
	Latency          time.Duration `json:"latency,omitempty"`
	Call             CallTelemetry `json:"call,omitempty"`
}

type Health struct {
	Available bool
	Detail    string
}

type ModelInfo struct {
	Provider, Model, Version string
	IdentityConfidence       string `json:"identity_confidence,omitempty"`
	IdentitySource           string `json:"identity_source,omitempty"`
}

type Engine interface {
	Generate(context.Context, Request) (Result, error)
}

type StructuredEngine interface {
	GenerateStructured(context.Context, Request, any) (Result, error)
}
type CapabilityReporter interface {
	Capabilities(context.Context) Capabilities
}

func GenerateStructured(ctx context.Context, e Engine, r Request, dst any) (Result, error) {
	s, ok := e.(StructuredEngine)
	if !ok {
		return Result{}, ErrUnsupported
	}
	if c, ok := e.(CapabilityReporter); ok && !c.Capabilities(ctx).Supports(StructuredGeneration) {
		return Result{}, ErrUnsupported
	}
	return s.GenerateStructured(ctx, r, dst)
}

type HealthReporter interface {
	Health(context.Context) (Health, error)
}
type ModelStateReporter interface {
	ModelState(context.Context) (ModelInfo, error)
}

var (
	ErrUnavailable   = errors.New("inference backend unavailable")
	ErrUnsupported   = errors.New("inference capability unsupported")
	ErrInvalidOutput = errors.New("invalid inference output")
)

func DecodeStrict(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}
