// Package thought defines the canonical semantic representation used between BAN,
// neural inference, compilers, and execution adapters.
package thought

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const Version = "1"

type Entity struct{ ID, Name, Kind, Description string }
type Relation struct {
	Subject, Predicate, Object string
	Confidence                 *float64 `json:"confidence,omitempty"`
}
type Assumption struct {
	ID, Statement string
	Confidence    *float64 `json:"confidence,omitempty"`
}
type Constraint struct {
	ID, Kind, Description string
	Required              bool
}
type EvidenceReference struct {
	ID, Source, Claim string
	URI               string   `json:"uri,omitempty"`
	Confidence        *float64 `json:"confidence,omitempty"`
}
type Uncertainty struct {
	Subject, Description string
	Probability          *float64 `json:"probability,omitempty"`
}
type TemporalRelationship struct{ Before, After, Relation string }
type SpatialRelationship struct {
	Subject, Object, Relation string
	CoordinateSystem          string `json:"coordinate_system,omitempty"`
}
type Action struct {
	ID, Actor, Operation, Target string
	Parameters                   json.RawMessage `json:"parameters,omitempty"`
}
type PredictedEffect struct {
	ActionID, Subject, Effect string
	Confidence                *float64 `json:"confidence,omitempty"`
}
type Observation struct{ ID, Subject, Value, Source string }
type Question struct {
	ID, Text string
	Blocking bool
}

type IR struct {
	Version             string                 `json:"version"`
	ID                  string                 `json:"id"`
	Goal                string                 `json:"goal"`
	Entities            []Entity               `json:"entities,omitempty"`
	Relations           []Relation             `json:"relations,omitempty"`
	Assumptions         []Assumption           `json:"assumptions,omitempty"`
	Constraints         []Constraint           `json:"constraints,omitempty"`
	Evidence            []EvidenceReference    `json:"evidence_references,omitempty"`
	Uncertainty         []Uncertainty          `json:"uncertainty,omitempty"`
	Temporal            []TemporalRelationship `json:"temporal_relationships,omitempty"`
	Spatial             []SpatialRelationship  `json:"spatial_relationships,omitempty"`
	CandidateActions    []Action               `json:"candidate_actions,omitempty"`
	PredictedEffects    []PredictedEffect      `json:"predicted_effects,omitempty"`
	Observations        []Observation          `json:"observations,omitempty"`
	UnresolvedQuestions []Question             `json:"unresolved_questions,omitempty"`
}

func (v IR) Validate() error {
	if v.Version != Version {
		return fmt.Errorf("unsupported ThoughtIR version %q", v.Version)
	}
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.Goal) == "" {
		return errors.New("ThoughtIR id and goal are required")
	}
	seen := map[string]bool{}
	for _, a := range v.CandidateActions {
		if a.ID == "" || a.Operation == "" {
			return errors.New("ThoughtIR action id and operation are required")
		}
		if seen[a.ID] {
			return errors.New("duplicate ThoughtIR action id")
		}
		seen[a.ID] = true
		if len(a.Parameters) > 0 && !json.Valid(a.Parameters) {
			return errors.New("invalid ThoughtIR action parameters")
		}
	}
	return nil
}
func (v IR) MarshalDeterministic() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func Parse(data []byte) (IR, error) {
	var v IR
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, err
	}
	if d.Decode(&struct{}{}) == nil {
		return v, errors.New("multiple JSON values")
	}
	return v, v.Validate()
}
func StableID(goal string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(goal)))
	return "tir-" + hex.EncodeToString(h[:8])
}

type Mutation struct {
	Version          string        `json:"version"`
	ThoughtID        string        `json:"thought_id"`
	AddActions       []Action      `json:"add_actions,omitempty"`
	AddObservations  []Observation `json:"add_observations,omitempty"`
	ResolveQuestions []string      `json:"resolve_questions,omitempty"`
}
