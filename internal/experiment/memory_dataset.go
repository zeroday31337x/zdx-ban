package experiment

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
)

type SeedMemory struct {
	ID, Title, Content, Category, StrategyType, CorrelationGroup string
	Tier                                                         memory.Tier
	Kind                                                         memory.Kind
	Status                                                       memory.Status
	Tags                                                         []string
	SourceClass                                                  memory.SourceClass
}
type MemoryCase struct {
	DatasetVersion, ID, Version, Category, Relationship string
	Origin, Difficulty, NoveltyClass, Behavior          string
	ExpectedObservables, AuthorityInformation           []string
	Exposure                                            []SeedMemory
	Evaluation                                          Case
	Misleading                                          []SeedMemory
	Metadata                                            map[string]any `json:"metadata,omitempty"`
}
type MemoryDataset struct {
	Version, Path, SHA256 string
	Cases                 []MemoryCase
}

// FoundationAnchors provide low-level, answer-free structure for a cold model.
// They are deliberately weaker than one independently measured organic well.
func FoundationAnchors(categories []string) []SeedMemory {
	seen := map[string]bool{}
	var out []SeedMemory
	for _, category := range categories {
		if seen[category] {
			continue
		}
		seen[category] = true
		var title, content string
		switch category {
		case "arithmetic_constraints":
			title, content = "Foundation arithmetic method", "Preserve explicit operation structure, compute grouped operations before outer operations, then submit a concise candidate for independent verification."
		case "logic_deduction":
			title, content = "Foundation logic method", "Translate each statement into explicit constraints, test consistent assignments, and submit a candidate for independent verification."
		case "structured_transformation":
			title, content = "Foundation structure method", "Preserve the requested schema and transform only the specified fields, then validate the complete structure independently."
		case "coding_debugging":
			title, content = "Foundation debugging method", "Reproduce the failure, isolate the smallest causal change, and validate the proposed fix with an independent check."
		case "forced_recovery":
			title, content = "Foundation recovery method", "Treat the first attractive route as provisional, generate an independent alternative, and verify the final candidate before acceptance."
		default:
			continue
		}
		out = append(out, SeedMemory{ID: "foundation-" + category, Title: title, Content: content, Category: category, StrategyType: category + "-foundation", Tier: memory.Foundation, Kind: memory.StructuralRule, Status: memory.Active, SourceClass: memory.MemoryGuidance})
	}
	return out
}

func LoadFoundationMemory(path string) ([]SeedMemory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []SeedMemory
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		var seed SeedMemory
		if err := json.Unmarshal(scan.Bytes(), &seed); err != nil {
			return nil, err
		}
		if seed.ID == "" || seed.Category == "" || seed.Content == "" || seed.Tier != memory.Foundation || seed.Kind != memory.StructuralRule {
			return nil, fmt.Errorf("invalid foundation memory record %q", seed.ID)
		}
		out = append(out, seed)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func LoadMemoryDataset(path string, registry *Registry) (MemoryDataset, error) {
	f, e := os.Open(path)
	if e != nil {
		return MemoryDataset{}, e
	}
	defer f.Close()
	h := sha256.New()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4<<20)
	d := MemoryDataset{Path: path}
	ids := map[string]bool{}
	prompts := map[string]string{}
	line := 0
	for scan.Scan() {
		line++
		raw := append([]byte(nil), scan.Bytes()...)
		h.Write(raw)
		h.Write([]byte{'\n'})
		var c MemoryCase
		if e = json.Unmarshal(raw, &c); e != nil {
			return d, fmt.Errorf("line %d: %w", line, e)
		}
		if c.ID == "" || c.Version == "" || c.DatasetVersion == "" || c.Relationship == "" {
			return d, fmt.Errorf("line %d incomplete memory case", line)
		}
		if strings.HasSuffix(c.DatasetVersion, "v2") {
			allowed := map[string]bool{"USEFUL_MEMORY_LEVERAGE": true, "IRRELEVANT_MEMORY_REJECTION": true, "STALE_MEMORY_OVERRIDE": true, "MISLEADING_MEMORY_RECOVERY": true, "UNKNOWN_NOVEL_PRESERVATION": true, "REPEATED_PROBLEM_EFFICIENCY": true, "FAILURE_MEMORY_LEVERAGE": true, "CONFLICTING_HISTORICAL_OBSERVATIONS": true}
			if !allowed[c.Behavior] || c.Origin == "" || c.Difficulty == "" || c.NoveltyClass == "" || len(c.ExpectedObservables) == 0 || len(c.AuthorityInformation) == 0 {
				return d, fmt.Errorf("line %d incomplete Pass 5 metadata", line)
			}
		}
		if ids[c.ID] {
			return d, fmt.Errorf("duplicate memory case id %q", c.ID)
		}
		ids[c.ID] = true
		if e = ValidateCase(c.Evaluation, registry); e != nil {
			return d, e
		}
		norm := normalizedLeak(c.Evaluation.Prompt)
		if prior, ok := prompts[norm]; ok {
			return d, fmt.Errorf("duplicate evaluation prompts %s and %s", prior, c.ID)
		}
		prompts[norm] = c.ID
		expected := normalizedLeak(fmt.Sprint(c.Evaluation.Expected))
		for _, seed := range append(append([]SeedMemory{}, c.Exposure...), c.Misleading...) {
			content := normalizedLeak(seed.Content)
			if content == norm || strings.Contains(content, norm) || strings.Contains(content, expected) && expected != "" {
				return d, fmt.Errorf("memory leakage in case %s seed %s", c.ID, seed.ID)
			}
			if seed.ID == c.Evaluation.ID {
				return d, fmt.Errorf("evaluation ID reused as memory ID")
			}
		}
		if d.Version == "" {
			d.Version = c.DatasetVersion
		} else if d.Version != c.DatasetVersion {
			return d, fmt.Errorf("mixed memory dataset versions")
		}
		d.Cases = append(d.Cases, c)
	}
	if e = scan.Err(); e != nil {
		return d, e
	}
	d.SHA256 = hex.EncodeToString(h.Sum(nil))
	for i := range d.Cases {
		d.Cases[i].Evaluation.Measurement.Contract.Provenance.DatasetVersion = d.Version
		d.Cases[i].Evaluation.Measurement.Contract.Provenance.DatasetHash = d.SHA256
	}
	if len(d.Cases) == 0 {
		return d, fmt.Errorf("empty memory dataset")
	}
	return d, nil
}
func normalizedLeak(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func SeedStore(ctx context.Context, seeds []SeedMemory, readOnly bool) (*memory.MemoryStore, error) {
	s := memory.NewMemoryStore()
	for _, x := range seeds {
		p := memory.Provenance{Source: "memory experiment seed", SourceClass: x.SourceClass, CorrelationGroup: x.CorrelationGroup, Independence: measurement.ModelDerived, CreatedAt: time.Unix(0, 0).UTC()}
		if x.SourceClass == memory.MemoryGuidance {
			p.Independence = measurement.PartiallyIndependent
		}
		r := memory.NewRecord(x.ID, x.Tier, x.Kind, x.Title, x.Content, p)
		r.Category = x.Category
		r.StrategyType = x.StrategyType
		r.CorrelationGroup = x.CorrelationGroup
		r.Status = x.Status
		r.Tags = x.Tags
		r.Metadata = map[string]any{"readOnly": readOnly}
		if x.Tier == memory.Foundation && x.SourceClass == memory.MemoryGuidance {
			r.Metadata["foundationAnchor"] = true
		}
		if e := s.Append(ctx, r); e != nil {
			return nil, e
		}
	}
	return s, nil
}
