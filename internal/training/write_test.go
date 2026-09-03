package training

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCandidatesJSONLEmptyIsNoop(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteCandidatesJSONL(dir, "run-empty", nil)
	if err != nil || path != "" {
		t.Fatalf("expected no-op for empty candidates, got path=%q err=%v", path, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written, got %v", entries)
	}
}

func TestWriteCandidatesJSONLWritesOneLinePerCandidate(t *testing.T) {
	dir := t.TempDir()
	a := candidate()
	b := candidate()
	b.ID = "c2"
	b.SourceGraphNode = "node-2"
	path, err := WriteCandidatesJSONL(dir, "run-1", []Candidate{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "run-1.candidates.jsonl"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var ids []string
	for scanner.Scan() {
		var c Candidate
		if e := json.Unmarshal(scanner.Bytes(), &c); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, c.ID)
	}
	if len(ids) != 2 || ids[0] != a.ID || ids[1] != b.ID {
		t.Fatalf("unexpected decoded ids: %v", ids)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only the final file, temp file leaked: %v", entries)
	}
}

func TestWriteCandidatesJSONLRejectsInvalidCandidate(t *testing.T) {
	dir := t.TempDir()
	invalid := candidate()
	invalid.ID = ""
	if _, err := WriteCandidatesJSONL(dir, "run-invalid", []Candidate{invalid}); err == nil {
		t.Fatal("expected validation error for invalid candidate")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no file written on validation failure, got %v", entries)
	}
}
