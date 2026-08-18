package training

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/thought"
)

type Target string

const (
	MemoryOnly  Target = "MEMORY_ONLY"
	W2Candidate Target = "W2_CANDIDATE"
	W1Candidate Target = "W1_CANDIDATE"
	Rejected    Target = "REJECTED"
	Unknown     Target = "UNKNOWN"
)

type PromotionState string

const (
	Observed              PromotionState = "OBSERVED"
	Recorded              PromotionState = "RECORDED"
	W2Eligible            PromotionState = "W2_ELIGIBLE"
	W2Validated           PromotionState = "W2_VALIDATED"
	Repeated              PromotionState = "REPEATED"
	W1Eligible            PromotionState = "W1_ELIGIBLE"
	W1Validated           PromotionState = "W1_VALIDATED"
	Promoted              PromotionState = "PROMOTED"
	PromotionRejected     PromotionState = "REJECTED"
	PromotionContradicted PromotionState = "CONTRADICTED"
	Expired               PromotionState = "EXPIRED"
	RolledBack            PromotionState = "ROLLED_BACK"
)

type Gravity struct {
	Value  float64 `json:"value"`
	Source string  `json:"source"`
	WellID string  `json:"well_id,omitempty"`
}

func (g Gravity) Validate() error {
	if g.Value < 0 || g.Value > 1 {
		return errors.New("gravity metadata must be within [0,1]")
	}
	return nil
}

type Provenance struct {
	RunID, GraphNodeID, ExecutionID, MemorySnapshotHash, ModelStateID string
	MeasurementIDs, MemoryIDs                                         []string
}
type Candidate struct {
	SchemaVersion                   int
	ID                              string
	Timestamp                       time.Time
	SourceRun, SourceGraphNode      string
	Input                           string
	ThoughtBefore                   *thought.IR
	ModelOutput                     string
	ThoughtAfter                    *thought.IR
	MemoryAttribution               []string
	VMExecutionID                   string
	MeasurementOutcome              measurement.Outcome
	Confidence, Novelty, Usefulness float64
	Contradictions                  []string
	RepetitionCount                 int
	Gravity                         Gravity
	Target                          Target
	ValidationState                 PromotionState
	Provenance                      Provenance
}

func (c Candidate) Validate() error {
	if c.SchemaVersion != 1 || c.ID == "" || c.SourceRun == "" || c.Timestamp.IsZero() {
		return errors.New("invalid training candidate")
	}
	if err := c.Gravity.Validate(); err != nil {
		return err
	}
	if c.ValidationState == Promoted {
		return errors.New("candidate creation cannot auto-promote")
	}
	return nil
}
func StableID(run, node, input string) string {
	h := sha256.Sum256([]byte(run + "\x00" + node + "\x00" + input))
	return "tc-" + hex.EncodeToString(h[:12])
}

type Policy interface {
	Next(Candidate, PromotionState) (PromotionState, error)
}
type DisabledPolicy struct{}

func (DisabledPolicy) Next(Candidate, PromotionState) (PromotionState, error) {
	return "", errors.New("automatic W1/W2 promotion disabled")
}

type Store interface {
	Append(context.Context, Candidate) error
	List(context.Context) ([]Candidate, error)
}
type MemoryStore struct{ Items []Candidate }

func (s *MemoryStore) Append(_ context.Context, c Candidate) error {
	if e := c.Validate(); e != nil {
		return e
	}
	for _, v := range s.Items {
		if v.ID == c.ID {
			return fmt.Errorf("duplicate training candidate %q", c.ID)
		}
	}
	s.Items = append(s.Items, c)
	return nil
}
func (s *MemoryStore) List(context.Context) ([]Candidate, error) {
	return append([]Candidate(nil), s.Items...), nil
}

type ExportManifest struct {
	SchemaVersion            int                         `json:"schema_version"`
	CreationTime             time.Time                   `json:"creation_time"`
	SourceExperimentIDs      []string                    `json:"source_experiment_ids"`
	CandidateCount           int                         `json:"candidate_count"`
	OutcomeDistribution      map[measurement.Outcome]int `json:"outcome_distribution"`
	TargetWeightDistribution map[Target]int              `json:"target_weight_distribution"`
	CandidatesSHA256         string                      `json:"candidates_sha256"`
	SelectionPolicy          string                      `json:"selection_policy"`
}

func Export(ctx context.Context, w io.Writer, candidates []Candidate) (ExportManifest, error) {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	h := sha256.New()
	mw := io.MultiWriter(w, h)
	runs := map[string]bool{}
	m := ExportManifest{SchemaVersion: 1, CreationTime: time.Unix(0, 0).UTC(), OutcomeDistribution: map[measurement.Outcome]int{}, TargetWeightDistribution: map[Target]int{}, SelectionPolicy: "observational; no automatic positive-label or weight promotion"}
	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return m, ctx.Err()
		default:
		}
		if e := c.Validate(); e != nil {
			return m, e
		}
		b, e := json.Marshal(c)
		if e != nil {
			return m, e
		}
		if _, e = mw.Write(append(b, '\n')); e != nil {
			return m, e
		}
		runs[c.SourceRun] = true
		m.CandidateCount++
		m.OutcomeDistribution[c.MeasurementOutcome]++
		m.TargetWeightDistribution[c.Target]++
	}
	for r := range runs {
		m.SourceExperimentIDs = append(m.SourceExperimentIDs, r)
	}
	sort.Strings(m.SourceExperimentIDs)
	m.CandidatesSHA256 = hex.EncodeToString(h.Sum(nil))
	return m, nil
}
func ExportDirectory(ctx context.Context, dir string, candidates []Candidate) (ExportManifest, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return ExportManifest{}, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "candidates.jsonl"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return ExportManifest{}, e
	}
	bw := bufio.NewWriter(f)
	m, e := Export(ctx, bw, candidates)
	if e == nil {
		e = bw.Flush()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return m, e
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return m, e
	}
	e = os.WriteFile(filepath.Join(dir, "manifest.json"), append(b, '\n'), 0600)
	return m, e
}
