package training

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteCandidatesJSONL atomically writes one observational candidate record
// per line to <dir>/<runID>.candidates.jsonl, mirroring the temp-file-then-
// rename pattern internal/trace.WriteAtomic uses for run traces. It returns
// ("", nil) without touching the filesystem when there are no candidates.
func WriteCandidatesJSONL(dir, runID string, candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	var buf []byte
	for _, c := range candidates {
		if err := c.Validate(); err != nil {
			return "", err
		}
		b, err := json.Marshal(c)
		if err != nil {
			return "", err
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	tmp, err := os.CreateTemp(dir, ".ban-candidates-*.tmp")
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
	final := filepath.Join(dir, fmt.Sprintf("%s.candidates.jsonl", runID))
	if err = os.Rename(name, final); err != nil {
		return "", err
	}
	ok = true
	return final, nil
}
