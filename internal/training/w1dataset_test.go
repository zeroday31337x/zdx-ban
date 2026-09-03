package training

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func w1eligible(id, instruction, output string) Candidate {
	c := candidate()
	c.ID = id
	c.Input = instruction
	c.ModelOutput = output
	c.Target = W1Candidate
	return c
}

func TestW1DatasetFromCandidatesFiltersToW1Candidate(t *testing.T) {
	memoryOnly := candidate()
	memoryOnly.ID = "mo"
	memoryOnly.Target = MemoryOnly
	memoryOnly.Input = "should not appear"
	memoryOnly.ModelOutput = "should not appear"
	eligible := w1eligible("w1", "why did it fail", "because of X")

	out := W1DatasetFromCandidates([]Candidate{memoryOnly, eligible})
	if len(out) != 1 || out[0].Instruction != "why did it fail" || out[0].Output != "because of X" {
		t.Fatalf("expected only the W1Candidate to survive, got %+v", out)
	}
}

func TestW1DatasetFromCandidatesTrimsAndSkipsEmpty(t *testing.T) {
	padded := w1eligible("padded", "  padded instruction  ", "  padded output  ")
	blankOutput := w1eligible("blank-output", "instruction", "   ")
	blankInstruction := w1eligible("blank-instruction", "   ", "output")

	out := W1DatasetFromCandidates([]Candidate{padded, blankOutput, blankInstruction})
	if len(out) != 1 || out[0].Instruction != "padded instruction" || out[0].Output != "padded output" {
		t.Fatalf("expected only the trimmed non-blank pair, got %+v", out)
	}
}

func TestW1DatasetFromCandidatesDeduplicatesByIDAndSortsDeterministically(t *testing.T) {
	first := w1eligible("b", "second alphabetically", "output-b")
	second := w1eligible("a", "first alphabetically", "output-a")
	duplicate := w1eligible("a", "first alphabetically", "output-a")

	out := W1DatasetFromCandidates([]Candidate{first, second, duplicate})
	if len(out) != 2 {
		t.Fatalf("expected duplicate id to collapse, got %d examples", len(out))
	}
	if out[0].Instruction != "first alphabetically" || out[1].Instruction != "second alphabetically" {
		t.Fatalf("expected deterministic id-sorted order, got %+v", out)
	}
}

func TestWriteW1DatasetEmptyIsNoop(t *testing.T) {
	dir := t.TempDir()
	memoryOnly := candidate()
	memoryOnly.Target = MemoryOnly
	path, err := WriteW1Dataset(dir, []Candidate{memoryOnly})
	if err != nil || path != "" {
		t.Fatalf("expected no-op when nothing is W1-eligible, got path=%q err=%v", path, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written, got %v", entries)
	}
}

func TestWriteW1DatasetRejectsInvalidCandidate(t *testing.T) {
	dir := t.TempDir()
	invalid := w1eligible("w1", "goal", "answer")
	invalid.ID = ""
	if _, err := WriteW1Dataset(dir, []Candidate{invalid}); err == nil {
		t.Fatal("expected validation error for invalid candidate")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no file written on validation failure, got %v", entries)
	}
}

func TestWriteW1DatasetWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	eligible := w1eligible("w1", "the goal", "the confirmed answer")
	path, err := WriteW1Dataset(dir, []Candidate{eligible})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "w1-training.jsonl"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var examples []W1Example
	for scanner.Scan() {
		var ex W1Example
		if e := json.Unmarshal(scanner.Bytes(), &ex); e != nil {
			t.Fatal(e)
		}
		examples = append(examples, ex)
	}
	if len(examples) != 1 || examples[0].Instruction != "the goal" || examples[0].Output != "the confirmed answer" {
		t.Fatalf("unexpected decoded examples: %+v", examples)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only the final file, temp file leaked: %v", entries)
	}
}
