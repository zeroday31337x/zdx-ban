package experiment

import (
	"context"
	"time"
	"zdx-ban/internal/ban"
)

type BranchVerifier struct {
	Case      Case
	Objective ObjectiveVerifier
}

func (v BranchVerifier) Name() string { return "experiment:" + v.Objective.Type() }
func (v BranchVerifier) Verify(ctx context.Context, _ string, s *ban.State) ban.VerificationResult {
	r := verify(v.Objective, ctx, v.Case, s.Hypothesis)
	return ban.VerificationResult{Verifier: v.Name(), Passed: r.Passed, Details: string(r.Outcome) + ": " + r.Details, At: time.Now().UTC()}
}
