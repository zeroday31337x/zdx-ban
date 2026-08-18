package ban

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func Fingerprint(s *State) string {
	n := nonWord.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Title+" "+s.Hypothesis)), " ")
	h := sha256.Sum256([]byte(strings.Join(strings.Fields(n), " ")))
	return hex.EncodeToString(h[:16])
}
func NewState(id string, p Proposal, depth int) *State {
	now := time.Now().UTC()
	return &State{ID: id, Depth: depth, Title: p.Title, Hypothesis: p.Hypothesis, ReasoningSummary: p.ReasoningSummary, Assumptions: p.Assumptions, Status: Proposed, Kind: ReasoningCandidate, CreatedAt: now, UpdatedAt: now, Metadata: map[string]string{}}
}
