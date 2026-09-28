package collector

import (
	"sort"
	"time"
)

// Spread holds the interquartile range (Q3 - Q1) of each Results metric
// across repeated runs of the same scenario, as a measure of variability
// alongside the median point estimate returned by Aggregate.
type Spread struct {
	ThroughputIQR    float64
	AvgLatencyIQR    time.Duration
	P50LatencyIQR    time.Duration
	P95LatencyIQR    time.Duration
	P99LatencyIQR    time.Duration
	AvgEndToEndIQR   time.Duration
	P50EndToEndIQR   time.Duration
	P95EndToEndIQR   time.Duration
	P99EndToEndIQR   time.Duration
	ErrorCountIQR    float64
	DroppedCountIQR  float64
	TotalDurationIQR time.Duration
}

// Aggregate reduces multiple independent runs of the same scenario into a
// single Results holding the per-metric median, and a Spread holding the
// per-metric interquartile range. A single run's variability is undefined,
// so with one run Aggregate returns that run's own Results and a zero
// Spread. Aggregate panics if runs is empty.
func Aggregate(runs []*Results) (*Results, *Spread) {
	if len(runs) == 0 {
		panic("collector.Aggregate: no runs to aggregate")
	}
	if len(runs) == 1 {
		return runs[0], &Spread{}
	}

	totalEvents := make([]float64, len(runs))
	throughput := make([]float64, len(runs))
	avgLatency := make([]float64, len(runs))
	p50Latency := make([]float64, len(runs))
	p95Latency := make([]float64, len(runs))
	p99Latency := make([]float64, len(runs))
	avgE2E := make([]float64, len(runs))
	p50E2E := make([]float64, len(runs))
	p95E2E := make([]float64, len(runs))
	p99E2E := make([]float64, len(runs))
	errorCount := make([]float64, len(runs))
	droppedCount := make([]float64, len(runs))
	totalDuration := make([]float64, len(runs))

	for i, r := range runs {
		totalEvents[i] = float64(r.TotalEvents)
		throughput[i] = r.Throughput
		avgLatency[i] = float64(r.AvgLatency)
		p50Latency[i] = float64(r.P50Latency)
		p95Latency[i] = float64(r.P95Latency)
		p99Latency[i] = float64(r.P99Latency)
		avgE2E[i] = float64(r.AvgEndToEnd)
		p50E2E[i] = float64(r.P50EndToEnd)
		p95E2E[i] = float64(r.P95EndToEnd)
		p99E2E[i] = float64(r.P99EndToEnd)
		errorCount[i] = float64(r.ErrorCount)
		droppedCount[i] = float64(r.DroppedCount)
		totalDuration[i] = float64(r.TotalDuration)
	}

	median := &Results{
		TotalEvents:   int(medianOf(totalEvents)),
		Throughput:    medianOf(throughput),
		AvgLatency:    time.Duration(medianOf(avgLatency)),
		P50Latency:    time.Duration(medianOf(p50Latency)),
		P95Latency:    time.Duration(medianOf(p95Latency)),
		P99Latency:    time.Duration(medianOf(p99Latency)),
		AvgEndToEnd:   time.Duration(medianOf(avgE2E)),
		P50EndToEnd:   time.Duration(medianOf(p50E2E)),
		P95EndToEnd:   time.Duration(medianOf(p95E2E)),
		P99EndToEnd:   time.Duration(medianOf(p99E2E)),
		ErrorCount:    int(medianOf(errorCount)),
		DroppedCount:  int(medianOf(droppedCount)),
		TotalDuration: time.Duration(medianOf(totalDuration)),
	}

	spread := &Spread{
		ThroughputIQR:    iqrOf(throughput),
		AvgLatencyIQR:    time.Duration(iqrOf(avgLatency)),
		P50LatencyIQR:    time.Duration(iqrOf(p50Latency)),
		P95LatencyIQR:    time.Duration(iqrOf(p95Latency)),
		P99LatencyIQR:    time.Duration(iqrOf(p99Latency)),
		AvgEndToEndIQR:   time.Duration(iqrOf(avgE2E)),
		P50EndToEndIQR:   time.Duration(iqrOf(p50E2E)),
		P95EndToEndIQR:   time.Duration(iqrOf(p95E2E)),
		P99EndToEndIQR:   time.Duration(iqrOf(p99E2E)),
		ErrorCountIQR:    iqrOf(errorCount),
		DroppedCountIQR:  iqrOf(droppedCount),
		TotalDurationIQR: time.Duration(iqrOf(totalDuration)),
	}

	return median, spread
}

func medianOf(xs []float64) float64 {
	return quantileOf(xs, 0.5)
}

func iqrOf(xs []float64) float64 {
	return quantileOf(xs, 0.75) - quantileOf(xs, 0.25)
}

func quantileOf(xs []float64, p float64) float64 {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)

	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}

	pos := p * float64(n-1)
	lo := int(pos)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := pos - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}
