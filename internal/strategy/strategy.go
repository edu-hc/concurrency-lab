package strategy

import (
	"context"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

// Strategy processes events from a channel using a given workload function,
// recording metrics into the collector.
type Strategy interface {
	Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error
}