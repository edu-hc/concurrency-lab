package strategy

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

const shardChannelBuffer = 64

// Sharding processes events using N independent shards, each owned by a
// single worker goroutine with its own balances map. Events are routed to a
// shard by hashing the sender's ID, so no shard's map is ever touched by
// more than one goroutine and no locking is needed.
type Sharding struct {
	numShards int
}

func NewSharding(numShards int) *Sharding {
	return &Sharding{numShards: numShards}
}

func (s *Sharding) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	shardChannels := make([]chan event.Event, s.numShards)
	for i := range shardChannels {
		shardChannels[i] = make(chan event.Event, shardChannelBuffer)
	}

	var wg sync.WaitGroup
	for i := 0; i < s.numShards; i++ {
		wg.Add(1)
		go func(shardIdx int) {
			defer wg.Done()
			balances := make(map[string]int64)
			for ev := range shardChannels[shardIdx] {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()

				balances[ev.Sender] -= ev.Amount
				balances[ev.Receiver] += ev.Amount

				err := workFn(ctx)
				processingTime := time.Since(start)
				endToEndTime := time.Since(ev.CreatedAt)
				col.Record(ev.ID, processingTime, endToEndTime, err)
			}
		}(i)
	}

	go func() {
		defer func() {
			for _, ch := range shardChannels {
				close(ch)
			}
		}()
		for ev := range events {
			if ctx.Err() != nil {
				return
			}
			shard := shardFor(ev.Sender, s.numShards)
			select {
			case shardChannels[shard] <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	return nil
}

func shardFor(sender string, numShards int) int {
	h := fnv.New32a()
	h.Write([]byte(sender))
	return int(h.Sum32() % uint32(numShards))
}