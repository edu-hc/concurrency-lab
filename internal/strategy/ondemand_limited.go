package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// OnDemandLimited spawns a new goroutine per incoming event, bounded by a
// semaphore that allows at most maxGoroutines concurrent executions.
type OnDemandLimited struct {
	maxGoroutines int
}

func NewOnDemandLimited(maxGoroutines int) *OnDemandLimited {
	return &OnDemandLimited{maxGoroutines: maxGoroutines}
}

func (od *OnDemandLimited) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	sem := make(chan struct{}, od.maxGoroutines)
	var wg sync.WaitGroup

loop:
	for ev := range events {
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}

		wg.Add(1)
		ev := ev
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			start := time.Now()
			err := workFn(ctx)
			processingTime := time.Since(start)
			endToEndTime := time.Since(ev.CreatedAt)
			col.Record(ev.ID, processingTime, endToEndTime, err)
		}()
	}

	wg.Wait()
	return ctx.Err()
}