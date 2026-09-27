package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// ContentionRWMutex processes events using a fixed pool of N worker goroutines
// that share a single balances map guarded by a sync.RWMutex, separating the
// read of the sender's balance from the write that debits/credits balances.
type ContentionRWMutex struct {
	numWorkers int
	balances   map[string]int64
	mu         sync.RWMutex
}

func NewContentionRWMutex(numWorkers int) *ContentionRWMutex {
	return &ContentionRWMutex{
		numWorkers: numWorkers,
		balances:   make(map[string]int64),
	}
}

func (cm *ContentionRWMutex) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	var wg sync.WaitGroup

	for i := 0; i < cm.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range events {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()

				cm.mu.RLock()
				_ = cm.balances[ev.Sender]
				cm.mu.RUnlock()

				cm.mu.Lock()
				cm.balances[ev.Sender] -= ev.Amount
				cm.balances[ev.Receiver] += ev.Amount
				cm.mu.Unlock()

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
