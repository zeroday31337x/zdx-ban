// Package simulation evaluates bounded, user-supplied decision trees.
// It never invents probabilities, executes external actions, or treats scores
// as calibrated predictions of real-world outcomes.
package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	maxChoices = 32
	maxDepth   = 8
	maxFanout  = 32
	maxPaths   = 8192
)

type Scenario struct {
	Title   string   `json:"title"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Branches []Branch `json:"branches"`
}

type Branch struct {
	Label       string   `json:"label"`
	Probability float64  `json:"probability"`
	Score       float64  `json:"score"`
	Next        []Branch `json:"next,omitempty"`
}

type ChoiceResult struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	ExpectedScore   float64 `json:"expected_score"`
	LossProbability float64 `json:"loss_probability"`
	BestScore       float64 `json:"best_score"`
	WorstScore      float64 `json:"worst_score"`
	TerminalPaths   int     `json:"terminal_paths"`
}

type Report struct {
	Title             string         `json:"title"`
	Status            string         `json:"status"`
	ProbabilitySource string         `json:"probability_source"`
	Ranking           []ChoiceResult `json:"ranking"`
}

// Evaluate enumerates terminal outcomes and ranks choices by expected score.
// Scores and conditional probabilities come solely from the caller.
func Evaluate(s Scenario) (Report, error) {
	if strings.TrimSpace(s.Title) == "" || len(s.Title) > 256 {
		return Report{}, fmt.Errorf("title must be 1-256 characters")
	}
	if len(s.Choices) == 0 || len(s.Choices) > maxChoices {
		return Report{}, fmt.Errorf("choices must contain 1-%d entries", maxChoices)
	}
	report := Report{Title: s.Title, Status: "ASSUMPTION_BASED", ProbabilitySource: "USER_SUPPLIED"}
	ids := make(map[string]bool, len(s.Choices))
	for _, choice := range s.Choices {
		if strings.TrimSpace(choice.ID) == "" || len(choice.ID) > 128 ||
			strings.TrimSpace(choice.Label) == "" || len(choice.Label) > 256 {
			return Report{}, fmt.Errorf("choice id and label must be nonempty and bounded")
		}
		if ids[choice.ID] {
			return Report{}, fmt.Errorf("duplicate choice id %q", choice.ID)
		}
		ids[choice.ID] = true
		result := ChoiceResult{ID: choice.ID, Label: choice.Label, BestScore: math.Inf(-1), WorstScore: math.Inf(1)}
		if err := walk(choice.Branches, 1, 1, 0, &result); err != nil {
			return Report{}, fmt.Errorf("choice %q: %w", choice.ID, err)
		}
		report.Ranking = append(report.Ranking, result)
	}
	sort.Slice(report.Ranking, func(i, j int) bool {
		a, b := report.Ranking[i], report.Ranking[j]
		if a.ExpectedScore == b.ExpectedScore {
			return a.ID < b.ID
		}
		return a.ExpectedScore > b.ExpectedScore
	})
	return report, nil
}

func walk(branches []Branch, depth int, probability, score float64, out *ChoiceResult) error {
	if depth > maxDepth || len(branches) == 0 || len(branches) > maxFanout {
		return fmt.Errorf("tree must have 1-%d branches per level and depth <= %d", maxFanout, maxDepth)
	}
	sum := 0.0
	for _, branch := range branches {
		if strings.TrimSpace(branch.Label) == "" || len(branch.Label) > 256 ||
			math.IsNaN(branch.Probability) || math.IsInf(branch.Probability, 0) ||
			branch.Probability <= 0 || branch.Probability > 1 ||
			math.IsNaN(branch.Score) || math.IsInf(branch.Score, 0) || math.Abs(branch.Score) > 1e9 {
			return fmt.Errorf("invalid branch label, probability, or score at depth %d", depth)
		}
		sum += branch.Probability
		nextP := probability * branch.Probability
		nextScore := score + branch.Score
		if len(branch.Next) > 0 {
			if err := walk(branch.Next, depth+1, nextP, nextScore, out); err != nil {
				return err
			}
			continue
		}
		out.TerminalPaths++
		if out.TerminalPaths > maxPaths {
			return fmt.Errorf("more than %d terminal paths", maxPaths)
		}
		out.ExpectedScore += nextP * nextScore
		if nextScore < 0 {
			out.LossProbability += nextP
		}
		out.BestScore = math.Max(out.BestScore, nextScore)
		out.WorstScore = math.Min(out.WorstScore, nextScore)
	}
	if math.Abs(sum-1) > 1e-6 {
		return fmt.Errorf("branch probabilities at depth %d sum to %.8f, expected 1", depth, sum)
	}
	return nil
}
