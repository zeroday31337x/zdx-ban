package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/telemetry"
)

type ObjectiveVerifier interface {
	Type() string
	Validate(Case) error
	Measure(context.Context, Case, string) measurement.Result
}
type Registry struct{ items map[string]ObjectiveVerifier }

func NewRegistry() *Registry {
	r := &Registry{items: map[string]ObjectiveVerifier{}}
	for _, v := range []ObjectiveVerifier{ExactVerifier{}, NumericVerifier{}, ConstraintVerifier{}, JSONVerifier{}} {
		r.items[v.Type()] = v
	}
	return r
}
func (r *Registry) Get(name string) (ObjectiveVerifier, bool) { v, ok := r.items[name]; return v, ok }
func measure(v ObjectiveVerifier, ctx context.Context, c Case, s string) measurement.Result {
	start := time.Now().UTC()
	before := telemetry.Capture()
	out := v.Measure(ctx, c, s)
	after := telemetry.Capture()
	out.StartedAt = start
	out.FinishedAt = time.Now().UTC()
	out.Cost.Latency = out.FinishedAt.Sub(start)
	out.Cost.HeapBytesBefore = before.HeapAlloc
	out.Cost.HeapBytesAfter = after.HeapAlloc
	out.Cost.LocalMeasurementCalls = 1
	out.Cost.PaidInferenceCalls = 0
	out.Provenance = c.Measurement.Contract.Provenance
	out.Provenance.Implementation = "zdx-ban/internal/experiment/" + v.Type()
	out.Provenance.ImplementationVersion = SchemaVersion
	out.Provenance.BenchmarkCase = c.ID
	out.Provenance.DatasetVersion = c.DatasetVersion
	out.Provenance.Runtime = runtime.Version()
	out.Provenance.Timestamp = out.FinishedAt
	if out.ContractID == "" {
		out.ContractID = c.Measurement.Contract.ID
	}
	if out.Claim == "" {
		out.Claim = c.Measurement.Contract.Claim
	}
	if out.VerificationClass == "" {
		out.VerificationClass = c.Measurement.VerificationClass
	}
	if out.Authority == "" {
		out.Authority = c.Measurement.Contract.Authority
	}
	if out.Independence == "" {
		out.Independence = c.Measurement.Contract.Independence
	}
	if out.Repeatability == "" {
		out.Repeatability = c.Measurement.Contract.Repeatability
	}
	out.Repeatable = out.Repeatability == measurement.Deterministic || out.Repeatability == measurement.Repeatable
	return out
}
func compatibility(m measurement.Result) Verification {
	v := Verification{Measurement: m, Passed: m.Outcome == measurement.Supported, Duration: m.Cost.Latency, Details: fmt.Sprintf("%s under %s: %v", m.Outcome, m.VerificationClass, m.Observation), Value: m.Observation}
	switch m.Outcome {
	case measurement.Supported:
		v.Outcome = Pass
	case measurement.Contradicted:
		v.Outcome = Incorrect
	case measurement.Inconclusive, measurement.NotMeasured:
		v.Outcome = Unscored
	case measurement.Unsupported:
		v.Outcome = Unsupported
	case measurement.Error:
		v.Outcome = VerifierError
	}
	return v
}
func verify(v ObjectiveVerifier, ctx context.Context, c Case, s string) Verification {
	return compatibility(measure(v, ctx, c, s))
}
func baseResult(c Case, obs any) measurement.Result {
	return measurement.Result{ID: fmt.Sprintf("%s-%d", c.ID, time.Now().UnixNano()), ContractID: c.Measurement.Contract.ID, Claim: c.Measurement.Contract.Claim, VerificationClass: c.Measurement.VerificationClass, Observation: obs, Expected: c.Expected, Tolerance: c.Measurement.Contract.Tolerance, Method: c.Measurement.Contract.Method, Authority: c.Measurement.Contract.Authority, Independence: c.Measurement.Contract.Independence, Repeatability: c.Measurement.Contract.Repeatability}
}
func evidence(c Case, obs any, rel measurement.EvidenceRelationship) []measurement.Evidence {
	return []measurement.Evidence{{ID: fmt.Sprintf("e-%s-%d", c.ID, time.Now().UnixNano()), Source: "deterministic local " + c.VerifierType, Observation: obs, Relationship: rel, MeasurementID: c.Measurement.Contract.ID, Provenance: c.Measurement.Contract.Provenance, Timestamp: time.Now().UTC(), Independent: c.Measurement.Contract.Independence == measurement.Independent, CorrelationGroup: c.Measurement.Contract.ID}}
}
func normalized(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(strings.Trim(s, "`\"'"))), " "))
}

type ExactVerifier struct{}

func (ExactVerifier) Type() string { return "exact" }
func (ExactVerifier) Validate(c Case) error {
	if c.Expected == nil {
		return fmt.Errorf("expected required")
	}
	return nil
}
func (ExactVerifier) Measure(_ context.Context, c Case, s string) measurement.Result {
	expected := fmt.Sprint(c.Expected)
	actual := strings.TrimSpace(s)
	norm, _ := c.VerifierConfig["normalize"].(bool)
	if norm {
		expected = normalized(expected)
		actual = normalized(actual)
	}
	r := baseResult(c, actual)
	r.Expected = expected
	if actual == expected {
		r.Outcome = measurement.Supported
		r.Evidence = evidence(c, actual, measurement.Supports)
	} else {
		r.Outcome = measurement.Contradicted
		r.Evidence = evidence(c, actual, measurement.Contradicts)
	}
	return r
}

var numberRE = regexp.MustCompile(`[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)

func extractNumber(s string) (float64, error) {
	m := numberRE.FindAllString(s, -1)
	if len(m) == 0 {
		return 0, fmt.Errorf("no numeric observation")
	}
	return strconv.ParseFloat(m[len(m)-1], 64)
}

type NumericVerifier struct{}

func (NumericVerifier) Type() string { return "numeric" }
func (NumericVerifier) Validate(c Case) error {
	if _, e := asFloat(c.Expected); e != nil {
		return fmt.Errorf("expected numeric")
	}
	return nil
}
func (NumericVerifier) Measure(_ context.Context, c Case, s string) measurement.Result {
	x, err := extractNumber(s)
	if err != nil {
		r := baseResult(c, nil)
		r.Outcome = measurement.Inconclusive
		r.Error = &measurement.MeasurementError{Code: "OBSERVATION_UNPARSEABLE", Message: err.Error()}
		return r
	}
	expected, _ := asFloat(c.Expected)
	tol := 0.0
	if v, ok := c.VerifierConfig["tolerance"]; ok {
		tol, _ = asFloat(v)
	}
	r := baseResult(c, x)
	r.Expected = expected
	a := tol
	r.Tolerance = &measurement.Tolerance{Absolute: &a}
	if math.Abs(x-expected) <= tol {
		r.Outcome = measurement.Supported
		r.Evidence = evidence(c, x, measurement.Supports)
	} else {
		r.Outcome = measurement.Contradicted
		r.Evidence = evidence(c, x, measurement.Contradicts)
	}
	return r
}

type ConstraintVerifier struct{}

func (ConstraintVerifier) Type() string { return "constraint" }
func (ConstraintVerifier) Validate(c Case) error {
	if len(c.Constraints) == 0 {
		return fmt.Errorf("constraints required")
	}
	for _, x := range c.Constraints {
		if !map[string]bool{"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true, "mod_eq": true}[x.Op] {
			return fmt.Errorf("unsupported op %q", x.Op)
		}
	}
	return nil
}
func (ConstraintVerifier) Measure(_ context.Context, c Case, s string) measurement.Result {
	x, err := extractNumber(s)
	r := baseResult(c, x)
	if err != nil {
		r.Outcome = measurement.Inconclusive
		r.Error = &measurement.MeasurementError{Code: "OBSERVATION_UNPARSEABLE", Message: err.Error()}
		return r
	}
	for _, rule := range c.Constraints {
		v, e := asFloat(rule.Value)
		if e != nil {
			r.Outcome = measurement.Error
			r.Error = &measurement.MeasurementError{Code: "CONFIGURATION", Message: e.Error()}
			return r
		}
		ok := false
		switch rule.Op {
		case "eq":
			ok = x == v
		case "ne":
			ok = x != v
		case "gt":
			ok = x > v
		case "gte":
			ok = x >= v
		case "lt":
			ok = x < v
		case "lte":
			ok = x <= v
		case "mod_eq":
			d, _ := asFloat(c.VerifierConfig["divisor"])
			if d == 0 {
				r.Outcome = measurement.Error
				r.Error = &measurement.MeasurementError{Code: "CONFIGURATION", Message: "nonzero divisor required"}
				return r
			}
			ok = math.Mod(x, d) == v
		}
		if !ok {
			r.Outcome = measurement.Contradicted
			r.Evidence = evidence(c, x, measurement.Contradicts)
			return r
		}
	}
	r.Outcome = measurement.Supported
	r.Evidence = evidence(c, x, measurement.Supports)
	return r
}

type JSONVerifier struct{}

func (JSONVerifier) Type() string { return "json" }
func (JSONVerifier) Validate(c Case) error {
	if _, ok := c.Expected.(map[string]any); !ok {
		return fmt.Errorf("expected object")
	}
	return nil
}
func (JSONVerifier) Measure(_ context.Context, c Case, s string) measurement.Result {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	r := baseResult(c, nil)
	if start < 0 || end < start {
		r.Outcome = measurement.Inconclusive
		r.Error = &measurement.MeasurementError{Code: "OBSERVATION_UNPARSEABLE", Message: "JSON object not found"}
		return r
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(s[start:end+1]), &actual); err != nil {
		r.Outcome = measurement.Inconclusive
		r.Error = &measurement.MeasurementError{Code: "OBSERVATION_UNPARSEABLE", Message: err.Error()}
		return r
	}
	r.Observation = actual
	for k, v := range c.Expected.(map[string]any) {
		if fmt.Sprint(actual[k]) != fmt.Sprint(v) {
			r.Outcome = measurement.Contradicted
			r.Evidence = evidence(c, actual, measurement.Contradicts)
			return r
		}
	}
	r.Outcome = measurement.Supported
	r.Evidence = evidence(c, actual, measurement.Supports)
	return r
}
func asFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		return strconv.ParseFloat(x, 64)
	default:
		return 0, fmt.Errorf("not numeric")
	}
}
