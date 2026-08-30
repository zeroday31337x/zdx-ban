package ban

import (
	"math"
	"strings"
	"time"
)

const gravitySuccessEMAAlpha = 0.2

// EffectiveStrength applies bounded recency, usage, and observed-success
// modulation to a well's evidence-derived strength. These factors affect
// routing only; they never change a branch's verification status.
func (w GravityWell) EffectiveStrength(now time.Time) float64 {
	base := clamp(w.Strength)
	if w.DecayHalfLife > 0 && !w.LastUsedAt.IsZero() && now.After(w.LastUsedAt) {
		halfLives := float64(now.Sub(w.LastUsedAt)) / float64(w.DecayHalfLife)
		base *= math.Pow(0.5, halfLives)
	}
	if w.ApplicationCount > 0 {
		base *= 1 / (1 + 0.15*math.Log1p(float64(w.ApplicationCount)))
	}
	if w.OutcomeCount > 0 {
		// OutcomeCount distinguishes an optional/unset rate from a measured
		// zero-percent rate, which must still reduce strength.
		base *= 0.4 + 0.6*clamp(w.SuccessRate)
	}
	return clamp(base)
}

// ApplyGravity blends historical routing evidence into a model-evaluated
// branch. It cannot mark a branch verified and is intentionally bounded.
func ApplyGravity(state *State, wells []GravityWell, weight float64) bool {
	return applyGravityAt(state, wells, weight, time.Now().UTC())
}

func applyGravityAt(state *State, wells []GravityWell, weight float64, now time.Time) bool {
	if state == nil || state.Status == Pruned || weight <= 0 || len(wells) == 0 {
		return false
	}
	branchTokens := tokenSet(state.Title + " " + state.ReasoningSummary + " " + strings.Join(state.Assumptions, " "))
	bestNet := 0.0
	bestAttraction := 0.0
	bestRepulsion := 0.0
	bestID := ""
	bestIndex := -1
	for i := range wells {
		well := wells[i]
		wellTokens := map[string]bool{}
		for _, token := range well.Keywords {
			wellTokens[token] = true
		}
		similarity := tokenOverlap(branchTokens, wellTokens)
		attraction := similarity * well.EffectiveStrength(now)
		repulsion := similarity * clamp(well.Repulsion)
		net := attraction - repulsion
		if bestID == "" || net > bestNet {
			bestID = well.ID
			bestNet = net
			bestAttraction = attraction
			bestRepulsion = repulsion
			bestIndex = i
		}
	}
	if bestID == "" || (bestAttraction == 0 && bestRepulsion == 0) {
		return false
	}
	state.GravityWellID = bestID
	state.InformationGravity = bestAttraction
	state.GravityRepulsion = bestRepulsion
	state.AggregateScore = clamp(state.AggregateScore + clamp(weight)*(bestAttraction-bestRepulsion))
	wells[bestIndex].LastUsedAt = now.UTC()
	wells[bestIndex].ApplicationCount++
	return true
}

// UpdateGravityOutcome records an authoritative terminal outcome for the well
// that influenced state. Inconclusive and un-routed states do not train wells.
func UpdateGravityOutcome(wells []GravityWell, state *State, now time.Time) bool {
	if state == nil || state.GravityWellID == "" || state.Status != Verified && state.Status != Failed {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	for i := range wells {
		well := &wells[i]
		if well.ID != state.GravityWellID {
			continue
		}
		sample := 0.0
		if state.Status == Verified {
			sample = 1
		}
		if well.OutcomeCount == 0 {
			well.SuccessRate = sample
		} else {
			well.SuccessRate = ema(well.SuccessRate, sample, gravitySuccessEMAAlpha)
		}
		well.OutcomeCount++
		return true
	}
	return false
}

func ema(current, sample, alpha float64) float64 {
	alpha = clamp(alpha)
	return clamp(alpha*clamp(sample) + (1-alpha)*clamp(current))
}

func tokenSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, token := range strings.Fields(nonWord.ReplaceAllString(strings.ToLower(value), " ")) {
		if len(token) > 2 {
			out[token] = true
		}
	}
	return out
}

func tokenOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hits := 0
	for token := range a {
		if b[token] {
			hits++
		}
	}
	denominator := len(a)
	if len(b) > denominator {
		denominator = len(b)
	}
	return float64(hits) / float64(denominator)
}
