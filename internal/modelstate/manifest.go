package modelstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type WeightClass string

const (
	W0 WeightClass = "W0"
	W1 WeightClass = "W1"
	W2 WeightClass = "W2"
)

type Identity struct {
	Class                                                                                               WeightClass `json:"class"`
	Name                                                                                                *string     `json:"name"`
	Version                                                                                             *string     `json:"version,omitempty"`
	Hash                                                                                                *string     `json:"hash"`
	Immutable                                                                                           bool        `json:"immutable"`
	ArtifactHash, TokenizerConfigHash, Quantization, Architecture, ParameterCount, ContextConfiguration string      `json:",omitempty"`
	IdentityConfidence, IdentitySource                                                                  string      `json:"identity_confidence,omitempty"`
}
type Manifest struct {
	SchemaVersion     int       `json:"schema_version"`
	Foundation        Identity  `json:"foundation"`
	Experience        *Identity `json:"experience"`
	Adaptive          *Identity `json:"adaptive"`
	DeploymentPackage string    `json:"deployment_package,omitempty"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported model-state schema %d", m.SchemaVersion)
	}
	if m.Foundation.Class != W0 || m.Foundation.Name == nil || strings.TrimSpace(*m.Foundation.Name) == "" || !m.Foundation.Immutable {
		return errors.New("W0 must be named and immutable")
	}
	if m.Experience != nil && m.Experience.Class != W1 {
		return errors.New("experience must be W1")
	}
	if m.Adaptive != nil && m.Adaptive.Class != W2 {
		return errors.New("adaptive must be W2")
	}
	return nil
}
func (m Manifest) MarshalDeterministic() ([]byte, error) {
	if e := m.Validate(); e != nil {
		return nil, e
	}
	return json.Marshal(m)
}
func (m Manifest) ID() (string, error) {
	b, e := m.MarshalDeterministic()
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return "modelstate-" + hex.EncodeToString(h[:8]), nil
}
func DeclaredW0(name, deployment string) Manifest {
	return Manifest{SchemaVersion: 1, Foundation: Identity{Class: W0, Name: &name, Immutable: true, IdentityConfidence: "DECLARED", IdentitySource: "provider deployment metadata; artifact unavailable"}, DeploymentPackage: deployment}
}

type Condition string

const (
	QwenBase      Condition = "QWEN_BASE"
	BANCore       Condition = "BAN_CORE"
	BANExperience Condition = "BAN_EXPERIENCE"
	BANAdaptive   Condition = "BAN_ADAPTIVE"
	BANFull       Condition = "BAN_FULL"
)

type Availability struct {
	Condition Condition
	Available bool
	Missing   []string
}

func (m Manifest) Availability(c Condition, thought, vm, memory bool) Availability {
	a := Availability{Condition: c, Available: true}
	need := func(ok bool, s string) {
		if !ok {
			a.Available = false
			a.Missing = append(a.Missing, s)
		}
	}
	switch c {
	case QwenBase:
	case BANCore:
		need(memory, "explicit memory/search")
	case BANExperience:
		need(m.Experience != nil, "W1")
	case BANAdaptive:
		need(m.Experience != nil, "W1")
		need(m.Adaptive != nil, "W2")
	case BANFull:
		need(m.Experience != nil, "W1")
		need(m.Adaptive != nil, "W2")
		need(memory, "memory")
		need(thought, "ThoughtCompiler")
		need(vm, "ZDXVM")
	default:
		need(false, "unknown condition")
	}
	return a
}
