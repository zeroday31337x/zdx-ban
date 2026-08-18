package inference

import "context"

type Difference struct {
	BackendA, BackendB                            string
	OutputEqual, StructuredEqual, StopReasonEqual bool
	OutputA, OutputB                              string
	StructuredA, StructuredB                      []byte
	StopReasonA, StopReasonB                      string
	UsageA, UsageB                                Usage
	ErrorA, ErrorB                                string
	TokenizationObservable                        bool
}

func Compare(ctx context.Context, aName string, a Engine, bName string, b Engine, r Request) Difference {
	d := Difference{BackendA: aName, BackendB: bName}
	ar, ae := a.Generate(ctx, r)
	br, be := b.Generate(ctx, r)
	d.OutputA, d.OutputB = ar.Output, br.Output
	d.OutputEqual = ar.Output == br.Output
	d.StructuredA, d.StructuredB = ar.Structured, br.Structured
	d.StructuredEqual = string(ar.Structured) == string(br.Structured)
	d.StopReasonA, d.StopReasonB = ar.StopReason, br.StopReason
	d.StopReasonEqual = ar.StopReason == br.StopReason
	d.UsageA, d.UsageB = ar.Usage, br.Usage
	if ae != nil {
		d.ErrorA = ae.Error()
	}
	if be != nil {
		d.ErrorB = be.Error()
	}
	d.TokenizationObservable = ar.Usage.PromptTokens > 0 || br.Usage.PromptTokens > 0
	return d
}
