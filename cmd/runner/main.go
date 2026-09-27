package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/urfave/cli/v2"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/config"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/exporter"
	"concurrency-lab/internal/template"
)

// workloadTypeName converts an event.WorkloadType into the string used in
// exported results ("CPU" or "IO").
func workloadTypeName(wt event.WorkloadType) string {
	switch wt {
	case event.CPU:
		return "CPU"
	case event.IO:
		return "IO"
	default:
		return "UNKNOWN"
	}
}

func main() {
	app := &cli.App{
		Name:  "concurrency-lab",
		Usage: "experimental framework for comparing Go concurrency strategies",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "run a scenario battery from a TOML preset",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "preset",
						Usage:    "path to the TOML preset file",
						Required: true,
					},
					&cli.IntFlag{
						Name:  "events",
						Usage: "override total_events for every scenario",
					},
					&cli.IntFlag{
						Name:  "rate",
						Usage: "override rate_per_second for every scenario",
					},
					&cli.StringFlag{
						Name:  "timeout",
						Usage: `override the per-scenario timeout (e.g. "120s")`,
					},
					&cli.StringFlag{
						Name:  "output",
						Usage: "output directory for the exported CSV",
						Value: "results",
					},
				},
				Action: runAction,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runAction(c *cli.Context) error {
	cfg, err := config.LoadTOML(c.String("preset"))
	if err != nil {
		return fmt.Errorf("loading preset: %w", err)
	}

	if c.IsSet("events") {
		events := c.Int("events")
		cfg.Defaults.TotalEvents = events
		for i := range cfg.Scenarios {
			cfg.Scenarios[i].TotalEvents = events
		}
	}
	if c.IsSet("rate") {
		rate := c.Int("rate")
		cfg.Defaults.RatePerSecond = rate
		for i := range cfg.Scenarios {
			cfg.Scenarios[i].RatePerSecond = rate
		}
	}
	if c.IsSet("timeout") {
		cfg.Defaults.Timeout = c.String("timeout")
	}

	scenarios, timeout, kafkaCfg, err := cfg.BuildScenarios()
	if err != nil {
		return fmt.Errorf("building scenarios: %w", err)
	}

	var results []exporter.RunResult

	for _, scn := range scenarios {
		col := collector.NewInMemoryCollector()

		var tmpl template.Template
		switch scn.TemplateName {
		case "", "inmemory":
			tmpl = template.NewInMemory()
		case "kafka":
			tmpl = template.NewKafka(kafkaCfg.Brokers, kafkaCfg.Topic)
		default:
			return fmt.Errorf("scenario %q: unknown template %q", scn.Name, scn.TemplateName)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		err := tmpl.Execute(ctx, scn, col)
		cancel()
		if err != nil {
			log.Printf("scenario %q failed: %v", scn.Name, err)
		}

		r := col.Results()
		fmt.Printf("=== %s ===\n", scn.Name)
		fmt.Printf("Total Events:    %d\n", r.TotalEvents)
		fmt.Printf("Throughput:      %.2f events/s\n", r.Throughput)
		fmt.Printf("Avg Latency:     %dµs\n", r.AvgLatency.Microseconds())
		fmt.Printf("P50 Latency:     %dµs\n", r.P50Latency.Microseconds())
		fmt.Printf("P95 Latency:     %dµs\n", r.P95Latency.Microseconds())
		fmt.Printf("P99 Latency:     %dµs\n", r.P99Latency.Microseconds())
		fmt.Printf("Avg E2E:         %dµs\n", r.AvgEndToEnd.Microseconds())
		fmt.Printf("P50 E2E:         %dµs\n", r.P50EndToEnd.Microseconds())
		fmt.Printf("P95 E2E:         %dµs\n", r.P95EndToEnd.Microseconds())
		fmt.Printf("P99 E2E:         %dµs\n", r.P99EndToEnd.Microseconds())
		fmt.Printf("Errors:          %d\n", r.ErrorCount)
		fmt.Printf("Duration:        %v\n\n", r.TotalDuration.Round(time.Second))

		results = append(results, exporter.RunResult{
			Name:             scn.Name,
			TotalEvents:      scn.TotalEvents,
			RatePerSecond:    scn.RatePerSecond,
			WorkloadType:     workloadTypeName(scn.WorkloadType),
			WorkloadDuration: scn.WorkloadDuration,
			Metrics:          r,
		})
	}

	path, err := exporter.ExportCSV(results, c.String("output"))
	if err != nil {
		return fmt.Errorf("exporting results: %w", err)
	}
	fmt.Printf("Resultados exportados para: %s\n", path)

	return nil
}
