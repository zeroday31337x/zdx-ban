package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteAtomic(dir, runID string, value any) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".ban-*.tmp")
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
	if _, err = tmp.Write(b); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, fmt.Sprintf("%s.json", runID))
	if err = os.Rename(name, final); err != nil {
		return "", err
	}
	ok = true
	return final, nil
}
