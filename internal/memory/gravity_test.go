package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"zdx-ban/internal/measurement"
)

func measuredGravityRecord(id, correlation string, status Status, at time.Time) Record {
	record := NewRecord(id, Episodic, EpisodeKind, "verified grouped arithmetic", "multiply the grouped terms before subtraction", Provenance{
		Source:           "measured test",
		SourceClass:      MemoryGuidance,
		Independence:     measurement.PartiallyIndependent,
		CorrelationGroup: correlation,
		MeasurementIDs:   []string{"measurement-" + id},
		CreatedAt:        at,
	})
	record.Category = "arithmetic_constraints"
	record.StrategyType = "grouped-arithmetic"
	record.CorrelationGroup = correlation
	record.Status = status
	return record
}

func TestGravityWellStrengthensWithDistinctVerifiedEvidence(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	one := BuildGravityWells([]Record{measuredGravityRecord("one", "a", Supported, now)}, now, DefaultGravityConfig())
	two := BuildGravityWells([]Record{measuredGravityRecord("one", "a", Supported, now), measuredGravityRecord("two", "b", Supported, now)}, now, DefaultGravityConfig())
	if len(one) != 1 || len(two) != 1 || two[0].Strength <= one[0].Strength {
		t.Fatalf("gravity did not strengthen: one=%+v two=%+v", one, two)
	}
	duplicate := BuildGravityWells([]Record{measuredGravityRecord("one", "a", Supported, now), measuredGravityRecord("two", "a", Supported, now)}, now, DefaultGravityConfig())
	if duplicate[0].SupportingEvidence != one[0].SupportingEvidence {
		t.Fatalf("correlated repetition inflated evidence: one=%+v duplicate=%+v", one[0], duplicate[0])
	}
	domain := measuredGravityRecord("domain", "domain", Supported, now)
	domain.Kind = DomainPattern
	domain.Provenance.ContributingRecordIDs = []string{"one", "two"}
	withConsolidation := BuildGravityWells([]Record{measuredGravityRecord("one", "a", Supported, now), measuredGravityRecord("two", "b", Supported, now), domain}, now, DefaultGravityConfig())
	if withConsolidation[0].SupportingEvidence != two[0].SupportingEvidence {
		t.Fatalf("derived consolidation double-counted evidence: base=%+v derived=%+v", two[0], withConsolidation[0])
	}
}

func TestGravityDecaysAndContradictionRepels(t *testing.T) {
	now := time.Unix(100000, 0).UTC()
	cfg := DefaultGravityConfig()
	cfg.HalfLife = time.Hour
	fresh := BuildGravityWells([]Record{measuredGravityRecord("good", "a", Supported, now)}, now, cfg)[0]
	old := BuildGravityWells([]Record{measuredGravityRecord("good", "a", Supported, now.Add(-time.Hour))}, now, cfg)[0]
	mixed := BuildGravityWells([]Record{measuredGravityRecord("good", "a", Supported, now), measuredGravityRecord("bad", "b", Contradicted, now)}, now, cfg)[0]
	if old.Strength >= fresh.Strength || mixed.Repulsion <= 0 {
		t.Fatalf("decay/repulsion missing: fresh=%+v old=%+v mixed=%+v", fresh, old, mixed)
	}
}

func TestGravityFeedbackSurvivesWellRebuild(t *testing.T) {
	now := time.Unix(200_000, 0).UTC()
	record := measuredGravityRecord("feedback", "a", Supported, now)
	initial := BuildGravityWells([]Record{record}, now, DefaultGravityConfig())[0]
	episode := Episode{
		Problem:       "measured problem",
		TaskSignature: "arithmetic",
		Category:      record.Category,
		Strategies:    []string{record.StrategyType},
		Timestamp:     now,
		GravityFeedback: []GravityFeedback{{
			WellID:           initial.ID,
			LastUsedAt:       now,
			SuccessRate:      0,
			ApplicationCount: 3,
			OutcomeCount:     3,
		}},
	}
	encoded, _ := json.Marshal(episode)
	record.Content = string(encoded)
	restored := BuildGravityWells([]Record{record}, now, DefaultGravityConfig())[0]
	if restored.ApplicationCount != 3 || restored.OutcomeCount != 3 || restored.SuccessRate != 0 || !restored.LastUsedAt.Equal(now) {
		t.Fatalf("gravity feedback was not restored: %+v", restored)
	}
	if restored.Strength >= initial.Strength {
		t.Fatalf("persisted failed usage did not weaken well: initial=%+v restored=%+v", initial, restored)
	}
}

func TestFoundationAnchorBootstrapsButOrganicEvidenceOutranks(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	anchor := NewRecord("anchor", Foundation, StructuralRule, "foundation arithmetic", "preserve operation structure", Provenance{SourceClass: MemoryGuidance, CreatedAt: now})
	anchor.Category = "arithmetic_constraints"
	anchor.Metadata = map[string]any{"foundationAnchor": true}
	anchor.Status = Active
	anchorWell := BuildGravityWells([]Record{anchor}, now, DefaultGravityConfig())
	if len(anchorWell) != 1 || !anchorWell[0].FoundationAnchor || anchorWell[0].Strength != DefaultGravityConfig().FoundationStrength {
		t.Fatalf("foundation anchor missing or too strong: %+v", anchorWell)
	}
	organic := measuredGravityRecord("organic", "organic", Supported, now)
	wells := BuildGravityWells([]Record{anchor, organic}, now, DefaultGravityConfig())
	if len(wells) != 1 || wells[0].Strength <= anchorWell[0].Strength || wells[0].SupportingEvidence != 1 {
		t.Fatalf("organic evidence did not outrank anchor: %+v", wells)
	}
}

func TestRetrievalRoutesByMeasuredInformationGravityOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()
	store := NewMemoryStore()
	measured := measuredGravityRecord("measured", "a", Supported, now)
	if err := store.Append(ctx, measured); err != nil {
		t.Fatal(err)
	}
	unmeasured := NewRecord("claim", Episodic, Strategy, "claimed arithmetic", "multiply grouped terms", Provenance{Source: "claim", SourceClass: MemoryGuidance, Independence: measurement.PartiallyIndependent, CreatedAt: now})
	unmeasured.Category = measured.Category
	unmeasured.Status = Supported
	if err := store.Append(ctx, unmeasured); err != nil {
		t.Fatal(err)
	}
	result, err := Retrieve(ctx, store, RetrievalRequest{Query: "compute grouped multiplication", Category: measured.Category, Now: now, Config: DefaultRetrievalConfig()})
	if err != nil || len(result.GravityWells) != 1 || result.GravityWells[0].SupportingEvidence != 1 {
		t.Fatalf("unexpected wells: result=%+v err=%v", result.GravityWells, err)
	}
	for _, item := range result.Records {
		if item.Reason.InformationGravity <= 0 {
			t.Fatalf("relevant record was not routed through gravity: %+v", item)
		}
	}
}

func TestEpisodeLeakGuardChecksExposedRouteNotAuditSubstrings(t *testing.T) {
	episode := Episode{Problem: "prior prompt", Category: "arithmetic_constraints", UsefulBranches: []string{"group multiplication before subtraction"}, Outcome: string(measurement.Supported), ModelCalls: 130, Timestamp: time.Unix(130, 0).UTC()}
	encoded, _ := json.Marshal(episode)
	record := measuredGravityRecord("episode", "a", Supported, time.Unix(1, 0).UTC())
	record.Content = string(encoded)
	request := RetrievalRequest{Config: DefaultRetrievalConfig(), ExcludeExactAnswer: "130", Now: time.Unix(1000, 0).UTC()}
	if !retrievalEligible(record, request) {
		t.Fatal("audit-only numeric substring incorrectly excluded verified route")
	}
	episode.UsefulBranches = []string{"calculation produces 130"}
	encoded, _ = json.Marshal(episode)
	record.Content = string(encoded)
	if retrievalEligible(record, request) {
		t.Fatal("model-visible exact answer leakage was not excluded")
	}
}
