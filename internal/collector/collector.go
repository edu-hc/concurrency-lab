package collector

import (
	"sort"
	"sync"
	"time"
)

// Collector records processing results for individual events.
type Collector interface {
	Record(eventID string, duration time.Duration, err error)
	Results() *Results
}

// Results holds aggregated metrics for a completed experiment.
type Results struct {
	TotalEvents   int
	Throughput    float64 // events per second
	AvgLatency    time.Duration
	P50Latency    time.Duration
	P95Latency    time.Duration
	P99Latency    time.Duration
	ErrorCount    int
	TotalDuration time.Duration
}

// InMemoryCollector is a thread-safe, in-memory implementation of Collector.
type InMemoryCollector struct {
	mu            sync.Mutex
	durations     []time.Duration
	errorCount    int
	totalDuration time.Duration
}

func NewInMemoryCollector() *InMemoryCollector {
	return &InMemoryCollector{}
}

func (c *InMemoryCollector) Record(eventID string, duration time.Duration, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.durations = append(c.durations, duration)
	if err != nil {
		c.errorCount++
	}
}

func (c *InMemoryCollector) SetTotalDuration(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalDuration = d
}

func (c *InMemoryCollector) Results() *Results {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(c.durations)
	if n == 0 {
		return &Results{
			ErrorCount:    c.errorCount,
			TotalDuration: c.totalDuration,
		}
	}

	sorted := make([]time.Duration, n)
	copy(sorted, c.durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	var throughput float64
	if c.totalDuration > 0 {
		throughput = float64(n) / c.totalDuration.Seconds()
	}

	return &Results{
		TotalEvents:   n,
		Throughput:    throughput,
		AvgLatency:    sum / time.Duration(n),
		P50Latency:    sorted[percentileIndex(n, 50)],
		P95Latency:    sorted[percentileIndex(n, 95)],
		P99Latency:    sorted[percentileIndex(n, 99)],
		ErrorCount:    c.errorCount,
		TotalDuration: c.totalDuration,
	}
}

// percentileIndex returns the slice index for the given percentile (0–100).
func percentileIndex(n, p int) int {
	i := (n * p / 100)
	if i >= n {
		i = n - 1
	}
	return i
}
