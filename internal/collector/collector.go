package collector

import (
	"sort"
	"sync"
	"time"
)

// Collector records processing results for individual events.
type Collector interface {
	// Record reports the outcome of an event that was actually attempted:
	// processingTime and endToEndTime are only meaningful for a completed
	// attempt (success or failure). err non-nil counts toward ErrorCount,
	// but its duration is excluded from the latency percentiles/averages
	// so a failed or cancelled attempt doesn't skew the numbers that
	// describe successful processing.
	Record(eventID string, processingTime time.Duration, endToEndTime time.Duration, err error)
	// RecordDropped reports an event that was never attempted at all —
	// shed at the door by a strategy under load (e.g. Backpressure) before
	// any processing began. It only increments DroppedCount; it does not
	// touch the latency percentiles/averages or Throughput's event count,
	// since a drop has no processing time to report.
	RecordDropped(eventID string)
	Results() *Results
	SetTotalDuration(d time.Duration)
}

// Results holds aggregated metrics for a completed experiment. All latency
// fields are computed only from events recorded via Record with a nil
// error; dropped events (RecordDropped) and failed/cancelled attempts
// (Record with a non-nil error) are excluded, so these numbers describe
// successful processing only.
type Results struct {
	TotalEvents   int
	Throughput    float64 // events per second, successful events only
	AvgLatency    time.Duration
	P50Latency    time.Duration
	P95Latency    time.Duration
	P99Latency    time.Duration
	AvgEndToEnd   time.Duration
	P50EndToEnd   time.Duration
	P95EndToEnd   time.Duration
	P99EndToEnd   time.Duration
	ErrorCount    int
	DroppedCount  int
	TotalDuration time.Duration
}

// InMemoryCollector is a thread-safe, in-memory implementation of Collector.
type InMemoryCollector struct {
	mu                sync.Mutex
	durations         []time.Duration
	endToEndDurations []time.Duration
	errorCount        int
	droppedCount      int
	totalDuration     time.Duration
}

func NewInMemoryCollector() *InMemoryCollector {
	return &InMemoryCollector{}
}

func (c *InMemoryCollector) Record(eventID string, processingTime time.Duration, endToEndTime time.Duration, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.errorCount++
		return
	}
	c.durations = append(c.durations, processingTime)
	c.endToEndDurations = append(c.endToEndDurations, endToEndTime)
}

func (c *InMemoryCollector) RecordDropped(eventID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.droppedCount++
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
			DroppedCount:  c.droppedCount,
			TotalDuration: c.totalDuration,
		}
	}

	sorted := make([]time.Duration, n)
	copy(sorted, c.durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	sortedE2E := make([]time.Duration, n)
	copy(sortedE2E, c.endToEndDurations)
	sort.Slice(sortedE2E, func(i, j int) bool { return sortedE2E[i] < sortedE2E[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	var sumE2E time.Duration
	for _, d := range sortedE2E {
		sumE2E += d
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
		AvgEndToEnd:   sumE2E / time.Duration(n),
		P50EndToEnd:   sortedE2E[percentileIndex(n, 50)],
		P95EndToEnd:   sortedE2E[percentileIndex(n, 95)],
		P99EndToEnd:   sortedE2E[percentileIndex(n, 99)],
		ErrorCount:    c.errorCount,
		DroppedCount:  c.droppedCount,
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
