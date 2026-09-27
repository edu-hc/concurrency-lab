package strategy

import (
	"context"
	"sync"
	"time"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/workload"
)

const pipelineStageBuffer = 64

// Pipeline processes events through three sequential stages — validation,
// processing, and recording — each backed by its own pool of worker
// goroutines connected by buffered channels.
type Pipeline struct {
	workersPerStage int
}

func NewPipeline(workersPerStage int) *Pipeline {
	return &Pipeline{workersPerStage: workersPerStage}
}

// pipelineEvent carries the original event along with the timestamp at which
// it entered stage 1, so the total processing time across all three stages
// can be measured once the event reaches stage 3.
type pipelineEvent struct {
	event.Event
	startedAt time.Time
}

func (p *Pipeline) Run(ctx context.Context, events <-chan event.Event, workFn workload.Func, col collector.Collector) error {
	stage2In := make(chan pipelineEvent, pipelineStageBuffer)
	stage3In := make(chan pipelineEvent, pipelineStageBuffer)

	var stage1WG, stage2WG, stage3WG sync.WaitGroup

	// Stage 1: validation.
	for i := 0; i < p.workersPerStage; i++ {
		stage1WG.Add(1)
		go func() {
			defer stage1WG.Done()
			for ev := range events {
				if ctx.Err() != nil {
					return
				}
				pe := pipelineEvent{Event: ev, startedAt: time.Now()}
				_ = workFn(ctx)

				select {
				case stage2In <- pe:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Stage 2: processing.
	for i := 0; i < p.workersPerStage; i++ {
		stage2WG.Add(1)
		go func() {
			defer stage2WG.Done()
			for pe := range stage2In {
				if ctx.Err() != nil {
					return
				}
				_ = workFn(ctx)

				select {
				case stage3In <- pe:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Stage 3: recording.
	for i := 0; i < p.workersPerStage; i++ {
		stage3WG.Add(1)
		go func() {
			defer stage3WG.Done()
			for pe := range stage3In {
				if ctx.Err() != nil {
					return
				}
				err := workFn(ctx)
				processingTime := time.Since(pe.startedAt)
				endToEndTime := time.Since(pe.CreatedAt)
				col.Record(pe.ID, processingTime, endToEndTime, err)
			}
		}()
	}

	go func() {
		stage1WG.Wait()
		close(stage2In)
	}()

	go func() {
		stage2WG.Wait()
		close(stage3In)
	}()

	stage3WG.Wait()
	return nil
}