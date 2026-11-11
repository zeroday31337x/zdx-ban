package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"zdx-ban/internal/simulation"
)

// scenarioCommand evaluates declarative inputs; it never executes actions.
func scenarioCommand(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("scenario requires exactly one JSON file path")
	}
	file, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer file.Close()
	const maxSize = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return err
	}
	if len(data) > maxSize {
		return fmt.Errorf("scenario file exceeds %d bytes", maxSize)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var scenario simulation.Scenario
	if err := dec.Decode(&scenario); err != nil {
		return fmt.Errorf("invalid scenario: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid scenario: trailing JSON content")
	}
	result, err := simulation.Evaluate(scenario)
	if err != nil {
		return err
	}
	return printJSON(result)
}
