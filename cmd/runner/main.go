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
					&cli.IntFlag{
						Name:  "repeat",
						Usage: "run each scenario N times and report the median and IQR across runs (recommend 5+ for a real confidence interval)",
						Value: 1,
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

// durationIQR formats a Spread field as " (IQR ±Xµs)" when repeat > 1, so a
// single run's printout stays exactly as before.
func durationIQR(iqr time.Duration, repeat int) string {
	if repeat <= 1 {
		return ""
	}
	return fmt.Sprintf(" (IQR %dµs)", iqr.Microseconds())
}

func floatIQR(iqr float64, repeat int) string {
	if repeat <= 1 {
		return ""
	}
	return fmt.Sprintf(" (IQR %.2f)", iqr)
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

	repeat := c.Int("repeat")
	if repeat < 1 {
		repeat = 1
	}

	var results []exporter.RunResult

	for _, scn := range scenarios {
		var tmpl template.Template
		switch scn.TemplateName {
		case "", "inmemory":
			tmpl = template.NewInMemory()
		case "kafka":
			tmpl = template.NewKafka(kafkaCfg.Brokers, kafkaCfg.Topic)
		default:
			return fmt.Errorf("scenario %q: unknown template %q", scn.Name, scn.TemplateName)
		}

		runs := make([]*collector.Results, 0, repeat)
		for i := 0; i < repeat; i++ {
			if repeat > 1 {
				log.Printf("scenario %q: run %d/%d", scn.Name, i+1, repeat)
			}

			col := collector.NewInMemoryCollector()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := tmpl.Execute(ctx, scn, col)
			cancel()
			if err != nil {
				log.Printf("scenario %q failed: %v", scn.Name, err)
			}

			runs = append(runs, col.Results())
		}

		r, spread := collector.Aggregate(runs)

		fmt.Printf("=== %s ===\n", scn.Name)
		if repeat > 1 {
			fmt.Printf("Repeats:         %d\n", repeat)
		}
		fmt.Printf("Total Events:    %d\n", r.TotalEvents)
		fmt.Printf("Throughput:      %.2f events/s%s\n", r.Throughput, floatIQR(spread.ThroughputIQR, repeat))
		fmt.Printf("Avg Latency:     %dµs\n", r.AvgLatency.Microseconds())
		fmt.Printf("P50 Latency:     %dµs%s\n", r.P50Latency.Microseconds(), durationIQR(spread.P50LatencyIQR, repeat))
		fmt.Printf("P95 Latency:     %dµs\n", r.P95Latency.Microseconds())
		fmt.Printf("P99 Latency:     %dµs\n", r.P99Latency.Microseconds())
		fmt.Printf("Avg E2E:         %dµs\n", r.AvgEndToEnd.Microseconds())
		fmt.Printf("P50 E2E:         %dµs%s\n", r.P50EndToEnd.Microseconds(), durationIQR(spread.P50EndToEndIQR, repeat))
		fmt.Printf("P95 E2E:         %dµs\n", r.P95EndToEnd.Microseconds())
		fmt.Printf("P99 E2E:         %dµs\n", r.P99EndToEnd.Microseconds())
		fmt.Printf("Errors:          %d\n", r.ErrorCount)
		fmt.Printf("Dropped:         %d\n", r.DroppedCount)
		fmt.Printf("Duration:        %v\n\n", r.TotalDuration.Round(time.Second))

		results = append(results, exporter.RunResult{
			Name:             scn.Name,
			TotalEvents:      scn.TotalEvents,
			RatePerSecond:    scn.RatePerSecond,
			WorkloadType:     workloadTypeName(scn.WorkloadType),
			WorkloadDuration: scn.WorkloadDuration,
			Repeats:          repeat,
			Metrics:          r,
			Spread:           spread,
		})
	}

	path, err := exporter.ExportCSV(results, c.String("output"))
	if err != nil {
		return fmt.Errorf("exporting results: %w", err)
	}
	fmt.Printf("Resultados exportados para: %s\n", path)

	return nil
}
