package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	p, err := WriteAtomic(dir, "run", map[string]any{"trace_schema_version": "0.1", "nodes": []string{"a"}, "recovered": true})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "run.json" {
		t.Fatal(p)
	}
	if _, err = os.Stat(p); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) > 0 {
		t.Fatal("temporary trace remains")
	}
}
