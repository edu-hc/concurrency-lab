package template

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/scenario"
	"concurrency-lab/internal/workload"
)

// minTickInterval is the floor for the generator's ticker period. Go's
// timer resolution can't reliably sustain sub-millisecond ticks (a naive
// "one event per tick" generator silently falls far short of high nominal
// rates, e.g. 50000/s = 20us/event), so instead of ticking that fast the
// generator ticks at this coarser, reliably-honored interval and emits a
// batch of events per tick.
const minTickInterval = 1 * time.Millisecond

// InMemory is a Template that generates events in memory and feeds them to the strategy.
type InMemory struct{}

func NewInMemory() *InMemory {
	return &InMemory{}
}

func (t *InMemory) Execute(ctx context.Context, scn scenario.Scenario, col collector.Collector) error {
	events := make(chan event.Event, scn.RatePerSecond)

	go generate(ctx, scn, events)

	workFn := workloadFunc(scn.WorkloadType, scn.WorkloadDuration)

	start := time.Now()
	err := scn.Strategy.Run(ctx, events, workFn, col)
	col.SetTotalDuration(time.Since(start))

	return err
}

// generate emits scn.TotalEvents events at scn.RatePerSecond, batching
// several events per tick (see minTickInterval) instead of relying on a
// single-event-per-tick ticker fine enough to hit the rate directly. A
// fractional carry accumulates leftover events across ticks so the average
// rate over time stays accurate despite the integer events-per-tick count.
// Once generation finishes (or is cancelled), it logs the actual achieved
// rate against the nominal target.
func generate(ctx context.Context, scn scenario.Scenario, events chan<- event.Event) {
	defer close(events)

	interval := time.Second / time.Duration(scn.RatePerSecond)
	if interval < minTickInterval {
		interval = minTickInterval
	}
	eventsPerTick := float64(scn.RatePerSecond) * interval.Seconds()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	sent := 0
	var carry float64

	for sent < scn.TotalEvents {
		select {
		case <-ctx.Done():
			logGenerationRate(scn, sent, time.Since(start), "cancelled")
			return
		case <-ticker.C:
			carry += eventsPerTick
			n := int(carry)
			carry -= float64(n)

			for i := 0; i < n && sent < scn.TotalEvents; i++ {
				ev := event.NewEvent(
					int64(100+rand.Intn(99901)),
					event.Currency(rand.Intn(3)),
					randomUser(),
					randomUser(),
					scn.WorkloadType,
				)
				select {
				case events <- ev:
					sent++
				case <-ctx.Done():
					logGenerationRate(scn, sent, time.Since(start), "cancelled")
					return
				}
			}
		}
	}

	logGenerationRate(scn, sent, time.Since(start), "done")
}

func logGenerationRate(scn scenario.Scenario, sent int, elapsed time.Duration, status string) {
	rate := float64(sent) / elapsed.Seconds()
	log.Printf("generator %s: %q emitted %d/%d events in %v (%.0f events/s, target %d/s)",
		status, scn.Name, sent, scn.TotalEvents, elapsed.Round(time.Millisecond), rate, scn.RatePerSecond)
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
