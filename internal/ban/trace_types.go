package ban

import (
	"context"
	"time"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/telemetry"
)

const TraceSchemaVersion = "0.3"

type MemoryInteraction struct {
	Enabled             bool
	InitialSnapshotHash string
	RetrievedMemoryIDs  []string
	RetrievalReasons    map[string]any
	WorkingMemoryChars  int
	GuidanceHash        string
	Events              []map[string]any
	GravityWells        []GravityWell
}
type GravityWell struct {
	ID, Category        string
	Strength, Repulsion float64
	DecayHalfLife       time.Duration
	LastUsedAt          time.Time
	SuccessRate         float64
	ApplicationCount    int
	OutcomeCount        int
	SupportingEvidence  int
	FoundationAnchor    bool
	Keywords            []string
}

// CapabilityExecutor computes a candidate from a strategy using a bounded,
// deterministic capability. Its output is still accepted only by Verifier.
type CapabilityExecutor interface {
	Execute(context.Context, string, string) (string, error)
}
type Metrics struct {
	TotalNodes, ExpandedNodes, PrunedNodes, ModelCalls, Tokens, PeakActiveBranches                          int
	CandidateProposals, DuplicateProposals, AnswerConvergences, DiversityRegenerations, ProviderEvaluations int
	GravityRoutedBranches, GravityWellHits, GravityRecoveryAttempts, GravityRecoveries                      int
	Latency                                                                                                 time.Duration
	DidRecover                                                                                              bool
}
type ExecutionTrace struct {
	TraceSchemaVersion               string `json:"trace_schema_version"`
	RunID, Problem                   string
	Config                           Config
	Model                            inference.ModelInfo
	StartedAt, FinishedAt            time.Time
	RuntimeStart, RuntimeFinish      telemetry.Snapshot
	Nodes                            []*State
	MeasurementEvents                []measurement.Result
	Edges                            []Edge
	InitialTopBranch, SelectedBranch string
	RecoveredFromWrongBranch         bool
	RecoveryDepth                    int
	ReasonForSwitch                  string
	Metrics                          Metrics
	Memory                           MemoryInteraction
	Result                           Result
}
