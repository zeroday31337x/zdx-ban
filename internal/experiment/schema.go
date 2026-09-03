package experiment

import (
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
	"zdx-ban/internal/telemetry"
	"zdx-ban/internal/training"
)

const SchemaVersion = "0.4"
const ExperimentName = "BAN-EXPERIMENT-001"

type Constraint struct {
	Field string `json:"field,omitempty"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}
type MeasurementSpec struct {
	VerificationClass measurement.VerificationClass `json:"verificationClass"`
	Contract          measurement.Contract          `json:"contract"`
}
type Case struct {
	DatasetVersion, ID, Version, Category, Prompt string
	Expected                                      any             `json:"expected"`
	Constraints                                   []Constraint    `json:"constraints,omitempty"`
	VerifierType                                  string          `json:"verifierType"`
	VerifierConfig                                map[string]any  `json:"verifierConfig,omitempty"`
	Measurement                                   MeasurementSpec `json:"measurement"`
	Tags                                          []string        `json:"tags,omitempty"`
	ForcedRecovery                                bool            `json:"forcedRecovery"`
	TrapDescription                               string          `json:"trapDescription,omitempty"`
	Metadata                                      map[string]any  `json:"metadata,omitempty"`
}
type RunConfig struct {
	Model, Provider, FoundationID string
	Temperature                   float64
	Seed                          *int `json:"seed,omitempty"`
	MaxTokens                     int
	ProposalMaxTokens             int
	EvaluationMaxTokens           int
	ChallengeMaxTokens            int
	FinalAnswerMaxTokens          int
	Timeout                       time.Duration `json:"timeout"`
	InferenceTimeout              time.Duration `json:"inferenceTimeout"`
	CaseTimeout                   time.Duration `json:"caseTimeout"`
	RunTimeout                    time.Duration `json:"runTimeout"`
	Streaming                     bool          `json:"streaming"`
	Repetitions                   int
	CaseLimit                     int
	BAN                           ban.Config
	RequireObjectiveVerification  bool
	MemoryEnabled                 bool
	MemoryRetrieval               memory.RetrievalConfig
	MemoryConsolidation           memory.ConsolidationConfig
	MemoryWritePolicy             string
	MemoryOnlineLearning          bool
}

// CommandDefaults supplies project-level defaults while CLI flags remain
// authoritative for a specific, recorded experiment run.
type CommandDefaults struct {
	Provider, FoundationID string
	Temperature            float64
	MemoryTemperature      float64
	MaxTokens              int
	ProposalMaxTokens      int
	EvaluationMaxTokens    int
	ChallengeMaxTokens     int
	FinalAnswerMaxTokens   int
	Timeout                time.Duration
	InferenceTimeout       time.Duration
	CaseTimeout            time.Duration
	RunTimeout             time.Duration
	Repetitions            int
	Seed                   int
	BAN                    ban.Config
}
type Manifest struct {
	Experiment, SchemaVersion, ExperimentID, DatasetVersion, DatasetPath, DatasetSHA256, GitCommit, GitRemote            string
	GitDirty                                                                                                             bool
	TraceSchemaVersion, MemorySchemaVersion, MemoryDatasetVersion, MemoryDatasetHash, InitialMemoryHash, FinalMemoryHash string
	Model                                                                                                                model.Info
	Configuration                                                                                                        RunConfig
	Host                                                                                                                 telemetry.Snapshot
	StartedAt                                                                                                            time.Time
	FinishedAt                                                                                                           *time.Time `json:"finishedAt,omitempty"`
}
type Outcome string

const (
	Pass                Outcome = "pass"
	Incorrect           Outcome = "incorrect"
	VerifierError       Outcome = "verifier_error"
	Misconfigured       Outcome = "misconfigured"
	Unsupported         Outcome = "unsupported"
	TimedOut            Outcome = "timeout"
	ProviderFailure     Outcome = "provider_failure"
	NoVerifiedCandidate Outcome = "no_verified_candidate"
	// ExecutionFailure is retained as a source-compatible alias.
	ExecutionFailure Outcome = NoVerifiedCandidate
	MalformedOutput  Outcome = "malformed_output"
	Interrupted      Outcome = "interrupted"
	Unscored         Outcome = "unscored"
)

type Verification struct {
	Outcome     Outcome            `json:"outcome"`
	Passed      bool               `json:"passed"`
	Details     string             `json:"details"`
	Value       any                `json:"value,omitempty"`
	Duration    time.Duration      `json:"duration"`
	Measurement measurement.Result `json:"measurement"`
}
type ProviderCall struct {
	Attempt                        int
	StartedAt, FinishedAt          time.Time
	Duration                       time.Duration
	Completed, Failed, TimedOut    bool
	FailureCode, Error             string
	PromptTokens, CompletionTokens *int
	Runtime                        model.CallTelemetry
}
type ProviderAccounting struct {
	Attempted, Completed, Failed, TimedOut, SuccessfulResponses int
	Calls                                                       []ProviderCall
}

type RunTelemetry struct {
	Before, After                                    telemetry.Snapshot
	PeakHeapAlloc, TotalAllocBefore, TotalAllocAfter uint64
	GCBefore, GCAfter                                uint32
	Duration, ModelLatency                           time.Duration
}
type GraphMetrics struct{ CandidateProposals, NodesCreated, DuplicateProposals, DuplicatesDetected, AnswerConvergences, DiversityRegenerations, Convergences, MultipleParentNodes, BranchesPruned, PrunedCandidates, ProviderEvaluations, MaxDepthReached, PeakActiveBranches, GravityRoutedBranches, GravityWellHits, GravityRecoveryAttempts, GravityRecoveries int }
type BranchRoute struct {
	ID, Title, ReasoningSummary, GravityWellID string
	Assumptions                                []string
	InformationGravity, GravityRepulsion       float64
}
type SideResult struct {
	Answer                                                                                                               string               `json:"answer"`
	Verification                                                                                                         Verification         `json:"verification"`
	Measurements                                                                                                         []measurement.Result `json:"measurements"`
	StartedAt, FinishedAt                                                                                                time.Time
	Latency, ProviderDuration, MemoryRetrievalDuration, GraphSearchDuration, VerificationDuration, OrchestrationDuration time.Duration
	ModelCalls                                                                                                           int                `json:"modelCalls"`
	Provider                                                                                                             ProviderAccounting `json:"providerAccounting"`
	Tokens                                                                                                               *int               `json:"tokens,omitempty"`
	Telemetry                                                                                                            RunTelemetry
	FailureCategory                                                                                                      string `json:"failureCategory,omitempty"`
}
type PairedResult struct {
	CaseID, Category                         string
	Repetition                               int
	Seed                                     *int
	Baseline, BAN                            SideResult
	BANInitial                               Verification
	CandidateMeasurements, FinalMeasurements []measurement.Result
	InitialWinnerID, FinalWinnerID           string
	RecoveryAttempted, RecoverySuccessful    bool
	Graph                                    GraphMetrics
	TraceRunID                               string
	Memory                                   ban.MemoryInteraction
	SelectedRoute                            BranchRoute
	AttemptStartedAt, CompletedAt            time.Time
	ConfigurationHash                        string
	// TrainingCandidates is an observational record of this pair's BAN run,
	// one candidate per graph node (see internal/cognitive.CandidatesFromBANTrace).
	// A node is only ever W1-eligible when this pair's own deterministic
	// verifier recorded an authoritative Supported measurement for it.
	TrainingCandidates []training.Candidate `json:"trainingCandidates,omitempty"`
}
type Distribution struct {
	Count                          int
	Mean, Median, Min, Max, StdDev float64
}
type Accuracy struct {
	Total, Passed, Unscored int
	Percentage              float64
}
type CategorySummary struct {
	Baseline, BAN                Accuracy
	Recoveries, RecoveryAttempts int
}
type MeasurementSummary struct {
	Supported, Contradicted, Inconclusive, Errors, NotMeasured int
	DeterministicLocalCalls, PaidInferenceCalls                int
	Latency                                                    time.Duration
	CandidateSupportedFinalContradicted                        int
}
type Summary struct {
	TotalPairs                                                                                                                  int
	Baseline, BAN, BANInitial                                                                                                   Accuracy
	RecoveryAttempts, SuccessfulRecoveries                                                                                      int
	RecoveryRate                                                                                                                float64
	BaselineCalls, BANCalls                                                                                                     int
	BaselineTokens, BANTokens                                                                                                   *int
	BaselineLatency, BANLatency                                                                                                 Distribution
	Graph                                                                                                                       GraphMetrics
	Categories                                                                                                                  map[string]CategorySummary
	Measurements                                                                                                                MeasurementSummary
	VerifiedPer100CallsBaseline, VerifiedPer100CallsBAN, AccuracyGain, AccuracyGainPerAdditionalCall, RecoveryPerAdditionalCall float64
	McNemar                                                                                                                     *McNemarResult `json:"mcnemar,omitempty"`
	GeneratedAt                                                                                                                 time.Time
}
type ResultFile struct {
	Experiment, SchemaVersion string
	Manifest                  Manifest
	Cases                     []PairedResult
	Summary                   Summary
}
