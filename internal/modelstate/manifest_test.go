package modelstate

import "testing"

func ptr(s string) *string { return &s }
func TestManifestConfigurationsAndW0Protection(t *testing.T) {
	m := DeclaredW0("qwen2.5:1.5b", "bin/ban")
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	m.Experience = &Identity{Class: W1, Name: ptr("experience"), Hash: ptr("abc")}
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	m.Adaptive = &Identity{Class: W2, Name: ptr("W2-2026-08-16"), Hash: ptr("def")}
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	m.Foundation.Immutable = false
	if e := m.Validate(); e == nil {
		t.Fatal("mutable W0 accepted")
	}
}
func TestConditionUnavailableDoesNotFallback(t *testing.T) {
	a := DeclaredW0("qwen2.5:1.5b", "bin/ban").Availability(BANFull, true, true, true)
	if a.Available || len(a.Missing) != 2 {
		t.Fatalf("unexpected availability: %+v", a)
	}
}
