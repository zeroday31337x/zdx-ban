package experiment

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
)

func verificationWith(o measurement.Outcome) Verification {
	return Verification{Measurement: measurement.Result{Outcome: o}}
}

func TestEpistemicOutcomesRemainDistinct(t *testing.T) {
	want := map[measurement.Outcome]EpistemicOutcome{measurement.Supported: EpistemicSupported, measurement.Contradicted: EpistemicContradicted, measurement.Inconclusive: EpistemicInconclusive, measurement.Unsupported: EpistemicUnsupported, measurement.NotMeasured: EpistemicUnknown, measurement.Error: EpistemicFailed}
	seen := map[EpistemicOutcome]bool{}
	for in, out := range want {
		got := classifyEpistemic(verificationWith(in))
		if got != out {
			t.Fatalf("%s became %s", in, got)
		}
		seen[got] = true
	}
	if len(seen) != 6 {
		t.Fatal("outcomes collapsed")
	}
}
func TestMemoryEffectsAndAuthoritativeOverride(t *testing.T) {
	r := MemoryCaseResult{Cold: SideResult{Verification: verificationWith(measurement.Contradicted)}, WithMemory: SideResult{Verification: verificationWith(measurement.Supported)}, Misleading: SideResult{Verification: verificationWith(measurement.Supported)}, MemoryAttribution: MemoryAttribution{MemoryRetrieved: true}, MisleadingAttribution: MemoryAttribution{MemoryRetrieved: true, EvidenceOverride: true, MemoryConflicted: []string{"stale"}}, MisleadingPair: PairedResult{RecoveryAttempted: true, RecoverySuccessful: true}}
	classifyMemoryEffects(&r)
	if r.MemoryAttribution.MemoryEffect != EffectHelpful || r.MisleadingAttribution.MemoryEffect != EffectRecovered {
		t.Fatalf("bad effects %+v", r)
	}
	if len(r.MisleadingAttribution.MemoryRejected) != 1 {
		t.Fatal("authoritative conflict not rejected")
	}
}
func TestRawJSONLRegeneratesAndCompares(t *testing.T) {
	rows := []MemoryRawResult{{CaseID: "x", Repetition: 1, Condition: ColdCondition, Result: EpistemicContradicted, Category: "logic"}, {CaseID: "x", Repetition: 1, Condition: MemoryConditionEnabled, Result: EpistemicSupported, Category: "logic", Attribution: MemoryAttribution{MemoryRetrieved: true, MemoryEffect: EffectHelpful}}}
	p := filepath.Join(t.TempDir(), "raw.jsonl")
	if err := writeMemoryRowsAtomic(p, rows); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMemoryRows(p)
	if err != nil || len(got) != 2 {
		t.Fatalf("load %v %d", err, len(got))
	}
	a := AggregateMemoryRows(got)
	if a.Helpful != 1 || a.ByCondition[MemoryConditionEnabled][EpistemicSupported] != 1 {
		t.Fatalf("bad aggregate %+v", a)
	}
	c := CompareMemoryRows(got, ColdCondition, MemoryConditionEnabled)
	if c.Improved != 1 {
		t.Fatalf("bad comparison %+v", c)
	}
	report := filepath.Join(t.TempDir(), "report.md")
	if err := WriteMemoryRawReport(report, got); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(b), "BAN_COLD -> BAN_MEMORY") {
		t.Fatalf("report regeneration failed: %v", err)
	}
}
func TestConditionSeedStoresAreIsolated(t *testing.T) {
	seed := SeedMemory{ID: "one", Title: "x", Content: "historical strategy", Tier: memory.Episodic, Kind: memory.Strategy, Status: memory.Supported, SourceClass: memory.MemoryGuidance}
	a, _ := SeedStore(context.Background(), []SeedMemory{seed}, false)
	b, _ := SeedStore(context.Background(), []SeedMemory{seed}, false)
	r := memory.NewRecord("two", memory.Episodic, memory.Strategy, "y", "other", memory.Provenance{CreatedAt: time.Unix(0, 0)})
	if err := a.Append(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	br, _ := b.List(context.Background())
	if len(br) != 1 {
		t.Fatal("condition store leaked")
	}
}
func TestTierTwoPromotionDoesNotIncreaseAuthority(t *testing.T) {
	s := memory.NewMemoryStore()
	for i := 0; i < 2; i++ {
		p := memory.Provenance{SourceClass: memory.MemoryGuidance, Independence: measurement.PartiallyIndependent, CorrelationGroup: string(rune('a' + i)), CreatedAt: time.Unix(int64(i), 0)}
		r := memory.NewRecord(string(rune('a'+i)), memory.Episodic, memory.EpisodeKind, "e", "history", p)
		r.Status = memory.Supported
		r.StrategyType = "bounded-check"
		r.CorrelationGroup = p.CorrelationGroup
		if err := s.Append(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	events, err := memory.Consolidate(context.Background(), s, memory.ConsolidationConfig{MinDistinctEpisodes: 2})
	if err != nil || len(events) == 0 || events[0].CreatedRecordID == "" {
		t.Fatalf("promotion failed %v %+v", err, events)
	}
	rec, _, _ := s.Get(context.Background(), events[0].CreatedRecordID)
	if rec.Provenance.SourceClass != memory.MemoryGuidance || rec.Provenance.Independence == measurement.Independent || rec.MeasurementAuthority == measurement.Formal {
		t.Fatal("promotion manufactured authority")
	}
}
