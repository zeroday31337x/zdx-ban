package experiment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"zdx-ban/internal/ban"
)

func validCase() Case {
	return Case{DatasetVersion: "v1", ID: "x", Version: "1", Category: "arithmetic_constraints", Prompt: "return seven", Expected: 7.0, VerifierType: "numeric", VerifierConfig: map[string]any{"tolerance": 0.0}}
}
func TestDatasetValidation(t *testing.T) {
	r := NewRegistry()
	c := validCase()
	if err := ValidateCase(c, r); err != nil {
		t.Fatal(err)
	}
	c.ForcedRecovery = true
	c.TrapDescription = "tempting wrong answer"
	if err := ValidateCase(c, r); err != nil {
		t.Fatal(err)
	}
	c.VerifierType = "missing"
	if err := ValidateCase(c, r); err == nil {
		t.Fatal("unsupported accepted")
	}
	path := filepath.Join(t.TempDir(), "d.jsonl")
	c = validCase()
	b, _ := json.Marshal(c)
	data := append(append(append([]byte{}, b...), '\n'), append(b, '\n')...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDataset(path, r); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestStrictVerifiers(t *testing.T) {
	r := NewRegistry()
	c := validCase()
	v, _ := r.Get("numeric")
	if !verify(v, context.Background(), c, "7").Passed {
		t.Fatal("numeric failed")
	}
	if verify(v, context.Background(), c, "8").Passed {
		t.Fatal("wrong passed")
	}
	j := Case{Expected: map[string]any{"x": 2.0}, VerifierType: "json"}
	jv, _ := r.Get("json")
	if !verify(jv, context.Background(), j, `{"x":2,"extra":true}`).Passed {
		t.Fatal("json failed")
	}
	con := Case{Constraints: []Constraint{{Op: "gt", Value: 5.0}, {Op: "lt", Value: 10.0}}, VerifierType: "constraint"}
	cv, _ := r.Get("constraint")
	if !verify(cv, context.Background(), con, "8").Passed {
		t.Fatal("constraint failed")
	}
}
func TestSummaryAndMcNemar(t *testing.T) {
	rows := []PairedResult{{Category: "logic", Baseline: SideResult{Verification: Verification{Passed: true}, ModelCalls: 1}, BAN: SideResult{Verification: Verification{Passed: false}, ModelCalls: 5}}, {Category: "logic", Baseline: SideResult{Verification: Verification{Passed: false}, ModelCalls: 1}, BAN: SideResult{Verification: Verification{Passed: true}, ModelCalls: 5}, RecoveryAttempted: true, RecoverySuccessful: true}}
	s := Summarize(rows)
	if s.TotalPairs != 2 || s.Baseline.Percentage != 50 || s.BAN.Percentage != 50 || s.RecoveryRate != 100 || s.McNemar == nil {
		t.Fatalf("bad summary %+v", s)
	}
	if Summarize(nil).RecoveryRate != 0 {
		t.Fatal("zero denominator")
	}
}
func TestRunnerRejectsPermissiveAndDrift(t *testing.T) {
	r := Runner{Registry: NewRegistry(), Config: RunConfig{Model: "m", MaxTokens: 10, Timeout: 1, Repetitions: 1, BAN: ban.DefaultConfig()}, OutputRoot: t.TempDir()}
	if err := r.Validate(); err == nil {
		t.Fatal("permissive experiment accepted")
	}
	a := RunConfig{Model: "m", Temperature: .2}
	b := a
	b.Temperature = .3
	if configHash(a) == configHash(b) {
		t.Fatal("drift undetected")
	}
}
