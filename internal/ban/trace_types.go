package ban

import (
	"time"
	"zdx-ban/internal/model"
	"zdx-ban/internal/telemetry"
)

const TraceSchemaVersion = "0.1"

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
	Edges                            []Edge
	InitialTopBranch, SelectedBranch string
	RecoveredFromWrongBranch         bool
	RecoveryDepth                    int
	ReasonForSwitch                  string
	Metrics                          Metrics
	Result                           Result
}
