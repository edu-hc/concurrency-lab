package scenario

import (
	"time"

	"concurrency-lab/internal/event"
	"concurrency-lab/internal/strategy"
)

// Scenario defines the parameters of a concurrency experiment.
type Scenario struct {
	Name             string
	TotalEvents      int
	RatePerSecond    int
	WorkloadType     event.WorkloadType
	WorkloadDuration time.Duration
	Strategy         strategy.Strategy
	// TemplateName selects which Template the runner should use to execute
	// this scenario (e.g. "inmemory", "kafka").
	TemplateName string
}
