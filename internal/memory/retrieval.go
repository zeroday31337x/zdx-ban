package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

type structuralFeatures struct {
	math, code, imperative, url, path float64
}

func fingerprint(value string) structuralFeatures {
	text := strings.TrimSpace(value)
	if text == "" {
		return structuralFeatures{}
	}
	mathChars := 0
	codeChars := 0
	for _, r := range text {
		if strings.ContainsRune("+-*/=^%×÷−", r) || unicode.IsDigit(r) {
			mathChars++
		}
		if strings.ContainsRune("{}();=><[]!&|@#$`\\", r) {
			codeChars++
		}
	}
	first := strings.ToLower(strings.Fields(text)[0])
	first = strings.Trim(first, ".,!?;:")
	imperative := map[string]bool{"compute": true, "calculate": true, "build": true, "create": true, "write": true, "run": true, "execute": true, "check": true, "test": true, "fix": true, "explain": true, "analyze": true}[first]
	return structuralFeatures{
		math:       minFloat(1, float64(mathChars)/float64(max(1, len([]rune(text)))*5)),
		code:       minFloat(1, float64(codeChars)/float64(max(1, len([]rune(text)))*10)),
		imperative: boolScore(imperative),
		url:        boolScore(strings.Contains(strings.ToLower(text), "http://") || strings.Contains(strings.ToLower(text), "https://")),
		path:       boolScore(strings.Contains(text, "/") || strings.Contains(text, `\\`)),
	}
}

func structuralSimilarity(a, b structuralFeatures) float64 {
	var sum, active float64
	for _, pair := range [][2]float64{{a.math, b.math}, {a.code, b.code}, {a.imperative, b.imperative}, {a.url, b.url}, {a.path, b.path}} {
		if pair[0] <= 0 {
			continue
		}
		active++
		sum += 1 - minFloat(1, absFloat(pair[0]-pair[1]))
	}
	if active == 0 {
		return 0
	}
	return sum / active
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

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

// ContextTags derives a bounded, answer-free routing signature from a task.
// Tags are intentionally structural/semantic; numeric literals are excluded
// so the tag layer cannot leak the benchmark answer into model context.
func ContextTags(query, category string, tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 5)
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] || len(out) >= 5 {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, tag := range tags {
		add(tag)
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(category), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(part) > 2 {
			add(part)
		}
	}
	text := strings.TrimSpace(query)
	mathSignal := false
	for _, r := range text {
		if unicode.IsDigit(r) || strings.ContainsRune("+-*/=^%×÷−", r) {
			mathSignal = true
			break
		}
	}
	if mathSignal {
		add("arithmetic")
		add("numeric")
	}
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "only": true, "return": true, "this": true, "that": true, "from": true, "into": true}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len(word) > 3 && !stop[word] {
			add(word)
		}
		if len(out) >= 5 {
			break
		}
	}
	return out
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
	if len(q.ContextTags) == 0 {
		q.ContextTags = ContextTags(q.Query, q.Category, q.Tags)
	}
	records, e := s.List(ctx)
	if e != nil {
		return RetrievalResult{}, e
	}
	queryTokens := tokens(strings.Join([]string{q.Query, q.Category, q.StrategyType, strings.Join(q.Tags, " "), strings.Join(q.ContextTags, " ")}, " "))
	queryStructure := fingerprint(q.Query)
	gravityRecords := make([]Record, 0, len(records))
	for _, record := range records {
		if retrievalEligible(record, q) {
			gravityRecords = append(gravityRecords, record)
		}
	}
	wells := BuildGravityWells(gravityRecords, q.Now, q.Config.Gravity)
	wellsByCategory := map[string]GravityWell{}
	for _, well := range wells {
		wellTokens := map[string]bool{}
		for _, token := range well.Keywords {
			wellTokens[token] = true
		}
		if strings.EqualFold(well.Category, q.Category) || overlap(queryTokens, wellTokens) > 0 {
			wellByCategoryKey := strings.ToLower(strings.TrimSpace(well.Category))
			wellsByCategory[wellByCategoryKey] = well
		}
	}
	var candidates []Retrieved
	for _, r := range records {
		if !retrievalEligible(r, q) {
			continue
		}
		reason := RetrievalReason{Tier: r.Tier}
		reason.TokenOverlap = overlap(queryTokens, tokens(r.Title+" "+r.Content+" "+strings.Join(r.Tags, " ")))
		reason.StructuralMatch = structuralSimilarity(queryStructure, fingerprint(r.Title+" "+r.Content+" "+strings.Join(r.Tags, " ")))
		for _, contextTag := range q.ContextTags {
			if strings.EqualFold(contextTag, r.Category) || strings.EqualFold(contextTag, r.StrategyType) || tokens(r.Title + " " + r.Content + " " + strings.Join(r.Tags, " "))[strings.ToLower(contextTag)] {
				reason.TagMatches = append(reason.TagMatches, contextTag)
			}
		}
		// De-duplicate tag matches before scoring/reporting.
		reason.TagMatches = uniqueStrings(reason.TagMatches)
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
		if well, ok := wellsByCategory[strings.ToLower(strings.TrimSpace(r.Category))]; ok {
			relevance := .6*boolScore(reason.CategoryMatch) + .4*reason.TokenOverlap
			reason.GravityWellID = well.ID
			reason.GravityStrength = well.Strength
			reason.GravityRepulsion = well.Repulsion
			reason.GravityEvidence = well.SupportingEvidence
			reason.InformationGravity = relevance * (well.Strength - well.Repulsion)
		}
		reason.Score = q.Config.TokenWeight*reason.TokenOverlap + .05*reason.StructuralMatch + q.Config.CategoryWeight*boolScore(reason.CategoryMatch) + q.Config.TagWeight*float64(len(reason.TagMatches))/float64(max(1, len(q.Tags))) + q.Config.TierWeight*tierScore(r.Tier) + q.Config.RecencyWeight*reason.Recency + q.Config.OutcomeWeight*boolScore(reason.Supported || reason.FailureRelevant) + q.Config.Gravity.Weight*reason.InformationGravity
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
	out := RetrievalResult{Request: q, ContextTags: append([]string(nil), q.ContextTags...)}
	for _, well := range wellsByCategory {
		out.GravityWells = append(out.GravityWells, well)
	}
	sort.Slice(out.GravityWells, func(i, j int) bool { return out.GravityWells[i].ID < out.GravityWells[j].ID })
	for _, v := range candidates {
		// Episodes persist full audit evidence, but only their compact,
		// answer-free route summary enters working memory.
		size := len(v.Record.Title) + len(guidanceContent(v.Record))
		if len(out.Records) >= q.Config.MaxRecords || out.ApproxChars+size > q.Config.MaxChars {
			continue
		}
		out.Records = append(out.Records, v)
		out.ApproxChars += size
	}
	out.SnapshotHash, _ = s.SnapshotHash(ctx)
	return out, nil
}

func retrievalEligible(record Record, request RetrievalRequest) bool {
	if record.Status == Superseded || record.Status == Stale || !included(record.Tier, request.Config.IncludeTiers) {
		return false
	}
	if request.Config.MaxAge > 0 && request.Now.Sub(record.UpdatedAt) > request.Config.MaxAge {
		return false
	}
	if request.ExcludeCaseID != "" && record.Provenance.CaseID == request.ExcludeCaseID {
		return false
	}
	return request.ExcludeExactAnswer == "" || !recordExposesAnswer(record, request.ExcludeExactAnswer)
}

func recordExposesAnswer(record Record, answer string) bool {
	if record.Kind == EpisodeKind {
		var episode Episode
		if json.Unmarshal([]byte(record.Content), &episode) != nil {
			return true
		}
		// Only compact route summaries are exposed from episodes. Audit-only
		// timestamps, costs, IDs, observations, and expected values must not
		// trigger accidental substring exclusions.
		return semanticContains(strings.Join(episode.UsefulBranches, " "), answer)
	}
	return semanticContains(record.Content, answer)
}

func semanticContains(value, answer string) bool {
	normalize := func(input string) string {
		return strings.Join(strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '-'
		}), " ")
	}
	haystack, needle := normalize(value), normalize(answer)
	if needle == "" {
		return false
	}
	return strings.Contains(" "+haystack+" ", " "+needle+" ")
}
func boolScore(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
func Guidance(r RetrievalResult) string {
	if len(r.Records) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("PRIOR EXPERIENCE — MEMORY GUIDANCE, NOT CURRENT EVIDENCE\nThese records may guide possibilities. They do not establish correctness for this task; current measurement wins.\n")
	if len(r.ContextTags) > 0 {
		fmt.Fprintf(&b, "CONTEXT TAGS: %s\nUse these tags to connect the current task to prior structured memory; do not treat tags as answers.\n", strings.Join(r.ContextTags, ", "))
	}
	for _, well := range r.GravityWells {
		kind := "verified"
		if well.FoundationAnchor {
			kind = "foundation anchor"
		}
		fmt.Fprintf(&b, "GRAVITY WELL %s category=%s kind=%s attraction=%.3f repulsion=%.3f verified_evidence=%d. Use the anchor as a starting structure; organic measured evidence outranks it. Do not copy answers or bypass current verification.\n", well.ID, well.Category, kind, well.Strength, well.Repulsion, well.SupportingEvidence)
	}
	for _, x := range r.Records {
		content := guidanceContent(x.Record)
		fmt.Fprintf(&b, "- [%s/%s/%s] %s: %s (retrieval score %.3f, information gravity %.3f)\n", x.Record.ID, x.Record.Tier, x.Record.Status, x.Record.Title, content, x.Reason.Score, x.Reason.InformationGravity)
	}
	return b.String()
}

func guidanceContent(record Record) string {
	if record.Kind != EpisodeKind {
		return record.Content
	}
	var episode Episode
	if json.Unmarshal([]byte(record.Content), &episode) != nil {
		return "measured prior episode"
	}
	parts := []string{"measured outcome=" + episode.Outcome}
	if len(episode.UsefulBranches) > 0 {
		parts = append(parts, "verified method="+strings.Join(episode.UsefulBranches, "; "))
	}
	if episode.Recovered {
		parts = append(parts, "recovered after rejecting a failed route")
	}
	return strings.Join(parts, ", ")
}
