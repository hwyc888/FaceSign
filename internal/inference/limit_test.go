package inference

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireBoundsTenConcurrentClientsToTwoInferenceTasks(t *testing.T) {
	const clients = 10
	var active atomic.Int32
	var maximum atomic.Int32
	var wait sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < clients; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			release := Acquire()
			current := active.Add(1)
			for previous := maximum.Load(); current > previous; previous = maximum.Load() {
				if maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			release()
		}()
	}

	close(start)
	wait.Wait()
	if got := maximum.Load(); got != MaxConcurrent {
		t.Fatalf("maximum concurrent inference=%d, want %d", got, MaxConcurrent)
	}
}
