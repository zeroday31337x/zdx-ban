package ban

import (
	"time"
	"zdx-ban/internal/measurement"
)

type Status string

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
	Status                                                                                                                                                  Status `json:"status"`
	CreatedAt, UpdatedAt                                                                                                                                    time.Time
	TokenCost                                                                                                                                               int
	Latency                                                                                                                                                 time.Duration
	VerificationResults                                                                                                                                     []VerificationResult
	Measurements                                                                                                                                            []measurement.Result
	Challenge                                                                                                                                               Challenge         `json:"challenge"`
	Metadata                                                                                                                                                map[string]string `json:"metadata,omitempty"`
}
type Config struct{ InitialBranches, RetainBranches, MaxDepth, MaxNodes, MaxConcurrentModelCalls, MaxConcurrentEvaluations, MaxActiveBranches int }

func DefaultConfig() Config { return Config{5, 2, 2, 20, 1, 1, 3} }

type Proposal struct {
	Title            string   `json:"title"`
	Hypothesis       string   `json:"hypothesis"`
	ReasoningSummary string   `json:"reasoning_summary"`
	Assumptions      []string `json:"assumptions"`
}
type Evaluation struct {
	Evidence                                                                                                                                []string `json:"evidence"`
	Contradictions                                                                                                                          []string `json:"contradictions"`
	Confidence, Uncertainty, EvidenceScore, ConsistencyScore, ContradictionScore, FeasibilityScore, RiskScore, UtilityScore, DiversityScore float64
	HardConstraintViolation                                                                                                                 bool   `json:"hard_constraint_violation"`
	HardConstraintReason                                                                                                                    string `json:"hard_constraint_reason"`
}
type Result struct {
	Answer   string `json:"answer"`
	Selected *State `json:"selected"`
}
