package strategy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// Backpressure processes events using a fixed pool of N worker goroutines
// reading from a bounded internal channel. When the internal channel is
// full, the dispatcher drops the event instead of blocking, recording it as
// a failure and incrementing the dropped counter.
type Backpressure struct {
	numWorkers int
	bufferSize int
	dropped    int64
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
				atomic.AddInt64(&bp.dropped, 1)
				col.Record(ev.ID, 0, time.Since(ev.CreatedAt), fmt.Errorf("dropped: buffer full"))
			}
		}
	}()

	wg.Wait()
	return nil
}