package memory

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"zdx-ban/internal/measurement"
)

func rec(id, content, cat string) Record {
	p := Provenance{Source: "test", SourceClass: MemoryGuidance, Independence: measurement.PartiallyIndependent, CorrelationGroup: id, CreatedAt: time.Unix(1, 0)}
	r := NewRecord(id, Episodic, Strategy, id, content, p)
	r.Category = cat
	r.Tags = []string{cat}
	r.CreatedAt = time.Unix(1, 0)
	r.UpdatedAt = time.Unix(1, 0)
	r.Status = Supported
	return r
}
func TestWorkingMemoryBoundsAndDeterminism(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	for _, r := range []Record{rec("b", "debug queue strategy", "debug"), rec("a", "debug memory strategy", "debug"), rec("c", "unrelated", "other")} {
		if e := s.Append(ctx, r); e != nil {
			t.Fatal(e)
		}
	}
	cfg := DefaultRetrievalConfig()
	cfg.MaxRecords = 1
	cfg.MaxChars = 100
	q := RetrievalRequest{Query: "debug memory", Category: "debug", Tags: []string{"debug"}, Now: time.Unix(2, 0), Config: cfg}
	a, _ := Retrieve(ctx, s, q)
	b, _ := Retrieve(ctx, s, q)
	if len(a.Records) != 1 || a.ApproxChars > 100 || !reflect.DeepEqual(a, b) {
		t.Fatalf("unbounded/nondeterministic %+v %+v", a, b)
	}
}
func TestMemoryIsNotIndependentCurrentEvidence(t *testing.T) {
	r := rec("x", "guidance", "c")
	r.Provenance.Independence = measurement.Independent
	if NewMemoryStore().Append(context.Background(), r) == nil {
		t.Fatal("memory guidance accepted as independent evidence")
	}
}
func TestAuthoritativeContradictionAndHistory(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	r := rec("x", "old strategy", "c")
	if e := s.Append(ctx, r); e != nil {
		t.Fatal(e)
	}
	m := measurement.Result{ID: "m", Outcome: measurement.Contradicted, Authority: measurement.Formal}
	ev, e := ApplyCurrentMeasurement(ctx, s, "x", m)
	if e != nil || ev.NewStatus != Contradicted {
		t.Fatal(e, ev)
	}
	got, _, _ := s.Get(ctx, "x")
	if got.Content != "old strategy" || got.Status != Contradicted || len(got.RelatedMeasurementIDs) != 1 {
		t.Fatal("history destroyed", got)
	}
}
func TestInconclusiveAndErrorPreserveMemory(t *testing.T) {
	for _, o := range []measurement.Outcome{measurement.Inconclusive, measurement.Error} {
		ctx := context.Background()
		s := NewMemoryStore()
		r := rec(string(o), "keep", "c")
		s.Append(ctx, r)
		ev, e := ApplyCurrentMeasurement(ctx, s, r.ID, measurement.Result{ID: "m", Outcome: o, Authority: measurement.Formal})
		if e != nil || ev.NewStatus != Supported {
			t.Fatalf("%s erased memory: %+v %v", o, ev, e)
		}
	}
}
func TestProvenanceRetrievalAndFailedGuidance(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	r := rec("fail", "avoid this", "debug")
	r.Kind = FailurePattern
	r.Status = Contradicted
	r.Provenance.TraceID = "trace"
	s.Append(ctx, r)
	cfg := DefaultRetrievalConfig()
	got, _ := Retrieve(ctx, s, RetrievalRequest{Query: "avoid debug", Category: "debug", Now: time.Now(), Config: cfg})
	if len(got.Records) != 1 || got.Records[0].Record.Provenance.TraceID != "trace" || !got.Records[0].Reason.FailureRelevant {
		t.Fatalf("failed provenance lost %+v", got)
	}
}
func TestEpisodeStoresSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	for i, out := range []string{string(measurement.Supported), string(measurement.Contradicted)} {
		e := Episode{Problem: string(rune('a' + i)), TaskSignature: "sig", Outcome: out, FailedStrategies: []string{"bad"}, Timestamp: time.Unix(int64(i+1), 0)}
		if _, er := RecordEpisode(ctx, s, e, Provenance{CorrelationGroup: out}); er != nil {
			t.Fatal(er)
		}
	}
	records, _ := s.List(ctx)
	if len(records) != 2 || records[0].Status == records[1].Status {
		t.Fatalf("episodes missing %+v", records)
	}
}
func TestModelDerivedRepetitionDoesNotConsolidate(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	for _, id := range []string{"a", "b"} {
		r := rec(id, "pattern", "c")
		r.StrategyType = "p"
		r.Provenance.SourceClass = ModelAssertion
		r.Provenance.Independence = measurement.ModelDerived
		r.CorrelationGroup = id
		s.Append(ctx, r)
	}
	events, _ := Consolidate(ctx, s, ConsolidationConfig{MinDistinctEpisodes: 2})
	if len(events) != 0 {
		t.Fatal("model repetition promoted")
	}
}
func TestConsolidationDistinctCorrelationAndContradiction(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	for _, id := range []string{"a", "b"} {
		r := rec(id, "pattern", "c")
		r.StrategyType = "p"
		r.CorrelationGroup = id
		s.Append(ctx, r)
	}
	events, e := Consolidate(ctx, s, ConsolidationConfig{2})
	if e != nil || len(events) != 1 || events[0].CreatedRecordID == "" {
		t.Fatalf("consolidation failed %+v %v", events, e)
	}
}
func TestResetAndStableHash(t *testing.T) {
	ctx := context.Background()
	a, b := NewMemoryStore(), NewMemoryStore()
	for _, s := range []*MemoryStore{a, b} {
		s.Append(ctx, rec("x", "v", "c"))
	}
	ha, _ := a.SnapshotHash(ctx)
	hb, _ := b.SnapshotHash(ctx)
	if ha != hb {
		t.Fatal("hash unstable")
	}
	a.Reset(ctx)
	v, _ := a.List(ctx)
	if len(v) != 0 {
		t.Fatal("reset contaminated")
	}
}
func TestJSONLCorruptionFailsSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.jsonl")
	s, e := OpenJSONL(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Append(context.Background(), rec("x", "v", "c")); e != nil {
		t.Fatal(e)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("{truncated")
	f.Close()
	if _, e = OpenJSONL(path); e == nil {
		t.Fatal("corruption accepted")
	}
}
func TestSupersessionPreservesOldRecord(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	old := rec("old", "old content", "c")
	s.Append(ctx, old)
	newr := rec("new", "new content", "c")
	if _, e := Supersede(ctx, s, "old", newr, "current measurement"); e != nil {
		t.Fatal(e)
	}
	got, _, _ := s.Get(ctx, "old")
	if got.Status != Superseded || got.Content != "old content" || got.SupersededBy != "new" {
		t.Fatal(got)
	}
}
