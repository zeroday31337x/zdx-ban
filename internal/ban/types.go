package ban

import (
	"time"
	"zdx-ban/internal/measurement"
)

type Status string
type NodeKind string
type CapabilityInvocation struct {
	Capability, Why, ArtifactID string
	At                          time.Time
}

const (
	Proposed  Status = "PROPOSED"
	Evaluated Status = "EVALUATED"
	Active    Status = "ACTIVE"
	Expanded  Status = "EXPANDED"
	Pruned    Status = "PRUNED"
	Verified  Status = "VERIFIED"
	Failed    Status = "FAILED"
	Selected  Status = "SELECTED"
)
const (
	ReasoningCandidate  NodeKind = "REASONING_CANDIDATE"
	PredictionNode      NodeKind = "PREDICTION"
	ExecutionNode       NodeKind = "EXECUTION"
	ObservationNode     NodeKind = "OBSERVATION"
	MeasurementNode     NodeKind = "MEASUREMENT"
	MemoryRetrievalNode NodeKind = "MEMORY_RETRIEVAL"
	ToolResultNode      NodeKind = "TOOL_RESULT"
)

type VerificationResult struct {
	Verifier string    `json:"verifier"`
	Passed   bool      `json:"passed"`
	Details  string    `json:"details"`
	At       time.Time `json:"at"`
}
type Challenge struct {
	Skeptic        string `json:"skeptic"`
	Counterfactual string `json:"counterfactual"`
}
type State struct {
	ID                                                                                                                                                      string `json:"id"`
	ParentIDs, ChildIDs                                                                                                                                     []string
	Depth                                                                                                                                                   int    `json:"depth"`
	Title                                                                                                                                                   string `json:"title"`
	Hypothesis                                                                                                                                              string `json:"hypothesis"`
	ReasoningSummary                                                                                                                                        string `json:"reasoning_summary"`
	Assumptions, Evidence, Contradictions                                                                                                                   []string
	Confidence, Uncertainty, EvidenceScore, ConsistencyScore, ContradictionScore, FeasibilityScore, RiskScore, UtilityScore, DiversityScore, AggregateScore float64
	InformationGravity, GravityRepulsion                                                                                                                    float64
	GravityWellID                                                                                                                                           string
	Status                                                                                                                                                  Status `json:"status"`
	CreatedAt, UpdatedAt                                                                                                                                    time.Time
	TokenCost                                                                                                                                               int
	Latency                                                                                                                                                 time.Duration
	VerificationResults                                                                                                                                     []VerificationResult
	Measurements                                                                                                                                            []measurement.Result
	Challenge                                                                                                                                               Challenge              `json:"challenge"`
	Metadata                                                                                                                                                map[string]string      `json:"metadata,omitempty"`
	Kind                                                                                                                                                    NodeKind               `json:"kind,omitempty"`
	CapabilityInvocations                                                                                                                                   []CapabilityInvocation `json:"capability_invocations,omitempty"`
}
type Config struct{ InitialBranches, RetainBranches, MaxDepth, MaxNodes, MaxConcurrentModelCalls, MaxConcurrentEvaluations, MaxActiveBranches, GravityRecoveryBranches int }

func DefaultConfig() Config {
	return Config{InitialBranches: 5, RetainBranches: 2, MaxDepth: 2, MaxNodes: 20, MaxConcurrentModelCalls: 1, MaxConcurrentEvaluations: 1, MaxActiveBranches: 3, GravityRecoveryBranches: 2}
}

type Proposal struct {
	Title            string   `json:"title"`
	Hypothesis       string   `json:"hypothesis"`
	ReasoningSummary string   `json:"reasoning_summary"`
	Assumptions      []string `json:"assumptions"`
}
type Evaluation struct {
	Evidence                []string `json:"evidence"`
	Contradictions          []string `json:"contradictions"`
	Confidence              float64  `json:"confidence"`
	Uncertainty             float64  `json:"uncertainty"`
	EvidenceScore           float64  `json:"evidence_score"`
	ConsistencyScore        float64  `json:"consistency_score"`
	ContradictionScore      float64  `json:"contradiction_score"`
	FeasibilityScore        float64  `json:"feasibility_score"`
	RiskScore               float64  `json:"risk_score"`
	UtilityScore            float64  `json:"utility_score"`
	DiversityScore          float64  `json:"diversity_score"`
	HardConstraintViolation bool     `json:"hard_constraint_violation"`
	HardConstraintReason    string   `json:"hard_constraint_reason"`
}
type Result struct {
	Answer   string `json:"answer"`
	Selected *State `json:"selected"`
}
