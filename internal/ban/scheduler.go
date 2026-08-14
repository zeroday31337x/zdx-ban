package ban

import (
	"context"
	"sync"
)

func Parallel[T any, R any](ctx context.Context, limit int, in []T, fn func(context.Context, T) (R, error)) ([]R, error) {
	if limit < 1 {
		limit = 1
	}
	out := make([]R, len(in))
	jobs := make(chan int)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for w := 0; w < limit; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r, err := fn(ctx, in[i])
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					continue
				}
				out[i] = r
			}
		}()
	}
	for i := range in {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return nil, err
	default:
		return out, nil
	}
}
