package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// Batching accumulates events into fixed-size batches and processes each
// batch's events concurrently, one goroutine per event.
type Batching struct {
	batchSize int
}

func NewBatching(batchSize int) *Batching {
	return &Batching{batchSize: batchSize}
}

func (b *Batching) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	batch := make([]event.Event, 0, b.batchSize)

	for {
		select {
		case <-ctx.Done():
			b.processBatch(ctx, batch, workFn, col)
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				b.processBatch(ctx, batch, workFn, col)
				return nil
			}
			batch = append(batch, ev)
			if len(batch) >= b.batchSize {
				b.processBatch(ctx, batch, workFn, col)
				batch = make([]event.Event, 0, b.batchSize)
			}
		}
	}
}

func (b *Batching) processBatch(ctx context.Context, batch []event.Event, workFn workload.Func, col collector.Collector) {
	var wg sync.WaitGroup

	for _, ev := range batch {
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
}