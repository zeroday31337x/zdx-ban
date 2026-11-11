package ban

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"
	"zdx-ban/internal/measurement"
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
	// byID lets bridged wells contribute their strength to a matched well.
	byID := make(map[string]int, len(wells))
	for i := range wells {
		if wells[i].ID != "" {
			byID[wells[i].ID] = i
		}
	}
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
		// Bridging: a matched well also pulls in the strength of wells
		// it co-occurred with in verified branches, weighted by the
		// bridge weight. This is how wells combine across domains.
		for targetID, bridgeWeight := range well.Bridges {
			if targetIdx, ok := byID[targetID]; ok && targetIdx != i {
				attraction += clamp(bridgeWeight) * wells[targetIdx].EffectiveStrength(now) * similarity
			}
		}
		attraction = clamp(attraction)
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

// independentMeasuredOutcome returns an independently measured, authoritative
// outcome. A verifier's bare Passed flag is not a measurement, and competing
// branches or model estimates cannot independently confirm one another.
// Contradiction takes priority over support when both are present.
func independentMeasuredOutcome(state *State) measurement.Outcome {
	if state == nil {
		return measurement.NotMeasured
	}
	supported := false
	for _, result := range state.Measurements {
		if result.ID == "" || !result.Authoritative() || result.Independence != measurement.Independent {
			continue
		}
		switch result.Outcome {
		case measurement.Contradicted:
			return measurement.Contradicted
		case measurement.Supported:
			supported = true
		}
	}
	if supported {
		return measurement.Supported
	}
	return measurement.NotMeasured
}

// UpdateGravityOutcome records only independently measured, authoritative
// outcomes for the well that influenced state. AcceptVerifier and other
// pass/fail-only verifiers may select branches, but cannot train gravity.
func UpdateGravityOutcome(wells []GravityWell, state *State, now time.Time) bool {
	if state == nil || state.GravityWellID == "" || state.Status != Verified && state.Status != Failed {
		return false
	}
	outcome := independentMeasuredOutcome(state)
	if (state.Status == Verified && outcome != measurement.Supported) ||
		(state.Status == Failed && outcome != measurement.Contradicted) {
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

// SeedWellsFromVerifiedStates creates gravity wells from a teacher's
// verified traces (distillation). The teacher's successful patterns become
// wells up front, so the student is pulled toward good solutions from the
// start instead of waiting for wells to emerge naturally. Only states that
// are Verified with an independently measured, authoritative Supported
// outcome seed a well; a pass/fail-only verifier cannot seed wells, the
// same rule that governs UpdateGravityOutcome. States with identical
// keyword sets merge into one well whose SupportingEvidence counts them.
// The returned wells are sorted by ID for determinism. Seeding affects
// routing only; it never verifies a branch.
func SeedWellsFromVerifiedStates(states []*State, baseStrength float64) []GravityWell {
	strength := clamp(baseStrength)
	grouped := map[string][]string{}
	support := map[string]int{}
	for _, state := range states {
		if state == nil || state.Status != Verified {
			continue
		}
		if independentMeasuredOutcome(state) != measurement.Supported {
			continue
		}
		tokens := tokenSet(state.Title + " " + state.ReasoningSummary + " " + strings.Join(state.Assumptions, " "))
		if len(tokens) == 0 {
			continue
		}
		keywords := make([]string, 0, len(tokens))
		for token := range tokens {
			keywords = append(keywords, token)
		}
		sort.Strings(keywords)
		key := strings.Join(keywords, " ")
		if _, ok := grouped[key]; !ok {
			grouped[key] = keywords
		}
		support[key]++
	}
	wells := make([]GravityWell, 0, len(grouped))
	for key, keywords := range grouped {
		wells = append(wells, GravityWell{
			ID:                 seededWellID(key),
			Category:           "distilled",
			Strength:           strength,
			Keywords:           keywords,
			SupportingEvidence: support[key],
		})
	}
	sort.Slice(wells, func(i, j int) bool { return wells[i].ID < wells[j].ID })
	return wells
}

func seededWellID(key string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("seeded-%x", h.Sum64())
}

// RecordWellCooccurrence strengthens the bridge between every pair of
// wells in ids, recording that they co-occurred in a verified branch.
// Bridged wells pull in each other's strength when one of them matches,
// so wells combine across domains. The bridge weight is symmetric and
// capped at 1. Call this only for wells that co-occurred in a branch
// whose outcome was independently measured and authoritative.
func RecordWellCooccurrence(wells []GravityWell, ids []string, increment float64) {
	if len(ids) < 2 || increment <= 0 {
		return
	}
	byID := make(map[string]int, len(wells))
	for i := range wells {
		if wells[i].ID != "" {
			byID[wells[i].ID] = i
		}
	}
	// Deduplicate ids so a repeated id does not bridge to itself.
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	for a := 0; a < len(unique); a++ {
		for b := a + 1; b < len(unique); b++ {
			ia, okA := byID[unique[a]]
			ib, okB := byID[unique[b]]
			if !okA || !okB || ia == ib {
				continue
			}
			if wells[ia].Bridges == nil {
				wells[ia].Bridges = make(map[string]float64)
			}
			if wells[ib].Bridges == nil {
				wells[ib].Bridges = make(map[string]float64)
			}
			wells[ia].Bridges[unique[b]] = clamp(wells[ia].Bridges[unique[b]] + increment)
			wells[ib].Bridges[unique[a]] = clamp(wells[ib].Bridges[unique[a]] + increment)
		}
	}
}

// PruneWells drops dead wells so the well set stays clean. A well is
// pruned when its effective strength has decayed below minStrength, or
// when it has at least minOutcomes recorded outcomes and its success rate
// is below minSuccessRate. Wells with fewer than minOutcomes outcomes are
// never pruned by the success-rate rule, so newly seeded wells survive.
// The input slice is not modified; a new slice is returned.
func PruneWells(wells []GravityWell, now time.Time, minStrength float64, minSuccessRate float64, minOutcomes int) []GravityWell {
	kept := make([]GravityWell, 0, len(wells))
	for _, well := range wells {
		if well.EffectiveStrength(now) < minStrength {
			continue
		}
		if minOutcomes > 0 && well.OutcomeCount >= minOutcomes && well.SuccessRate < minSuccessRate {
			continue
		}
		kept = append(kept, well)
	}
	return kept
}
