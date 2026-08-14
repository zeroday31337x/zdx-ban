package ban

import (
	"context"
	"time"
)

type Verifier interface {
	Name() string
	Verify(context.Context, string, *State) VerificationResult
}
type AcceptVerifier struct{}

func (AcceptVerifier) Name() string { return "accept" }
func (AcceptVerifier) Verify(_ context.Context, _ string, _ *State) VerificationResult {
	return VerificationResult{Verifier: "accept", Passed: true, Details: "no deterministic constraint supplied", At: time.Now().UTC()}
}
