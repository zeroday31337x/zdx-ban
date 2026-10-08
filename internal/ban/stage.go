package ban

import (
	"context"
	"time"
)

// runStage reports stage boundaries and regular heartbeats. Slow CPU model
// inference remains cancellable by the caller's context.
func (e *Engine) runStage(ctx context.Context, label string, work func() error) error {
	if e.Logf == nil {
		return work()
	}
	start := time.Now()
	e.Logf("[BAN] %s started", label)
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				e.Logf("[BAN] %s - still processing (%s elapsed)", label, time.Since(start).Round(time.Second))
			}
		}
	}()
	err := work()
	if err != nil {
		e.Logf("[BAN] %s failed after %s: %v", label, time.Since(start).Round(time.Second), err)
	} else {
		e.Logf("[BAN] %s completed in %s", label, time.Since(start).Round(time.Second))
	}
	return err
}
