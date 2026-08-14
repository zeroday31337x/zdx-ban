package benchmark

type Item struct {
	ID                 string            `json:"id"`
	Category           string            `json:"category"`
	Prompt             string            `json:"prompt"`
	ExpectedConditions []string          `json:"expected_conditions"`
	VerificationMethod string            `json:"verification_method"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}
type Comparison struct {
	Item                               Item           `json:"item"`
	Baseline                           BaselineResult `json:"baseline"`
	BANAnswer                          string         `json:"ban_answer"`
	BANModelCalls, BANNodes, BANPruned int
	Recovered                          bool `json:"recovered"`
}
