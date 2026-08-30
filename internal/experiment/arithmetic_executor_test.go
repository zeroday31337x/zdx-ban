package experiment

import (
	"context"
	"testing"
)

func TestArithmeticExecutorUsesExplicitExpression(t *testing.T) {
	e := ArithmeticExecutor{}
	got, err := e.Execute(context.Background(), "Compute (31 × 14) − 8. Return only the number.", "ignore")
	if err != nil || got != "426" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestArithmeticExecutorRejectsUnsupportedGoal(t *testing.T) {
	if _, err := (ArithmeticExecutor{}).Execute(context.Background(), "Compute 31 × 14 − 8", ""); err == nil {
		t.Fatal("expected unsupported expression")
	}
}
