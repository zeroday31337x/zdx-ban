package measurement

import (
	"fmt"
	"time"
)

type Outcome string

const (
	Supported    Outcome = "SUPPORTED"
	Contradicted Outcome = "CONTRADICTED"
	Inconclusive Outcome = "INCONCLUSIVE"
	Error        Outcome = "ERROR"
	Unsupported  Outcome = "UNSUPPORTED"
	NotMeasured  Outcome = "NOT_MEASURED"
)

type VerificationClass string

const (
	BenchmarkVerified    VerificationClass = "BENCHMARK_VERIFIED"
	FormallyVerified     VerificationClass = "FORMALLY_VERIFIED"
	ExecutionVerified    VerificationClass = "EXECUTION_VERIFIED"
	MeasurementSupported VerificationClass = "MEASUREMENT_SUPPORTED"
	CausallySupported    VerificationClass = "CAUSALLY_SUPPORTED"
	Unverified           VerificationClass = "UNVERIFIED"
)

type Authority string

const (
	Formal               Authority = "FORMAL"
	DeterministicRuntime Authority = "DETERMINISTIC_RUNTIME"
	Instrumented         Authority = "INSTRUMENTED"
	Experimental         Authority = "EXPERIMENTAL"
	Observational        Authority = "OBSERVATIONAL"
	ModelEstimate        Authority = "MODEL_ESTIMATE"
	UnknownAuthority     Authority = "UNKNOWN"
)

type Independence string

const (
	Independent          Independence = "INDEPENDENT"
	PartiallyIndependent Independence = "PARTIALLY_INDEPENDENT"
	ModelDerived         Independence = "MODEL_DERIVED"
	UnknownIndependence  Independence = "UNKNOWN"
)

type Repeatability string

const (
	Deterministic Repeatability = "DETERMINISTIC"
	Repeatable    Repeatability = "REPEATABLE"
	Stochastic    Repeatability = "STOCHASTIC"
	OneShot       Repeatability = "ONE_SHOT"
)

type MeasurementType string

const (
	Exact       MeasurementType = "EXACT_COMPARISON"
	Numeric     MeasurementType = "NUMERIC_COMPARISON"
	Constraint  MeasurementType = "CONSTRAINT_EVALUATION"
	Structured  MeasurementType = "STRUCTURED_INVARIANT"
	Execution   MeasurementType = "CONTROLLED_EXECUTION"
	Observation MeasurementType = "SYSTEM_OBSERVATION"
)

type EvidenceRelationship string

const (
	Supports    EvidenceRelationship = "SUPPORTS"
	Contradicts EvidenceRelationship = "CONTRADICTS"
	Neutral     EvidenceRelationship = "NEUTRAL"
)

type Tolerance struct {
	Absolute *float64 `json:"absolute,omitempty"`
	Relative *float64 `json:"relative,omitempty"`
	Unit     string   `json:"unit,omitempty"`
}
type Uncertainty struct {
	PlusMinus    float64 `json:"plusMinus"`
	Unit, Source string
}
type Provenance struct {
	Implementation, ImplementationVersion, BenchmarkCase, DatasetVersion, DatasetHash, FixtureVersion, Runtime, GitCommit, GitRemote string
	GitDirty                                                                                                                         bool
	Host                                                                                                                             map[string]any `json:"host,omitempty"`
	Timestamp                                                                                                                        time.Time
}
type Contract struct {
	ID, Claim                                string
	MeasurementType                          MeasurementType
	Observable, Method, ExpectedRelationship string
	Tolerance                                *Tolerance `json:"tolerance,omitempty"`
	Authority                                Authority
	Independence                             Independence
	Repeatability                            Repeatability
	Provenance                               Provenance
	Metadata                                 map[string]any `json:"metadata,omitempty"`
}
type Evidence struct {
	ID, Source       string
	Observation      any
	Relationship     EvidenceRelationship
	Strength         *float64 `json:"strength,omitempty"`
	MeasurementID    string
	Provenance       Provenance
	Timestamp        time.Time
	Independent      bool
	CorrelationGroup string `json:"correlationGroup,omitempty"`
}
type Cost struct {
	Latency                                                     time.Duration
	CPUTime                                                     *time.Duration `json:"cpuTime,omitempty"`
	HeapBytesBefore, HeapBytesAfter                             uint64
	ExternalAPICalls, PaidInferenceCalls, LocalMeasurementCalls int
}
type MeasurementError struct {
	Code, Message string
	Retryable     bool
}
type Result struct {
	ID, ContractID, Claim string
	Outcome               Outcome
	VerificationClass     VerificationClass
	Observation, Expected any
	Tolerance             *Tolerance   `json:"tolerance,omitempty"`
	Uncertainty           *Uncertainty `json:"uncertainty,omitempty"`
	Evidence              []Evidence
	Method                string
	Authority             Authority
	Independence          Independence
	Repeatable            bool
	Repeatability         Repeatability
	Provenance            Provenance
	StartedAt, FinishedAt time.Time
	Cost                  Cost
	Error                 *MeasurementError `json:"error,omitempty"`
}

func (r Result) Passed() bool { return r.Outcome == Supported }
func (r Result) Authoritative() bool {
	return r.Authority == Formal || r.Authority == DeterministicRuntime
}
func ValidateContract(c Contract) error {
	if c.ID == "" || c.Claim == "" || c.Observable == "" || c.Method == "" {
		return fmt.Errorf("measurement contract missing required fields")
	}
	if c.Independence == Independent && c.Authority == ModelEstimate {
		return fmt.Errorf("model estimate cannot be labeled independent")
	}
	return nil
}
