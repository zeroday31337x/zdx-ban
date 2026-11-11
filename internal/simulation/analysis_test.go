package simulation

import (
	"math"
	"testing"
)

func TestRiskTrajectoriesAndStressRanking(t *testing.T) {
	s := Scenario{
		Title: "two choices", StressDelta: 0.1,
		Choices: []Choice{
			{ID: "a", Label: "A", Branches: []Branch{
				{Label: "up", Probability: 0.6, Score: 10},
				{Label: "down", Probability: 0.4, Score: -5},
			}},
			{ID: "b", Label: "B", Branches: []Branch{
				{Label: "start", Probability: 1, Score: 3, Next: []Branch{
					{Label: "up", Probability: 0.5, Score: 2},
					{Label: "down", Probability: 0.5, Score: -1},
				}},
			}},
		},
	}
	got, err := Evaluate(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ranking[0].ID != "a" || len(got.StressRanking) != 2 ||
		got.StressRanking[0] != "b" || got.StressRanking[1] != "a" {
		t.Fatalf("unexpected baseline/stress rankings: %+v", got)
	}
	a, b := got.Ranking[0].Risk, got.Ranking[1].Risk
	eq := func(actual, expected float64) bool { return math.Abs(actual-expected) < 1e-10 }
	if !eq(a.P10Score, -5) || !eq(a.MedianScore, 10) || !eq(a.P90Score, 10) ||
		!eq(a.WorstTenPercentMean, -5) || !eq(a.PessimisticShiftExpectedScore, 2.5) {
		t.Fatalf("option a distribution: %+v", a)
	}
	if len(a.ExpectedScoreByStage) != 1 || !eq(a.ExpectedScoreByStage[0], 4) ||
		len(b.ExpectedScoreByStage) != 2 || !eq(b.ExpectedScoreByStage[0], 3) ||
		!eq(b.ExpectedScoreByStage[1], 3.5) || !eq(b.PessimisticShiftExpectedScore, 3.2) {
		t.Fatalf("option trajectories: %+v %+v", a, b)
	}
	if len(a.WorstPaths) != 2 || a.WorstPaths[0].Labels[0] != "down" ||
		len(b.MostLikelyPaths) != 2 || b.MostLikelyPaths[0].CumulativeScores[0] != 3 {
		t.Fatalf("explanation paths: %+v %+v", a, b)
	}
}

func TestRiskStageCarryForwardAndTailBoundary(t *testing.T) {
	s := Scenario{
		Title: "mixed path lengths",
		Choices: []Choice{{ID: "x", Label: "X", Branches: []Branch{
			{Label: "short", Probability: 0.2, Score: -2},
			{Label: "long", Probability: 0.8, Score: 1, Next: []Branch{
				{Label: "finish", Probability: 1, Score: 4},
			}},
		}}},
	}
	got, err := Evaluate(s)
	if err != nil {
		t.Fatal(err)
	}
	v := got.Ranking[0].Risk
	if len(v.ExpectedScoreByStage) != 2 ||
		math.Abs(v.ExpectedScoreByStage[0]-0.4) > 1e-10 ||
		math.Abs(v.ExpectedScoreByStage[1]-3.6) > 1e-10 ||
		math.Abs(v.WorstTenPercentMean-(-2)) > 1e-10 {
		t.Fatalf("bad carried-forward path: %+v", v)
	}
}

func TestStressValidationAndNoStressReport(t *testing.T) {
	s := Scenario{Title: "T", Choices: []Choice{{ID: "x", Label: "X", Branches: []Branch{{Label: "only", Probability: 1, Score: 1}}}}}
	for _, bad := range []float64{-0.01, 0.251, math.NaN(), math.Inf(1)} {
		s.StressDelta = bad
		if _, err := Evaluate(s); err == nil {
			t.Fatalf("accepted stress_delta=%v", bad)
		}
	}
	s.StressDelta = 0
	got, err := Evaluate(s)
	if err != nil || got.StressRanking != nil || got.Ranking[0].Risk.StressDelta != 0 {
		t.Fatalf("unexpected no-stress output: %+v err=%v", got, err)
	}
}

func TestConditionalStressRespectsAvailableProbability(t *testing.T) {
	s := Scenario{Title: "small donor", StressDelta: 0.2, Choices: []Choice{{ID: "x", Label: "X", Branches: []Branch{
		{Label: "good", Probability: 0.05, Score: 10},
		{Label: "bad", Probability: 0.95, Score: 0},
	}}}}
	got, err := Evaluate(s)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Ranking[0].Risk.PessimisticShiftExpectedScore) > 1e-10 {
		t.Fatalf("cannot remove more than donor probability: %+v", got)
	}
}
