package inference

import "context"

const MaxConcurrent = 2

var slots = make(chan struct{}, MaxConcurrent)

// Acquire waits for one of the bounded CPU inference slots and returns its release function.
func Acquire() func() {
	release, _ := AcquireContext(context.Background())
	return release
}

// AcquireContext waits for an inference slot but allows callers to bound queue time.
func AcquireContext(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
