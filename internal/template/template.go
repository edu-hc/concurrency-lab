package template

import (
	"context"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/scenario"
)

// Template is the infrastructure context where a strategy runs.
type Template interface {
	Execute(ctx context.Context, scn scenario.Scenario, col collector.Collector) error
}
