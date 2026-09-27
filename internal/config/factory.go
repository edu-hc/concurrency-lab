package config

import (
	"fmt"

	"concurrency-lab/internal/strategy"
)

// BuildStrategy maps a ScenarioConfig's Strategy name to its constructor,
// reading only the fields that constructor needs.
func BuildStrategy(cfg ScenarioConfig) (strategy.Strategy, error) {
	switch cfg.Strategy {
	case "workerpool":
		return strategy.NewWorkerPool(cfg.Workers), nil
	case "ondemand":
		return strategy.NewOnDemand(), nil
	case "ondemand_limited":
		return strategy.NewOnDemandLimited(cfg.MaxGoroutines), nil
	case "batching":
		return strategy.NewBatching(cfg.BatchSize), nil
	case "contention_mutex":
		return strategy.NewContentionMutex(cfg.Workers), nil
	case "contention_rwmutex":
		return strategy.NewContentionRWMutex(cfg.Workers), nil
	case "sharding":
		return strategy.NewSharding(cfg.NumShards), nil
	case "pipeline":
		return strategy.NewPipeline(cfg.WorkersPerStage), nil
	case "backpressure":
		return strategy.NewBackpressure(cfg.Workers, cfg.BufferSize), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", cfg.Strategy)
	}
}
