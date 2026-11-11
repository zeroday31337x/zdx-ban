package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScenarioCommandRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	valid := `{"title":"Plan","choices":[{"id":"a","label":"A","branches":[{"label":"ok","probability":1,"score":2}]}]}`
	if err := scenarioCommand([]string{write("valid.json", valid)}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, value string }{
		{"unknown.json", strings.Replace(valid, `"title":`, `"unknown":1,"title":`, 1)},
		{"trailing.json", valid + " {}"},
		{"invalid.json", strings.Replace(valid, `"probability":1`, `"probability":0.8`, 1)},
	} {
		if err := scenarioCommand([]string{write(tt.name, tt.value)}); err == nil {
			t.Fatalf("%s accepted", tt.name)
		}
	}
	if err := scenarioCommand(nil); err == nil {
		t.Fatal("missing path accepted")
	}
}
