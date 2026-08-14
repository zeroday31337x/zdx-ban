package ban

import (
	"time"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/model"
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
}
type Metrics struct {
	TotalNodes, ExpandedNodes, PrunedNodes, ModelCalls, Tokens, PeakActiveBranches int
	Latency                                                                        time.Duration
	DidRecover                                                                     bool
}
type ExecutionTrace struct {
	TraceSchemaVersion               string `json:"trace_schema_version"`
	RunID, Problem                   string
	Config                           Config
	Model                            model.Info
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
