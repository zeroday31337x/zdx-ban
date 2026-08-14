package experiment

import (
	"encoding/json"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/model"
	"zdx-ban/internal/telemetry"
)

const SchemaVersion = "0.1"
const ExperimentName = "BAN-EXPERIMENT-001"

type Constraint struct {
	Field string `json:"field,omitempty"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}
type Case struct {
	DatasetVersion  string         `json:"datasetVersion"`
	ID              string         `json:"id"`
	Version         string         `json:"version"`
	Category        string         `json:"category"`
	Prompt          string         `json:"prompt"`
	Expected        any            `json:"expected"`
	Constraints     []Constraint   `json:"constraints,omitempty"`
	VerifierType    string         `json:"verifierType"`
	VerifierConfig  map[string]any `json:"verifierConfig,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	ForcedRecovery  bool           `json:"forcedRecovery"`
	TrapDescription string         `json:"trapDescription,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}
type RunConfig struct {
	Model                        string        `json:"model"`
	Provider                     string        `json:"provider"`
	Temperature                  float64       `json:"temperature"`
	Seed                         *int          `json:"seed,omitempty"`
	MaxTokens                    int           `json:"maxTokens"`
	Timeout                      time.Duration `json:"timeout"`
	Repetitions                  int           `json:"repetitions"`
	BAN                          ban.Config    `json:"ban"`
	RequireObjectiveVerification bool          `json:"requireObjectiveVerification"`
}
type Manifest struct {
	Experiment, SchemaVersion, ExperimentID, DatasetVersion, DatasetPath, DatasetSHA256, GitCommit string
	Model                                                                                          model.Info
	Configuration                                                                                  RunConfig
	Host                                                                                           telemetry.Snapshot
	StartedAt                                                                                      time.Time
	FinishedAt                                                                                     *time.Time `json:"finishedAt,omitempty"`
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
)

type Verification struct {
	Outcome  Outcome       `json:"outcome"`
	Passed   bool          `json:"passed"`
	Details  string        `json:"details"`
	Value    any           `json:"value,omitempty"`
	Duration time.Duration `json:"duration"`
}
type RunTelemetry struct {
	Before, After                                    telemetry.Snapshot
	PeakHeapAlloc, TotalAllocBefore, TotalAllocAfter uint64
	GCBefore, GCAfter                                uint32
	Duration                                         time.Duration
	ModelLatency                                     time.Duration
}
type GraphMetrics struct{ NodesCreated, DuplicatesDetected, Convergences, MultipleParentNodes, BranchesPruned, MaxDepthReached, PeakActiveBranches int }
type SideResult struct {
	Answer          string        `json:"answer"`
	Verification    Verification  `json:"verification"`
	Latency         time.Duration `json:"latency"`
	ModelCalls      int           `json:"modelCalls"`
	Tokens          *int          `json:"tokens,omitempty"`
	Telemetry       RunTelemetry  `json:"telemetry"`
	FailureCategory string        `json:"failureCategory,omitempty"`
}
type PairedResult struct {
	CaseID, Category                      string
	Repetition                            int
	Seed                                  *int
	Baseline                              SideResult
	BAN                                   SideResult
	BANInitial                            Verification
	InitialWinnerID, FinalWinnerID        string
	RecoveryAttempted, RecoverySuccessful bool
	Graph                                 GraphMetrics
	TraceRunID                            string
	CompletedAt                           time.Time
	ConfigurationHash                     string
}
type Distribution struct {
	Count                          int
	Mean, Median, Min, Max, StdDev float64
}
type Accuracy struct {
	Total, Passed int
	Percentage    float64
}
type CategorySummary struct {
	Baseline, BAN                Accuracy
	Recoveries, RecoveryAttempts int
}
type Summary struct {
	TotalPairs                                                                                                                  int
	Baseline, BAN                                                                                                               Accuracy
	BANInitial                                                                                                                  Accuracy
	RecoveryAttempts, SuccessfulRecoveries                                                                                      int
	RecoveryRate                                                                                                                float64
	BaselineCalls, BANCalls                                                                                                     int
	BaselineTokens, BANTokens                                                                                                   *int
	BaselineLatency, BANLatency                                                                                                 Distribution
	Graph                                                                                                                       GraphMetrics
	Categories                                                                                                                  map[string]CategorySummary
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

func decodeRaw(raw json.RawMessage) (any, error) {
	var v any
	err := json.Unmarshal(raw, &v)
	return v, err
}
