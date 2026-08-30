package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"zdx-ban/internal/measurement"
)

type gravityAccumulator struct {
	well         GravityWell
	keywordSet   map[string]bool
	correlations map[string]bool
	feedbackAt   time.Time
}

// EffectiveStrength returns the bounded routing strength represented by the
// well at now. Verification authority remains entirely outside gravity.
func (w GravityWell) EffectiveStrength(now time.Time) float64 {
	baseStrength := w.BaseStrength
	if baseStrength == 0 && w.Strength > 0 {
		// Preserve compatibility with wells serialized before BaseStrength was
		// introduced.
		baseStrength = w.Strength
	}
	base := clampGravity(baseStrength)
	if w.DecayHalfLife > 0 && !w.LastUsedAt.IsZero() && now.After(w.LastUsedAt) {
		halfLives := float64(now.Sub(w.LastUsedAt)) / float64(w.DecayHalfLife)
		base *= math.Pow(0.5, halfLives)
	}
	if w.ApplicationCount > 0 {
		base *= 1 / (1 + 0.15*math.Log1p(float64(w.ApplicationCount)))
	}
	if w.OutcomeCount > 0 {
		base *= 0.4 + 0.6*clampGravity(w.SuccessRate)
	}
	return clampGravity(base)
}

// BuildGravityWells derives routing attractors only from measured memory.
// Seed guidance without measurement provenance remains retrievable, but it
// cannot increase gravity by merely claiming to be supported.
func BuildGravityWells(records []Record, now time.Time, cfg GravityConfig) []GravityWell {
	if !cfg.Enabled {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cfg.EvidenceSaturation <= 0 {
		cfg.EvidenceSaturation = 2
	}
	if cfg.FoundationStrength <= 0 {
		cfg.FoundationStrength = .28
	}
	groups := map[string]*gravityAccumulator{}
	for _, record := range records {
		if record.Category == "" || record.Provenance.SourceClass == ModelAssertion {
			continue
		}
		measurementIDs := map[string]bool{}
		for _, id := range append(append([]string(nil), record.RelatedMeasurementIDs...), record.Provenance.MeasurementIDs...) {
			if id != "" {
				measurementIDs[id] = true
			}
		}
		evidence := len(measurementIDs)
		foundationAnchor := record.Tier == Foundation && record.Metadata != nil && record.Metadata["foundationAnchor"] == true
		// Domain patterns are derived from episodes already present in the
		// store and are not fresh gravity evidence.
		if record.Kind == DomainPattern {
			evidence = 0
		}
		if evidence == 0 && !foundationAnchor {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(record.Category))
		acc := groups[key]
		if acc == nil {
			h := sha256.Sum256([]byte(key))
			acc = &gravityAccumulator{well: GravityWell{ID: "well-" + hex.EncodeToString(h[:8]), Category: record.Category}, keywordSet: map[string]bool{}, correlations: map[string]bool{}}
			groups[key] = acc
		}
		if record.Kind == EpisodeKind {
			var episode Episode
			if json.Unmarshal([]byte(record.Content), &episode) == nil {
				for _, feedback := range episode.GravityFeedback {
					if feedback.WellID != acc.well.ID || feedback.ApplicationCount < 1 || feedback.LastUsedAt.Before(acc.feedbackAt) || feedback.LastUsedAt.Equal(acc.feedbackAt) && feedback.ApplicationCount <= acc.well.ApplicationCount {
						continue
					}
					acc.feedbackAt = feedback.LastUsedAt
					acc.well.LastUsedAt = feedback.LastUsedAt
					acc.well.SuccessRate = clampGravity(feedback.SuccessRate)
					acc.well.ApplicationCount = feedback.ApplicationCount
					acc.well.OutcomeCount = feedback.OutcomeCount
				}
			}
		}
		correlation := record.CorrelationGroup
		if correlation == "" {
			correlation = record.Provenance.CorrelationGroup
		}
		if correlation == "" {
			correlation = record.ID
		}
		if acc.correlations[correlation] {
			continue
		}
		acc.correlations[correlation] = true
		if record.Status == Supported && record.Provenance.Independence != measurement.ModelDerived {
			acc.well.SupportingEvidence += evidence
		} else if record.Status == Contradicted || record.Status == Stale {
			acc.well.ContradictingEvidence += evidence
		}
		if foundationAnchor {
			acc.well.FoundationAnchor = true
		}
		acc.well.ContributingRecordIDs = append(acc.well.ContributingRecordIDs, record.ID)
		if record.UpdatedAt.After(acc.well.UpdatedAt) {
			acc.well.UpdatedAt = record.UpdatedAt
		}
		for token := range gravityTokens(gravityText(record)) {
			acc.keywordSet[token] = true
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wells := make([]GravityWell, 0, len(keys))
	for _, key := range keys {
		acc := groups[key]
		support := float64(acc.well.SupportingEvidence)
		contradiction := float64(acc.well.ContradictingEvidence)
		acc.well.BaseStrength = 1 - math.Exp(-support/cfg.EvidenceSaturation)
		if acc.well.FoundationAnchor && acc.well.BaseStrength < cfg.FoundationStrength {
			acc.well.BaseStrength = cfg.FoundationStrength
		}
		acc.well.DecayHalfLife = cfg.HalfLife
		if acc.well.LastUsedAt.IsZero() {
			acc.well.LastUsedAt = acc.well.UpdatedAt
		}
		acc.well.Strength = acc.well.EffectiveStrength(now)
		acc.well.Repulsion = 1 - math.Exp(-contradiction/cfg.EvidenceSaturation)
		if cfg.HalfLife > 0 && !acc.well.UpdatedAt.IsZero() && now.After(acc.well.UpdatedAt) {
			decay := math.Exp(-math.Ln2 * float64(now.Sub(acc.well.UpdatedAt)) / float64(cfg.HalfLife))
			acc.well.Repulsion *= decay
		}
		for token := range acc.keywordSet {
			acc.well.Keywords = append(acc.well.Keywords, token)
		}
		sort.Strings(acc.well.Keywords)
		sort.Strings(acc.well.ContributingRecordIDs)
		wells = append(wells, acc.well)
	}
	return wells
}

func clampGravity(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func gravityText(record Record) string {
	if record.Kind != EpisodeKind {
		return record.Title + " " + record.Content + " " + record.StrategyType
	}
	var episode Episode
	if json.Unmarshal([]byte(record.Content), &episode) != nil {
		return record.Title + " " + record.StrategyType
	}
	return strings.Join(append([]string{record.Title, record.StrategyType, episode.TaskSignature, episode.Category}, append(episode.Strategies, episode.UsefulBranches...)...), " ")
}

func gravityTokens(value string) map[string]bool {
	out := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(token) > 2 {
			out[token] = true
		}
	}
	return out
}
