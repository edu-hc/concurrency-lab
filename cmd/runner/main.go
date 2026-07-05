package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/exporter"
	"concurrency-lab/internal/scenario"
	"concurrency-lab/internal/strategy"
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
	scenarios := []scenario.Scenario{
		// I/O bound - Worker Pool com poucos workers (gargalo no processamento)
		{
			Name:             "IO - Worker Pool (4 workers)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewWorkerPool(4),
		},
		{
			Name:             "IO - Worker Pool (10 workers)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewWorkerPool(10),
		},
		{
			Name:             "IO - On-Demand",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemand(),
		},
		// CPU bound - mesma comparação
		{
			Name:             "CPU - Worker Pool (4 workers)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewWorkerPool(4),
		},
		{
			Name:             "CPU - Worker Pool (10 workers)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewWorkerPool(10),
		},
		{
			Name:             "CPU - On-Demand",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemand(),
		},
		{
			Name:             "IO - Batching (batch=10)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewBatching(10),
		},
		{
			Name:             "CPU - Batching (batch=10)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewBatching(10),
		},
		{
			Name:             "IO - OnDemand Limited (10)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemandLimited(10),
		},
		{
			Name:             "IO - OnDemand Limited (50)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.IO,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemandLimited(50),
		},
		{
			Name:             "CPU - OnDemand Limited (10)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemandLimited(10),
		},
		{
			Name:             "CPU - OnDemand Limited (50)",
			TotalEvents:      10000,
			RatePerSecond:    10000,
			WorkloadType:     event.CPU,
			WorkloadDuration: 5 * time.Millisecond,
			Strategy:         strategy.NewOnDemandLimited(50),
		},
	}

	var results []exporter.RunResult

	for _, scn := range scenarios {
		col := collector.NewInMemoryCollector()
		tmpl := template.NewInMemory()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

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

	path, err := exporter.ExportCSV(results, "results")
	if err != nil {
		log.Fatalf("exporting results: %v", err)
	}
	fmt.Printf("Resultados exportados para: %s\n", path)
}
