package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"zdx-ban/internal/measurement"
)

func RecordEpisode(ctx context.Context, s Store, e Episode, p Provenance) (Record, error) {
	if e.Problem == "" || e.Timestamp.IsZero() {
		return Record{}, fmt.Errorf("incomplete episode")
	}
	b, _ := json.Marshal(e)
	h := sha256.Sum256(b)
	id := "episode-" + hex.EncodeToString(h[:8])
	p.SourceClass = MemoryGuidance
	p.CreatedAt = e.Timestamp
	r := NewRecord(id, Episodic, EpisodeKind, "Experience: "+e.TaskSignature, string(b), p)
	r.RelatedMeasurementIDs = append([]string(nil), p.MeasurementIDs...)
	r.Category = e.Category
	r.Tags = append([]string{"episode"}, e.Strategies...)
	if len(e.Strategies) > 0 {
		r.StrategyType = e.Strategies[0]
	}
	r.Metadata = map[string]any{"outcome": e.Outcome, "failureClassification": e.FailureClassification, "recovered": e.Recovered}
	if e.Outcome == string(measurement.Supported) {
		r.Status = Supported
	} else if e.Outcome == string(measurement.Contradicted) {
		r.Status = Contradicted
	} else {
		r.Status = Uncertain
	}
	if er := s.Append(ctx, r); er != nil {
		return Record{}, er
	}
	return r, nil
}
func Consolidate(ctx context.Context, s Store, c ConsolidationConfig) ([]ConsolidationRecord, error) {
	if c.MinDistinctEpisodes < 2 {
		c.MinDistinctEpisodes = 2
	}
	records, e := s.List(ctx)
	if e != nil {
		return nil, e
	}
	groups := map[string][]Record{}
	for _, r := range records {
		if r.Tier == Episodic && r.StrategyType != "" && r.Status == Supported && r.Provenance.SourceClass != ModelAssertion && r.Provenance.Independence != measurement.ModelDerived {
			groups[r.StrategyType] = append(groups[r.StrategyType], r)
		}
	}
	var out []ConsolidationRecord
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, pattern := range keys {
		items := groups[pattern]
		event := ConsolidationRecord{Pattern: pattern, At: time.Now().UTC()}
		correlations := map[string]bool{}
		contradicted := false
		for _, r := range records {
			if r.StrategyType == pattern && r.Status == Contradicted {
				contradicted = true
			}
		}
		for _, r := range items {
			correlations[r.CorrelationGroup] = true
			event.ContributingEpisodeIDs = append(event.ContributingEpisodeIDs, r.ID)
		}
		if contradicted {
			event.SkippedReason = "authoritative contradiction exists"
		} else if len(correlations) < c.MinDistinctEpisodes {
			event.SkippedReason = "insufficient distinct correlation groups"
		} else {
			id := "domain-" + shortHash(pattern+strings.Join(event.ContributingEpisodeIDs, ","))
			p := Provenance{Source: "deterministic consolidation", SourceClass: MemoryGuidance, ContributingRecordIDs: event.ContributingEpisodeIDs, Authority: measurement.DeterministicRuntime, Independence: measurement.PartiallyIndependent, CreatedAt: event.At}
			r := NewRecord(id, Domain, DomainPattern, "Reusable strategy: "+pattern, "Prior distinct measured episodes supported strategy "+pattern, p)
			r.Status = Supported
			r.StrategyType = pattern
			r.CorrelationGroup = id
			if er := s.Append(ctx, r); er != nil {
				return out, er
			}
			event.CreatedRecordID = id
		}
		out = append(out, event)
	}
	return out, nil
}
func shortHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:8]) }
