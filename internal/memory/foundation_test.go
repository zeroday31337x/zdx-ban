package memory

import (
	"context"
	"testing"
	"zdx-ban/internal/measurement"
)

func TestFoundationMutationRequiresSupersession(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	r := rec("rule", "stable invariant", "system")
	r.Tier = Foundation
	if e := s.Append(ctx, r); e != nil {
		t.Fatal(e)
	}
	changed := r
	changed.Content = "silent rewrite"
	if e := s.Replace(ctx, changed); e == nil {
		t.Fatal("foundation memory silently rewritten")
	}
	replacement := rec("rule-v2", "revised invariant", "system")
	replacement.Tier = Foundation
	replacement.Provenance = Provenance{Source: "explicit revision", SourceClass: MemoryGuidance, Independence: measurement.PartiallyIndependent}
	if _, e := Supersede(ctx, s, r.ID, replacement, "explicit measured revision"); e != nil {
		t.Fatal(e)
	}
}
