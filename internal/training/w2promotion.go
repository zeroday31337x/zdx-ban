package training

import (
	"sort"
	"time"
	"zdx-ban/internal/measurement"
)

// W2PromotionCandidates groups W1Candidate records by Input and, for every
// input independently confirmed Supported across at least `threshold`
// distinct runs, synthesizes one derived Candidate representing that the
// input has cleared the W2 repetition bar.
//
// It never mutates or reclassifies the underlying per-run candidates: this
// is a derived view for a human to review, not an automatic promotion. The
// result's ValidationState is W2Eligible, never Promoted (Candidate.Validate
// rejects Promoted at creation, and DisabledPolicy refuses to advance a
// PromotionState automatically — this function does not touch either).
//
// threshold must be at least 1 and is clamped up to 1 otherwise. A threshold
// of 1 makes every W1Candidate input immediately W2-eligible, which is
// almost certainly not the intent: the whole point of W2 (as opposed to W1,
// which already requires one authoritative Supported measurement) is that
// the same reasoning pattern has been independently reconfirmed across
// multiple separate runs, not just observed once.
func W2PromotionCandidates(candidates []Candidate, threshold int, now time.Time) []Candidate {
	if threshold < 1 {
		threshold = 1
	}
	type group struct {
		runs   map[string]bool
		ids    []string
		latest Candidate
	}
	groups := map[string]*group{}
	order := make([]string, 0)
	for _, c := range candidates {
		if c.Target != W1Candidate {
			continue
		}
		g, ok := groups[c.Input]
		if !ok {
			g = &group{runs: map[string]bool{}}
			groups[c.Input] = g
			order = append(order, c.Input)
		}
		g.runs[c.SourceRun] = true
		g.ids = append(g.ids, c.ID)
		if c.Timestamp.After(g.latest.Timestamp) {
			g.latest = c
		}
	}
	sort.Strings(order)
	out := make([]Candidate, 0, len(order))
	for _, input := range order {
		g := groups[input]
		if len(g.runs) < threshold {
			continue
		}
		ids := append([]string(nil), g.ids...)
		sort.Strings(ids)
		out = append(out, Candidate{
			SchemaVersion:      1,
			ID:                 StableID("w2-promotion", "", input),
			Timestamp:          now,
			SourceRun:          "w2-promotion",
			Input:              input,
			ModelOutput:        g.latest.ModelOutput,
			MeasurementOutcome: measurement.Supported,
			Confidence:         g.latest.Confidence,
			RepetitionCount:    len(g.runs),
			Gravity:            Gravity{Value: g.latest.Gravity.Value, Source: "repetition-gate-w2-eligible"},
			Target:             W2Candidate,
			ValidationState:    W2Eligible,
			Provenance:         Provenance{RunID: "w2-promotion", ModelStateID: g.latest.Provenance.ModelStateID},
			PromotedFrom:       ids,
		})
	}
	return out
}
