package workload

import (
	"context"
	"time"
)

// Func is a function that simulates work and respects context cancellation.
type Func func(ctx context.Context) error

// CPU returns a Func that simulates CPU-bound work for the given duration.
// It runs arithmetic operations in a loop, checking for cancellation periodically.
func CPU(duration time.Duration) Func {
	return func(ctx context.Context) error {
		start := time.Now()
		var x float64 = 1.0
		for time.Since(start) < duration {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Arithmetic operations to simulate CPU work.
			for i := 0; i < 1000; i++ {
				x = x*1.0000001 + 0.0000001
			}
		}
		return nil
	}
}

// IO returns a Func that simulates I/O-bound work for the given duration.
// It uses time.Sleep semantics via a timer, respecting context cancellation.
func IO(duration time.Duration) Func {
	return func(ctx context.Context) error {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
