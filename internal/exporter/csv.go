package exporter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"concurrency-lab/internal/collector"
)

// RunResult groups the parameters of an executed scenario with the metrics
// collected for it, ready to be exported. Metrics is the median across
// Repeats runs (or the single run's own Results when Repeats is 1); Spread
// is the matching per-metric interquartile range, zero when Repeats is 1.
type RunResult struct {
	Name             string
	TotalEvents      int
	RatePerSecond    int
	WorkloadType     string
	WorkloadDuration time.Duration
	Repeats          int
	Metrics          *collector.Results
	Spread           *collector.Spread
}

// ExportCSV writes results to a timestamped CSV file inside dir and returns
// the path to the generated file.
func ExportCSV(results []RunResult, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	filename := time.Now().Format("2006-01-02_150405") + ".csv"
	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating csv file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)

	header := []string{
		"name", "total_events", "rate_per_second", "workload_type", "workload_duration_us", "repeats",
		"events_processed", "events_dropped",
		"throughput", "throughput_iqr",
		"avg_latency_us", "p50_latency_us", "p95_latency_us", "p99_latency_us",
		"avg_e2e_us", "p50_e2e_us", "p95_e2e_us", "p99_e2e_us",
		"p50_latency_iqr_us", "p50_e2e_iqr_us",
		"errors", "duration_ms",
	}
	if err := w.Write(header); err != nil {
		return "", fmt.Errorf("writing csv header: %w", err)
	}

	for _, r := range results {
		spread := r.Spread
		if spread == nil {
			spread = &collector.Spread{}
		}
		repeats := r.Repeats
		if repeats == 0 {
			repeats = 1
		}

		record := []string{
			r.Name,
			strconv.Itoa(r.TotalEvents),
			strconv.Itoa(r.RatePerSecond),
			r.WorkloadType,
			strconv.FormatInt(r.WorkloadDuration.Microseconds(), 10),
			strconv.Itoa(repeats),
			strconv.Itoa(r.Metrics.TotalEvents),
			strconv.Itoa(r.Metrics.DroppedCount),
			strconv.FormatFloat(r.Metrics.Throughput, 'f', 2, 64),
			strconv.FormatFloat(spread.ThroughputIQR, 'f', 2, 64),
			strconv.FormatInt(r.Metrics.AvgLatency.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P50Latency.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P95Latency.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P99Latency.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.AvgEndToEnd.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P50EndToEnd.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P95EndToEnd.Microseconds(), 10),
			strconv.FormatInt(r.Metrics.P99EndToEnd.Microseconds(), 10),
			strconv.FormatInt(spread.P50LatencyIQR.Microseconds(), 10),
			strconv.FormatInt(spread.P50EndToEndIQR.Microseconds(), 10),
			strconv.Itoa(r.Metrics.ErrorCount),
			strconv.FormatInt(r.Metrics.TotalDuration.Milliseconds(), 10),
		}
		if err := w.Write(record); err != nil {
			return "", fmt.Errorf("writing csv record: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("flushing csv writer: %w", err)
	}

	return path, nil
}
