package memory

import (
	"strings"
	"zdx-ban/internal/ban"
)

type Working struct {
	Goal                      string
	Constraints, Observations []string
	Active                    []*ban.State
	MaxChars                  int
}

func (w Working) Context() string {
	var b strings.Builder
	b.WriteString("Goal: " + w.Goal + "\n")
	for _, s := range w.Active {
		b.WriteString(s.Title + ": " + s.ReasoningSummary + "\n")
		if w.MaxChars > 0 && b.Len() >= w.MaxChars {
			break
		}
	}
	v := b.String()
	if w.MaxChars > 0 && len(v) > w.MaxChars {
		return v[:w.MaxChars]
	}
	return v
}
