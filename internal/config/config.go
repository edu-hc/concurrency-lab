package config

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"

	"concurrency-lab/internal/event"
	"concurrency-lab/internal/scenario"
)

// Config is the root of a TOML scenario battery file: shared defaults plus
// the list of scenarios to build from them.
type Config struct {
	Defaults  DefaultsConfig   `toml:"defaults"`
	Kafka     KafkaConfig      `toml:"kafka"`
	Scenarios []ScenarioConfig `toml:"scenarios"`
}

// KafkaConfig holds the connection details for the Kafka template.
type KafkaConfig struct {
	Brokers []string `toml:"brokers"`
	Topic   string   `toml:"topic"`
}

// DefaultsConfig holds the fallback values applied to any ScenarioConfig
// field left empty or zero.
type DefaultsConfig struct {
	TotalEvents      int    `toml:"total_events"`
	RatePerSecond    int    `toml:"rate_per_second"`
	WorkloadDuration string `toml:"workload_duration"`
	Timeout          string `toml:"timeout"`
	// Template selects which Template runs the scenarios: "inmemory"
	// (default) or "kafka".
	Template string `toml:"template"`
}

// ScenarioConfig is the TOML representation of a single scenario. Fields
// shared with DefaultsConfig are optional and inherit the default value
// when left empty/zero; strategy-specific fields (Workers, BatchSize,
// BufferSize, MaxGoroutines, WorkersPerStage, NumShards) are only read by
// the strategy named in Strategy.
type ScenarioConfig struct {
	Name             string `toml:"name"`
	WorkloadType     string `toml:"workload_type"`
	TotalEvents      int    `toml:"total_events"`
	RatePerSecond    int    `toml:"rate_per_second"`
	WorkloadDuration string `toml:"workload_duration"`
	Template         string `toml:"template"`
	Strategy         string `toml:"strategy"`
	Workers          int    `toml:"workers"`
	BatchSize        int    `toml:"batch_size"`
	BufferSize       int    `toml:"buffer_size"`
	MaxGoroutines    int    `toml:"max_goroutines"`
	WorkersPerStage  int    `toml:"workers_per_stage"`
	NumShards        int    `toml:"num_shards"`
}

// LoadTOML reads and parses a scenario battery file at path.
func LoadTOML(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading TOML config from %q: %w", path, err)
	}
	return &cfg, nil
}

// defaultTemplateName is used when neither a scenario nor Defaults sets Template.
const defaultTemplateName = "inmemory"

// BuildScenarios turns every ScenarioConfig into a scenario.Scenario,
// applying defaults for empty/zero fields and instantiating the configured
// strategy. It also returns the shared timeout parsed from Defaults.Timeout
// and the Kafka connection details, so the runner can pass them to the
// Kafka template when a scenario asks for it.
func (c *Config) BuildScenarios() ([]scenario.Scenario, time.Duration, *KafkaConfig, error) {
	timeoutStr := c.Defaults.Timeout
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("parsing defaults.timeout %q: %w", timeoutStr, err)
	}

	defaultTemplate := c.Defaults.Template
	if defaultTemplate == "" {
		defaultTemplate = defaultTemplateName
	}

	scenarios := make([]scenario.Scenario, 0, len(c.Scenarios))
	for _, sc := range c.Scenarios {
		totalEvents := sc.TotalEvents
		if totalEvents == 0 {
			totalEvents = c.Defaults.TotalEvents
		}

		ratePerSecond := sc.RatePerSecond
		if ratePerSecond == 0 {
			ratePerSecond = c.Defaults.RatePerSecond
		}

		workloadDurationStr := sc.WorkloadDuration
		if workloadDurationStr == "" {
			workloadDurationStr = c.Defaults.WorkloadDuration
		}
		workloadDuration, err := time.ParseDuration(workloadDurationStr)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("scenario %q: parsing workload_duration %q: %w", sc.Name, workloadDurationStr, err)
		}

		wt, err := parseWorkloadType(sc.WorkloadType)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("scenario %q: %w", sc.Name, err)
		}

		templateName := sc.Template
		if templateName == "" {
			templateName = defaultTemplate
		}

		strat, err := BuildStrategy(sc)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("scenario %q: %w", sc.Name, err)
		}

		scenarios = append(scenarios, scenario.Scenario{
			Name:             sc.Name,
			TotalEvents:      totalEvents,
			RatePerSecond:    ratePerSecond,
			WorkloadType:     wt,
			WorkloadDuration: workloadDuration,
			Strategy:         strat,
			TemplateName:     templateName,
		})
	}

	return scenarios, timeout, &c.Kafka, nil
}

// parseWorkloadType maps a scenario's workload_type string to event.WorkloadType.
// An empty string falls back to event.CPU, the type's zero value.
func parseWorkloadType(s string) (event.WorkloadType, error) {
	switch s {
	case "", "CPU":
		return event.CPU, nil
	case "IO":
		return event.IO, nil
	default:
		return 0, fmt.Errorf("unknown workload_type %q", s)
	}
}
