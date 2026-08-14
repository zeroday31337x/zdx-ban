package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/model"
)

func Run(ctx context.Context, p model.Provider, e *ban.Engine, items []Item) ([]Comparison, error) {
	out := make([]Comparison, 0, len(items))
	for _, item := range items {
		base, err := Baseline(ctx, p, item.Prompt, e.Temperature, e.MaxTokens)
		if err != nil {
			return nil, err
		}
		res, t, err := e.Run(ctx, item.Prompt)
		if err != nil {
			return nil, err
		}
		out = append(out, Comparison{item, base, res.Answer, t.Metrics.ModelCalls, t.Metrics.TotalNodes, t.Metrics.PrunedNodes, t.RecoveredFromWrongBranch})
	}
	return out, nil
}
func Load(path string) ([]Item, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v []Item
	err = json.Unmarshal(b, &v)
	return v, err
}
