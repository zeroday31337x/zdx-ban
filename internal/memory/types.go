package memory

import (
	"time"
	"zdx-ban/internal/measurement"
)

const SchemaVersion = "0.1"

type Tier string

const (
	Foundation Tier = "TIER_1_FOUNDATION"
	Domain     Tier = "TIER_2_DOMAIN"
	Episodic   Tier = "TIER_3_EPISODIC"
)

type Status string

const (
	Active       Status = "ACTIVE"
	Supported    Status = "SUPPORTED"
	Contradicted Status = "CONTRADICTED"
	Superseded   Status = "SUPERSEDED"
	Uncertain    Status = "UNCERTAIN"
	Stale        Status = "STALE"
)

type Kind string

const (
	StructuralRule Kind = "STRUCTURAL_RULE"
	DomainPattern  Kind = "DOMAIN_PATTERN"
	EpisodeKind    Kind = "EPISODE"
	FailurePattern Kind = "FAILURE_PATTERN"
	Strategy       Kind = "STRATEGY"
)

type SourceClass string

const (
	CurrentMeasurement SourceClass = "CURRENT_MEASUREMENT_EVIDENCE"
	MemoryGuidance     SourceClass = "MEMORY_GUIDANCE"
	ModelAssertion     SourceClass = "MODEL_DERIVED"
)

type Provenance struct {
	Source, ExperimentID, CaseID, TraceID, BranchID, DatasetVersion, DatasetHash, GitCommit, CorrelationGroup string
	MeasurementIDs                                                                                            []string
	Authority                                                                                                 measurement.Authority
	Independence                                                                                              measurement.Independence
	SourceClass                                                                                               SourceClass
	ContributingRecordIDs                                                                                     []string
	CreatedAt                                                                                                 time.Time
}
type Record struct {
	ID                                     string `json:"id"`
	SchemaVersion                          string `json:"schemaVersion"`
	Tier                                   Tier   `json:"tier"`
	Kind                                   Kind   `json:"kind"`
	Title, Content, Category, StrategyType string
	Tags                                   []string
	CreatedAt, UpdatedAt                   time.Time
	Provenance                             Provenance
	RelatedMeasurementIDs                  []string
	MeasurementAuthority                   measurement.Authority
	Status                                 Status
	SupersededBy                           string         `json:"supersededBy,omitempty"`
	CorrelationGroup                       string         `json:"correlationGroup,omitempty"`
	Metadata                               map[string]any `json:"metadata,omitempty"`
}
type BranchExperience struct {
	BranchID, Title, Hypothesis, Status string
	GraphPath                           []string
	Measurements                        []measurement.Result
	Speculative                         bool
}
type Episode struct {
	Problem, TaskSignature, Category         string
	InputMetadata                            map[string]any
	Strategies                               []string
	Branches                                 []BranchExperience
	CandidateMeasurements, FinalMeasurements []measurement.Result
	Outcome, FailureClassification           string
	Recovered                                bool
	RecoveryEvents                           []string
	UsefulBranches, FailedStrategies         []string
	Latency                                  time.Duration
	ModelCalls, Tokens                       int
	Timestamp                                time.Time
}
type RetrievalConfig struct {
	MaxRecords, MaxChars                                                             int
	MaxAge                                                                           time.Duration
	IncludeTiers                                                                     []Tier
	TokenWeight, CategoryWeight, TagWeight, TierWeight, RecencyWeight, OutcomeWeight float64
}

func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{MaxRecords: 8, MaxChars: 6000, IncludeTiers: []Tier{Foundation, Domain, Episodic}, TokenWeight: .45, CategoryWeight: .2, TagWeight: .15, TierWeight: .1, RecencyWeight: .05, OutcomeWeight: .05}
}

type RetrievalRequest struct {
	Query, Category, StrategyType     string
	Tags                              []string
	Now                               time.Time
	Config                            RetrievalConfig
	ExcludeCaseID, ExcludeExactAnswer string
}
type RetrievalReason struct {
	CategoryMatch, StrategyMatch, Supported, FailureRelevant bool
	TokenOverlap                                             float64
	TagMatches                                               []string
	Tier                                                     Tier
	Recency                                                  float64
	Score                                                    float64
}
type Retrieved struct {
	Record Record
	Reason RetrievalReason
}
type RetrievalResult struct {
	Request      RetrievalRequest
	Records      []Retrieved
	ApproxChars  int
	SnapshotHash string
	Warnings     []string
}
type UpdateEvent struct {
	RecordID, Action, Reason, MeasurementID string
	PreviousStatus, NewStatus               Status
	At                                      time.Time
}
type ConsolidationConfig struct{ MinDistinctEpisodes int }
type ConsolidationRecord struct {
	Pattern                string
	ContributingEpisodeIDs []string
	CreatedRecordID        string
	SkippedReason          string
	At                     time.Time
}
