package simulation

import (
	"math"
	"strings"
	"testing"
)

func TestEvaluateRanksMultiStepDecisions(t *testing.T) {
	s := Scenario{
		Title: "Equipment choices",
		Choices: []Choice{
			{ID: "a", Label: "Option A", Branches: []Branch{
				{Label: "success", Probability: 0.6, Score: 10},
				{Label: "failure", Probability: 0.4, Score: -5},
			}},
			{ID: "b", Label: "Option B", Branches: []Branch{
				{Label: "start", Probability: 1, Score: 3, Next: []Branch{
					{Label: "improvement", Probability: 0.5, Score: 2},
					{Label: "setback", Probability: 0.5, Score: -1},
				}},
			}},
		},
	}
	report, err := Evaluate(s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ASSUMPTION_BASED" || report.ProbabilitySource != "USER_SUPPLIED" || len(report.Ranking) != 2 {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	a, b := report.Ranking[0], report.Ranking[1]
	if a.ID != "a" || math.Abs(a.ExpectedScore-4) > 1e-9 || math.Abs(a.LossProbability-0.4) > 1e-9 || a.TerminalPaths != 2 || a.BestScore != 10 || a.WorstScore != -5 {
		t.Fatalf("option a: %+v", a)
	}
	if b.ID != "b" || math.Abs(b.ExpectedScore-3.5) > 1e-9 || b.LossProbability != 0 || b.TerminalPaths != 2 || b.BestScore != 5 || b.WorstScore != 2 {
		t.Fatalf("option b: %+v", b)
	}
}

func TestEvaluateRejectsInvalidInputs(t *testing.T) {
	s := Scenario{Title: "T", Choices: []Choice{{ID: "a", Label: "A", Branches: []Branch{{Label: "x", Probability: 0.7, Score: 1}}}}}
	if _, err := Evaluate(s); err == nil || !strings.Contains(err.Error(), "sum") {
		t.Fatalf("invalid probability was accepted: %v", err)
	}
	s.Choices[0].Branches[0].Probability = 1
	s.Choices[0].Branches[0].Score = math.NaN()
	if _, err := Evaluate(s); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("NaN was accepted: %v", err)
	}
	s.Choices[0].Branches[0].Score = 1
	s.Choices = append(s.Choices, s.Choices[0])
	if _, err := Evaluate(s); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate choice accepted: %v", err)
	}
}

func TestEvaluateEnforcesBranchBudgets(t *testing.T) {
	s := Scenario{Title: "T", Choices: []Choice{{ID: "a", Label: "A", Branches: []Branch{{Label: "step", Probability: 1}}}}}
	p := &s.Choices[0].Branches[0]
	for i := 0; i < 8; i++ {
		p.Next = []Branch{{Label: "step", Probability: 1}}
		p = &p.Next[0]
	}
	if _, err := Evaluate(s); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("unbounded depth accepted: %v", err)
	}
}
