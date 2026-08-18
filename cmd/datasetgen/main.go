package main

import (
	"encoding/json"
	"fmt"
	"os"
	"zdx-ban/internal/experiment"
	"zdx-ban/internal/measurement"
)

func main() {
	if err := writeMemory("datasets/ban-memory-experiment-001-smoke.jsonl", "ban-memory-exp-001-smoke-v2", 4); err != nil {
		panic(err)
	}
	if err := writeMemory("datasets/ban-memory-experiment-001.jsonl", "ban-memory-exp-001-v2", 20); err != nil {
		panic(err)
	}
	if err := write("datasets/ban-experiment-001-smoke.jsonl", "ban-exp-001-smoke-v2", 4); err != nil {
		panic(err)
	}
	if err := write("datasets/ban-experiment-001.jsonl", "ban-exp-001-v2", 20); err != nil {
		panic(err)
	}
}
func write(path, version string, n int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, c := range cases(version, n) {
		if err = enc.Encode(c); err != nil {
			return err
		}
	}
	return f.Sync()
}
func cases(version string, n int) []experiment.Case {
	var out []experiment.Case
	add := func(c experiment.Case) {
		c.DatasetVersion = version
		c.Version = "2"
		class := measurement.BenchmarkVerified
		authority := measurement.DeterministicRuntime
		mt := measurement.Exact
		if c.VerifierType == "numeric" || c.VerifierType == "constraint" {
			class = measurement.FormallyVerified
			authority = measurement.Formal
			mt = measurement.Numeric
		}
		if c.VerifierType == "json" {
			class = measurement.FormallyVerified
			authority = measurement.Formal
			mt = measurement.Structured
		}
		c.Measurement = experiment.MeasurementSpec{VerificationClass: class, Contract: measurement.Contract{ID: "contract-" + c.ID, Claim: "candidate output satisfies benchmark case " + c.ID, MeasurementType: mt, Observable: "model output", Method: "deterministic local " + c.VerifierType + " comparison", ExpectedRelationship: "observed output satisfies hidden benchmark specification", Authority: authority, Independence: measurement.Independent, Repeatability: measurement.Deterministic, Metadata: map[string]any{"scope": "benchmark specification only", "tier": "V0"}}}
		if c.VerifierType == "numeric" {
			zero := 0.0
			c.Measurement.Contract.Tolerance = &measurement.Tolerance{Absolute: &zero}
		}
		out = append(out, c)
	}
	for i := 1; i <= n; i++ {
		a := i*7 + 3
		b := i*3 + 2
		add(experiment.Case{ID: fmt.Sprintf("arithmetic-%03d", i), Category: "arithmetic_constraints", Prompt: fmt.Sprintf("Compute (%d × %d) − %d. Return only the number.", a, b, i+4), Expected: a*b - (i + 4), VerifierType: "numeric", VerifierConfig: map[string]any{"tolerance": 0}, Tags: []string{"arithmetic"}})
	}
	for i := 1; i <= n; i++ {
		winner := fmt.Sprintf("Agent-%02d", i)
		add(experiment.Case{ID: fmt.Sprintf("logic-%03d", i), Category: "logic_deduction", Prompt: fmt.Sprintf("Exactly one statement is true: (1) %s has the key. (2) %s does not have the key. Given that statement 1 is true and statement 2 is false, who has the key? Return only the name.", winner, winner), Expected: winner, VerifierType: "exact", VerifierConfig: map[string]any{"normalize": true}, Tags: []string{"logic"}})
	}
	for i := 1; i <= n; i++ {
		key := fmt.Sprintf("item%d", i)
		val := i * 11
		add(experiment.Case{ID: fmt.Sprintf("structured-%03d", i), Category: "structured_transformation", Prompt: fmt.Sprintf("Return one JSON object with fields name and value. Set name to %q and value to %d. No prose.", key, val), Expected: map[string]any{"name": key, "value": val}, VerifierType: "json", Tags: []string{"json", "transformation"}})
	}
	issues := []struct{ snippet, symptom, answer string }{
		{"var m map[string]int; m[\"x\"] = 1", "panics on assignment", "nil map assignment"},
		{"defer f.Close() before checking os.Open error", "may panic when open fails", "defer before error check"},
		{"for i := 0; i <= len(xs); i++ { _ = xs[i] }", "panics at loop end", "off by one"},
		{"go func(){ total++ }()", "concurrent increments are unsafe", "data race"},
		{"if err != nil { return nil }", "failure is silently reported as success", "error swallowed"},
		{"time.Sleep(time.Duration(ms))", "milliseconds are treated as nanoseconds", "missing duration unit"},
		{"json.Unmarshal(data, value)", "non-pointer destination fails", "non pointer destination"},
		{"scanner.Scan(); use(scanner.Text())", "scan error is never inspected", "scanner error ignored"},
		{"mu.Lock(); work(); return", "mutex remains locked", "missing mutex unlock"},
		{"ctx, cancel := context.WithCancel(parent); _ = ctx", "cancel resources leak", "cancel function unused"},
		{"append(dst, src...); assume dst unchanged", "backing array may be reused", "slice aliasing"},
		{"for _, v := range xs { ptrs = append(ptrs, &v) }", "pointers may target loop variable", "range variable address"},
		{"http.Get(url); ignore response body", "connections cannot be reused", "response body unclosed"},
		{"close(ch); close(ch)", "second close panics", "double channel close"},
		{"select { case ch <- x: }", "send can wait forever", "blocking channel send"},
		{"wg.Add(1) inside the new goroutine", "Wait may return too early", "late waitgroup add"},
		{"strings.Trim(s, \".json\")", "removes a character set not suffix", "trim versus trimsuffix"},
		{"errors.New(fmt.Sprintf(\"bad %s\", x))", "formatting is unnecessarily nested", "use fmt errorf"},
		{"sort.Slice(xs, func(i,j int) bool { return i < j })", "comparison ignores values", "invalid sort comparator"},
		{"recover() outside a deferred function", "panic is not intercepted", "ineffective recover"},
	}
	for i := 1; i <= n; i++ {
		issue := issues[i-1]
		prompt := fmt.Sprintf("Inspect this Go fragment: `%s`. It %s. Name the defect using exactly the canonical phrase requested.", issue.snippet, issue.symptom)
		add(experiment.Case{ID: fmt.Sprintf("coding-%03d", i), Category: "coding_debugging", Prompt: prompt, Expected: issue.answer, VerifierType: "exact", VerifierConfig: map[string]any{"normalize": true}, Tags: []string{"go", "debugging"}})
	}
	for i := 1; i <= n; i++ {
		start := 20 + i
		lost := i + 2
		found := i % 3
		expected := start - lost + found
		add(experiment.Case{ID: fmt.Sprintf("recovery-%03d", i), Category: "forced_recovery", Prompt: fmt.Sprintf("A box label says it contains %d bolts. The label is an outdated inventory count, not a current measurement. A signed audit records that %d bolts were removed and %d returned. How many bolts are actually present? Ignore the tempting label-as-current interpretation. Return only the number.", start, lost, found), Expected: expected, VerifierType: "numeric", VerifierConfig: map[string]any{"tolerance": 0}, ForcedRecovery: true, TrapDescription: "The surface label invites treating the starting count as current without applying the signed audit.", Tags: []string{"misleading_surface", "recovery"}})
	}
	return out
}
