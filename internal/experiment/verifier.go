package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ObjectiveVerifier interface {
	Type() string
	Validate(Case) error
	Verify(context.Context, Case, string) Verification
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
func verify(v ObjectiveVerifier, ctx context.Context, c Case, s string) Verification {
	start := time.Now()
	out := v.Verify(ctx, c, s)
	out.Duration = time.Since(start)
	return out
}
func normalized(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(strings.Trim(s, "`\"'"))), " "))
}

type ExactVerifier struct{}

func (ExactVerifier) Type() string { return "exact" }
func (ExactVerifier) Validate(c Case) error {
	if c.Expected == nil {
		return fmt.Errorf("expected is required")
	}
	return nil
}
func (ExactVerifier) Verify(_ context.Context, c Case, s string) Verification {
	expected := fmt.Sprint(c.Expected)
	actual := strings.TrimSpace(s)
	norm, _ := c.VerifierConfig["normalize"].(bool)
	if norm {
		expected = normalized(expected)
		actual = normalized(actual)
	}
	pass := actual == expected
	return Verification{Outcome: choose(pass, Pass, Incorrect), Passed: pass, Details: fmt.Sprintf("expected %q, got %q", expected, actual), Value: actual}
}

var numberRE = regexp.MustCompile(`[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)

func extractNumber(s string) (float64, error) {
	matches := numberRE.FindAllString(s, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("no numeric value")
	}
	return strconv.ParseFloat(matches[len(matches)-1], 64)
}

type NumericVerifier struct{}

func (NumericVerifier) Type() string { return "numeric" }
func (NumericVerifier) Validate(c Case) error {
	if _, err := asFloat(c.Expected); err != nil {
		return fmt.Errorf("expected must be numeric")
	}
	if v, ok := c.VerifierConfig["tolerance"]; ok {
		if n, e := asFloat(v); e != nil || n < 0 {
			return fmt.Errorf("invalid tolerance")
		}
	}
	return nil
}
func (NumericVerifier) Verify(_ context.Context, c Case, s string) Verification {
	actual, err := extractNumber(s)
	if err != nil {
		return Verification{Outcome: MalformedOutput, Details: err.Error()}
	}
	expected, _ := asFloat(c.Expected)
	tol := 0.0
	if v, ok := c.VerifierConfig["tolerance"]; ok {
		tol, _ = asFloat(v)
	}
	pass := math.Abs(actual-expected) <= tol
	return Verification{Outcome: choose(pass, Pass, Incorrect), Passed: pass, Details: fmt.Sprintf("expected %g ± %g, got %g", expected, tol, actual), Value: actual}
}

type ConstraintVerifier struct{}

func (ConstraintVerifier) Type() string { return "constraint" }
func (ConstraintVerifier) Validate(c Case) error {
	if len(c.Constraints) == 0 {
		return fmt.Errorf("constraints required")
	}
	for _, x := range c.Constraints {
		if !map[string]bool{"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true, "mod_eq": true}[x.Op] {
			return fmt.Errorf("unsupported constraint op %q", x.Op)
		}
		if _, e := asFloat(x.Value); e != nil {
			return fmt.Errorf("constraint value must be numeric")
		}
	}
	return nil
}
func (ConstraintVerifier) Verify(_ context.Context, c Case, s string) Verification {
	x, err := extractNumber(s)
	if err != nil {
		return Verification{Outcome: MalformedOutput, Details: err.Error()}
	}
	for _, rule := range c.Constraints {
		v, _ := asFloat(rule.Value)
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
			div, _ := asFloat(c.VerifierConfig["divisor"])
			if div == 0 {
				return Verification{Outcome: Misconfigured, Details: "mod_eq requires nonzero divisor"}
			}
			ok = math.Mod(x, div) == v
		}
		if !ok {
			return Verification{Outcome: Incorrect, Details: fmt.Sprintf("%g fails %s %g", x, rule.Op, v), Value: x}
		}
	}
	return Verification{Outcome: Pass, Passed: true, Details: "all constraints satisfied", Value: x}
}

type JSONVerifier struct{}

func (JSONVerifier) Type() string { return "json" }
func (JSONVerifier) Validate(c Case) error {
	if _, ok := c.Expected.(map[string]any); !ok {
		return fmt.Errorf("expected must be an object")
	}
	return nil
}
func (JSONVerifier) Verify(_ context.Context, c Case, s string) Verification {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return Verification{Outcome: MalformedOutput, Details: "JSON object not found"}
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(s[start:end+1]), &actual); err != nil {
		return Verification{Outcome: MalformedOutput, Details: err.Error()}
	}
	expected := c.Expected.(map[string]any)
	for k, v := range expected {
		a, ok := actual[k]
		if !ok || fmt.Sprint(a) != fmt.Sprint(v) {
			return Verification{Outcome: Incorrect, Details: fmt.Sprintf("field %s mismatch", k), Value: actual}
		}
	}
	return Verification{Outcome: Pass, Passed: true, Details: "required JSON values match", Value: actual}
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
func choose[T any](b bool, a, c T) T {
	if b {
		return a
	}
	return c
}
