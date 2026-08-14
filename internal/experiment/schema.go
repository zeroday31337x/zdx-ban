package experiment

import (
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
	"zdx-ban/internal/telemetry"
)

const SchemaVersion = "0.3"
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
	Model, Provider              string
	Temperature                  float64
	Seed                         *int `json:"seed,omitempty"`
	MaxTokens                    int
	Timeout                      time.Duration
	Repetitions                  int
	BAN                          ban.Config
	RequireObjectiveVerification bool
	MemoryEnabled                bool
	MemoryRetrieval              memory.RetrievalConfig
	MemoryConsolidation          memory.ConsolidationConfig
	MemoryWritePolicy            string
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
	Pass            Outcome = "pass"
	Incorrect       Outcome = "incorrect"
	VerifierError   Outcome = "verifier_error"
	Misconfigured   Outcome = "misconfigured"
	Unsupported     Outcome = "unsupported"
	TimedOut        Outcome = "timeout"
	ProviderFailure Outcome = "provider_failure"
	MalformedOutput Outcome = "malformed_output"
	Interrupted     Outcome = "interrupted"
	Unscored        Outcome = "unscored"
)

type Verification struct {
	Outcome     Outcome            `json:"outcome"`
	Passed      bool               `json:"passed"`
	Details     string             `json:"details"`
	Value       any                `json:"value,omitempty"`
	Duration    time.Duration      `json:"duration"`
	Measurement measurement.Result `json:"measurement"`
}
type RunTelemetry struct {
	Before, After                                    telemetry.Snapshot
	PeakHeapAlloc, TotalAllocBefore, TotalAllocAfter uint64
	GCBefore, GCAfter                                uint32
	Duration, ModelLatency                           time.Duration
}
type GraphMetrics struct{ NodesCreated, DuplicatesDetected, Convergences, MultipleParentNodes, BranchesPruned, MaxDepthReached, PeakActiveBranches int }
type SideResult struct {
	Answer          string               `json:"answer"`
	Verification    Verification         `json:"verification"`
	Measurements    []measurement.Result `json:"measurements"`
	Latency         time.Duration
	ModelCalls      int
	Tokens          *int `json:"tokens,omitempty"`
	Telemetry       RunTelemetry
	FailureCategory string `json:"failureCategory,omitempty"`
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
	CompletedAt                              time.Time
	ConfigurationHash                        string
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
