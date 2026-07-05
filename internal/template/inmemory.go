package template

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/scenario"
	"concurrency-lab/internal/workload"
)

// InMemory is a Template that generates events in memory and feeds them to the strategy.
type InMemory struct{}

func NewInMemory() *InMemory {
	return &InMemory{}
}

func (t *InMemory) Execute(ctx context.Context, scn scenario.Scenario, col collector.Collector) error {
	events := make(chan event.Event, scn.RatePerSecond)

	go func() {
		defer close(events)
		ticker := time.NewTicker(time.Second / time.Duration(scn.RatePerSecond))
		defer ticker.Stop()

		sent := 0
		for sent < scn.TotalEvents {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ev := event.NewEvent(
					int64(100+rand.Intn(99901)),
					event.Currency(rand.Intn(3)),
					randomUser(),
					randomUser(),
					scn.WorkloadType,
				)
				events <- ev
				sent++
			}
		}
	}()

	workFn := workloadFunc(scn.WorkloadType, scn.WorkloadDuration)

	start := time.Now()
	err := scn.Strategy.Run(ctx, events, workFn, col)
	col.SetTotalDuration(time.Since(start))

	return err
}

func workloadFunc(wt event.WorkloadType, duration time.Duration) workload.Func {
	switch wt {
	case event.CPU:
		return workload.CPU(duration)
	default:
		return workload.IO(duration)
	}
}

func randomUser() string {
	return fmt.Sprintf("user-%d", rand.Intn(1000))
}
