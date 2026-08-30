package experiment

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ArithmeticExecutor is deliberately narrow: it evaluates the explicit
// parenthesized three-number arithmetic form used by the benchmark. It never
// reads Case.Expected, memory answers, or model output.
type ArithmeticExecutor struct{}

var arithmeticPrompt = regexp.MustCompile(`(?i)^\s*compute\s*\(\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s*([+\-*/×÷−])\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s*\)\s*([+\-*/×÷−])\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))`)

func (ArithmeticExecutor) Execute(_ context.Context, goal, _ string) (string, error) {
	m := arithmeticPrompt.FindStringSubmatch(strings.TrimSpace(goal))
	if len(m) != 6 {
		return "", fmt.Errorf("unsupported arithmetic expression")
	}
	a, _ := strconv.ParseFloat(m[1], 64)
	b, _ := strconv.ParseFloat(m[3], 64)
	c, _ := strconv.ParseFloat(m[5], 64)
	inner, err := arithmeticOp(a, b, m[2])
	if err != nil {
		return "", err
	}
	out, err := arithmeticOp(inner, c, m[4])
	if err != nil {
		return "", err
	}
	return formatNumber(out), nil
}

func arithmeticOp(a, b float64, op string) (float64, error) {
	switch op {
	case "+":
		return a + b, nil
	case "-", "−":
		return a - b, nil
	case "*", "×":
		return a * b, nil
	case "/", "÷":
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	default:
		return 0, fmt.Errorf("unsupported arithmetic operator %q", op)
	}
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
