package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, v := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(v) > 1 {
			out[v] = true
		}
	}
	return out
}
func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hit := 0
	for x := range a {
		if b[x] {
			hit++
		}
	}
	den := len(a)
	if len(b) > den {
		den = len(b)
	}
	return float64(hit) / float64(den)
}
func included(t Tier, v []Tier) bool {
	for _, x := range v {
		if x == t {
			return true
		}
	}
	return false
}
func tierScore(t Tier) float64 {
	switch t {
	case Foundation:
		return 1
	case Domain:
		return .8
	default:
		return .5
	}
}
func Retrieve(ctx context.Context, s Store, q RetrievalRequest) (RetrievalResult, error) {
	if q.Now.IsZero() {
		q.Now = time.Now().UTC()
	}
	if q.Config.MaxRecords <= 0 || q.Config.MaxChars <= 0 {
		return RetrievalResult{Request: q}, nil
	}
	records, e := s.List(ctx)
	if e != nil {
		return RetrievalResult{}, e
	}
	queryTokens := tokens(strings.Join([]string{q.Query, q.Category, q.StrategyType, strings.Join(q.Tags, " ")}, " "))
	var candidates []Retrieved
	for _, r := range records {
		if r.Status == Superseded || r.Status == Stale {
			continue
		}
		if !included(r.Tier, q.Config.IncludeTiers) {
			continue
		}
		if q.Config.MaxAge > 0 && q.Now.Sub(r.UpdatedAt) > q.Config.MaxAge {
			continue
		}
		if q.ExcludeCaseID != "" && r.Provenance.CaseID == q.ExcludeCaseID {
			continue
		}
		if q.ExcludeExactAnswer != "" && strings.Contains(strings.ToLower(r.Content), strings.ToLower(q.ExcludeExactAnswer)) {
			continue
		}
		reason := RetrievalReason{Tier: r.Tier}
		reason.TokenOverlap = overlap(queryTokens, tokens(r.Title+" "+r.Content+" "+strings.Join(r.Tags, " ")))
		reason.CategoryMatch = q.Category != "" && r.Category == q.Category
		reason.StrategyMatch = q.StrategyType != "" && r.StrategyType == q.StrategyType
		for _, qt := range q.Tags {
			for _, rt := range r.Tags {
				if strings.EqualFold(qt, rt) {
					reason.TagMatches = append(reason.TagMatches, qt)
				}
			}
		}
		reason.Supported = r.Status == Supported
		reason.FailureRelevant = r.Kind == FailurePattern || len(r.Metadata) > 0 && r.Metadata["outcome"] == "failure"
		age := q.Now.Sub(r.UpdatedAt)
		reason.Recency = 1 / (1 + age.Hours()/24)
		reason.Score = q.Config.TokenWeight*reason.TokenOverlap + q.Config.CategoryWeight*boolScore(reason.CategoryMatch) + q.Config.TagWeight*float64(len(reason.TagMatches))/float64(max(1, len(q.Tags))) + q.Config.TierWeight*tierScore(r.Tier) + q.Config.RecencyWeight*reason.Recency + q.Config.OutcomeWeight*boolScore(reason.Supported || reason.FailureRelevant)
		if reason.Score > 0 {
			candidates = append(candidates, Retrieved{r, reason})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Reason.Score == candidates[j].Reason.Score {
			return candidates[i].Record.ID < candidates[j].Record.ID
		}
		return candidates[i].Reason.Score > candidates[j].Reason.Score
	})
	out := RetrievalResult{Request: q}
	for _, v := range candidates {
		size := len(v.Record.Title) + len(v.Record.Content)
		if len(out.Records) >= q.Config.MaxRecords || out.ApproxChars+size > q.Config.MaxChars {
			continue
		}
		out.Records = append(out.Records, v)
		out.ApproxChars += size
	}
	out.SnapshotHash, _ = s.SnapshotHash(ctx)
	return out, nil
}
func boolScore(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
func Guidance(r RetrievalResult) string {
	if len(r.Records) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("PRIOR EXPERIENCE — MEMORY GUIDANCE, NOT CURRENT EVIDENCE\nThese records may guide possibilities. They do not establish correctness for this task; current measurement wins.\n")
	for _, x := range r.Records {
		fmt.Fprintf(&b, "- [%s/%s/%s] %s: %s (retrieval score %.3f)\n", x.Record.ID, x.Record.Tier, x.Record.Status, x.Record.Title, x.Record.Content, x.Reason.Score)
	}
	return b.String()
}
