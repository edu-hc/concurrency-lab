package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// WorkerPool processes events using a fixed pool of N worker goroutines.
type WorkerPool struct {
	numWorkers int
}

func NewWorkerPool(numWorkers int) *WorkerPool {
	return &WorkerPool{numWorkers: numWorkers}
}

func (wp *WorkerPool) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	var wg sync.WaitGroup

	for i := 0; i < wp.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range events {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()
				err := workFn(ctx)
				processingTime := time.Since(start)
				endToEndTime := time.Since(ev.CreatedAt)
				col.Record(ev.ID, processingTime, endToEndTime, err)
			}
		}()
	}

	wg.Wait()
	return nil
}
