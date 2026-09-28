package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// Backpressure processes events using a fixed pool of N worker goroutines
// reading from a bounded internal channel. When the internal channel is
// full, the dispatcher drops the event instead of blocking, reporting it to
// the collector as a drop rather than a processed event.
type Backpressure struct {
	numWorkers int
	bufferSize int
}

func NewBackpressure(numWorkers int, bufferSize int) *Backpressure {
	return &Backpressure{
		numWorkers: numWorkers,
		bufferSize: bufferSize,
	}
}

func (bp *Backpressure) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	internal := make(chan event.Event, bp.bufferSize)

	var wg sync.WaitGroup

	for i := 0; i < bp.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range internal {
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(internal)
		for ev := range events {
			if ctx.Err() != nil {
				return
			}
			select {
			case internal <- ev:
			default:
				col.RecordDropped(ev.ID)
			}
		}
	}()

	wg.Wait()
	return nil
}
