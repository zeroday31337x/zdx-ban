package experiment

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
	"zdx-ban/internal/measurement"
)

type EpistemicOutcome string

const (
	EpistemicSupported    EpistemicOutcome = "SUPPORTED"
	EpistemicContradicted EpistemicOutcome = "CONTRADICTED"
	EpistemicInconclusive EpistemicOutcome = "INCONCLUSIVE"
	EpistemicUnsupported  EpistemicOutcome = "UNSUPPORTED"
	EpistemicUnknown      EpistemicOutcome = "UNKNOWN"
	EpistemicFailed       EpistemicOutcome = "FAILED"
)

type MemoryEffect string

const (
	EffectNone      MemoryEffect = "NONE"
	EffectHelpful   MemoryEffect = "HELPFUL"
	EffectHarmful   MemoryEffect = "HARMFUL"
	EffectNeutral   MemoryEffect = "NEUTRAL"
	EffectRecovered MemoryEffect = "RECOVERED_FROM_HARM"
	EffectUnknown   MemoryEffect = "UNKNOWN"
)

type StabilityClass string

const (
	JustifiedImprovement StabilityClass = "JUSTIFIED_IMPROVEMENT"
	JustifiedCorrection  StabilityClass = "JUSTIFIED_CORRECTION"
	UnjustifiedDrift     StabilityClass = "UNJUSTIFIED_DRIFT"
	HarmfulAnchoring     StabilityClass = "HARMFUL_ANCHORING"
	StabilityRecovery    StabilityClass = "RECOVERY"
	NoMaterialChange     StabilityClass = "NO_MATERIAL_CHANGE"
)

type MemoryAttribution struct {
	MemoryRetrieved          bool
	MemoryIDs                []string
	MemoryRelevance          map[string]float64
	MemoryUsed               bool
	MemoryRejected           []string
	MemoryConflicted         []string
	MemoryConflictResolution string
	MemoryEffect             MemoryEffect
	EvidenceOverride         bool
	ObservableBasis          string
}
type MemoryRawResult struct {
	ExperimentID, RunID, AttemptID                          string
	CaseID, DatasetVersion, DatasetHash, Category, Behavior string
	Condition                                               MemoryCondition
	Repetition                                              int
	Seed                                                    *int
	Provider, Model, ConfigurationHash                      string
	Result                                                  EpistemicOutcome
	Verification                                            Verification
	Attribution                                             MemoryAttribution
	Stability                                               StabilityClass
	Graph                                                   GraphMetrics
	ModelCalls                                              int
	ProviderRuntime                                         ProviderAccounting
	Tokens                                                  *int
	Measurements                                            int
	BranchesAvoided, ModelCallsAvoided                      int
	RecoveryAttempted, RecoverySuccessful                   bool
	Duration                                                time.Duration
	InitialMemoryHash, FinalMemoryHash                      string
	FailureCode, Error                                      string
	AttemptStartedAt, AttemptFinishedAt, RecordPersistedAt  time.Time
	Timestamp                                               time.Time
}
type MemoryAggregate struct {
	Rows                                                                                                                                    int
	ByCondition                                                                                                                             map[MemoryCondition]map[EpistemicOutcome]int
	ByCategory                                                                                                                              map[string]map[MemoryCondition]map[EpistemicOutcome]int
	Effects                                                                                                                                 map[MemoryEffect]int
	Stability                                                                                                                               map[StabilityClass]int
	Retrievals, RetrievalObservations, RelevantRetrievals, RetrievedUnused, Helpful, Harmful, Recovered, IrrelevantRejected, StaleOverrides int
	ProviderCalls, ProviderAttempts, ProviderFailures, ProviderTimeouts, Measurements                                                       int
	BranchesAvoided, ModelCallsAvoided, ContractCompliant, NovelCandidatesPreserved                                                         int
	Duration                                                                                                                                time.Duration
}

func classifyEpistemic(v Verification) EpistemicOutcome {
	switch v.Measurement.Outcome {
	case measurement.Supported:
		return EpistemicSupported
	case measurement.Contradicted:
		return EpistemicContradicted
	case measurement.Inconclusive:
		return EpistemicInconclusive
	case measurement.Unsupported:
		return EpistemicUnsupported
	case measurement.NotMeasured:
		return EpistemicUnknown
	default:
		return EpistemicFailed
	}
}
func AggregateMemoryRows(rows []MemoryRawResult) MemoryAggregate {
	a := MemoryAggregate{Rows: len(rows), ByCondition: map[MemoryCondition]map[EpistemicOutcome]int{}, ByCategory: map[string]map[MemoryCondition]map[EpistemicOutcome]int{}, Effects: map[MemoryEffect]int{}, Stability: map[StabilityClass]int{}}
	for _, r := range rows {
		if a.ByCondition[r.Condition] == nil {
			a.ByCondition[r.Condition] = map[EpistemicOutcome]int{}
		}
		a.ByCondition[r.Condition][r.Result]++
		if a.ByCategory[r.Category] == nil {
			a.ByCategory[r.Category] = map[MemoryCondition]map[EpistemicOutcome]int{}
		}
		if a.ByCategory[r.Category][r.Condition] == nil {
			a.ByCategory[r.Category][r.Condition] = map[EpistemicOutcome]int{}
		}
		a.ByCategory[r.Category][r.Condition][r.Result]++
		a.Effects[r.Attribution.MemoryEffect]++
		a.Stability[r.Stability]++
		a.Retrievals += len(r.Attribution.MemoryIDs)
		if r.Attribution.MemoryRetrieved {
			a.RetrievalObservations++
		}
		for _, score := range r.Attribution.MemoryRelevance {
			if score >= .20 {
				a.RelevantRetrievals++
			}
		}
		if r.Attribution.MemoryRetrieved && !r.Attribution.MemoryUsed {
			a.RetrievedUnused += len(r.Attribution.MemoryIDs)
		}
		if r.Attribution.MemoryEffect == EffectHelpful {
			a.Helpful++
		}
		if r.Attribution.MemoryEffect == EffectHarmful {
			a.Harmful++
		}
		if r.Attribution.MemoryEffect == EffectRecovered {
			a.Recovered++
		}
		if len(r.Attribution.MemoryRejected) > 0 {
			a.IrrelevantRejected++
		}
		if r.Attribution.EvidenceOverride {
			a.StaleOverrides++
		}
		a.ProviderCalls += r.ModelCalls
		a.ProviderAttempts += r.ProviderRuntime.Attempted
		a.ProviderFailures += r.ProviderRuntime.Failed
		a.ProviderTimeouts += r.ProviderRuntime.TimedOut
		a.Measurements += r.Measurements
		a.Duration += r.Duration
		a.BranchesAvoided += r.BranchesAvoided
		a.ModelCallsAvoided += r.ModelCallsAvoided
		if r.Measurements > 0 && r.Result != EpistemicFailed {
			a.ContractCompliant++
		}
		if r.Behavior == "UNKNOWN_NOVEL_PRESERVATION" && r.Result != EpistemicContradicted && r.Result != EpistemicFailed {
			a.NovelCandidatesPreserved++
		}
	}
	return a
}
func LoadMemoryRows(path string) ([]MemoryRawResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 8<<20)
	var rows []MemoryRawResult
	for line := 1; s.Scan(); line++ {
		var r MemoryRawResult
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, r)
	}
	return rows, s.Err()
}

type ConditionComparison struct {
	From, To                              MemoryCondition
	Total, Improved, Regressed, Unchanged int
}

func CompareMemoryRows(rows []MemoryRawResult, from, to MemoryCondition) ConditionComparison {
	c := ConditionComparison{From: from, To: to}
	by := map[string]map[MemoryCondition]EpistemicOutcome{}
	for _, r := range rows {
		k := fmt.Sprintf("%s/%d", r.CaseID, r.Repetition)
		if by[k] == nil {
			by[k] = map[MemoryCondition]EpistemicOutcome{}
		}
		by[k][r.Condition] = r.Result
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a, oka := by[k][from]
		b, okb := by[k][to]
		if !oka || !okb {
			continue
		}
		if a == EpistemicFailed || b == EpistemicFailed {
			continue
		}
		c.Total++
		if a != EpistemicSupported && b == EpistemicSupported {
			c.Improved++
		} else if a == EpistemicSupported && b != EpistemicSupported {
			c.Regressed++
		} else {
			c.Unchanged++
		}
	}
	return c
}
