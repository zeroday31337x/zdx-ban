package training

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// W1Example is one instruction/output pair in the shape
// training/slow_android/trainer.py's release contract requires for
// w1-training.jsonl.
type W1Example struct {
	Instruction string `json:"instruction"`
	Output      string `json:"output"`
}

// W1DatasetFromCandidates extracts {instruction, output} pairs suitable for
// a W1 release's w1-training.jsonl from a set of recorded candidates,
// possibly merged from several runs.
//
// Only candidates whose Target is W1Candidate are included: it is the one
// signal this pipeline produces that means an authoritative deterministic
// verifier actually confirmed the selected reasoning outcome (see
// internal/cognitive.CandidatesFromBANTrace), not merely that the search
// engine ranked it highest. This performs no dataset-quality, deduplication
// beyond exact candidate ID, PII, or safety review; the release contract in
// training/slow_android/README.md still requires human review of the result
// before it is placed in a W1 release.
func W1DatasetFromCandidates(candidates []Candidate) []W1Example {
	sorted := append([]Candidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := make([]W1Example, 0, len(sorted))
	seen := make(map[string]bool, len(sorted))
	for _, c := range sorted {
		if c.Target != W1Candidate || seen[c.ID] {
			continue
		}
		instruction := strings.TrimSpace(c.Input)
		output := strings.TrimSpace(c.ModelOutput)
		if instruction == "" || output == "" {
			continue
		}
		seen[c.ID] = true
		out = append(out, W1Example{Instruction: instruction, Output: output})
	}
	return out
}

// WriteW1Dataset atomically writes the extracted examples to
// <dir>/w1-training.jsonl. It returns ("", nil) without touching the
// filesystem when there is nothing W1-eligible yet.
func WriteW1Dataset(dir string, candidates []Candidate) (string, error) {
	for _, c := range candidates {
		if err := c.Validate(); err != nil {
			return "", err
		}
	}
	examples := W1DatasetFromCandidates(candidates)
	if len(examples) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	var buf []byte
	for _, ex := range examples {
		b, err := json.Marshal(ex)
		if err != nil {
			return "", err
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	tmp, err := os.CreateTemp(dir, ".ban-w1-training-*.tmp")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if _, err = tmp.Write(buf); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, "w1-training.jsonl")
	if err = os.Rename(name, final); err != nil {
		return "", err
	}
	ok = true
	return final, nil
}
