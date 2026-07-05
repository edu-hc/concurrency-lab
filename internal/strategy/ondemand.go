package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// OnDemand spawns a new goroutine for each incoming event, with no concurrency limit.
type OnDemand struct{}

func NewOnDemand() *OnDemand {
	return &OnDemand{}
}

func (od *OnDemand) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	var wg sync.WaitGroup

	for ev := range events {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		ev := ev
		go func() {
			defer wg.Done()
			start := time.Now()
			err := workFn(ctx)
			processingTime := time.Since(start)
			endToEndTime := time.Since(ev.CreatedAt)
			col.Record(ev.ID, processingTime, endToEndTime, err)
		}()
	}

	wg.Wait()
	return nil
}
