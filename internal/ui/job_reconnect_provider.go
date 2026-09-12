package ui

import (
	"context"
	"errors"
	"time"

	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/operationsview"
)

var ErrInvalidJobReconnectProvider = errors.New("invalid job reconnect provider")

// JobReconnectProvider is the read-only boundary used by the operational UI
// when it resumes observation of one existing durable Job. Implementations must
// not claim leases, retry, cancel or otherwise mutate Job state.
type JobReconnectProvider interface {
	JobReconnect(context.Context, string, uint64) (operationsview.JobReconnectSnapshot, error)
}

type JobReader interface {
	Get(context.Context, string) (job.Job, error)
}

type JobTimelineReader interface {
	List(context.Context, string) ([]job.TimelineEntry, error)
}

type DurableJobReconnectProvider struct {
	Jobs     JobReader
	Timeline JobTimelineReader
	Now      func() time.Time
}

func (provider DurableJobReconnectProvider) JobReconnect(ctx context.Context, jobID string, afterVersion uint64) (operationsview.JobReconnectSnapshot, error) {
	if provider.Jobs == nil || provider.Now == nil {
		return operationsview.JobReconnectSnapshot{}, ErrInvalidJobReconnectProvider
	}
	current, err := provider.Jobs.Get(ctx, jobID)
	if err != nil {
		return operationsview.JobReconnectSnapshot{}, err
	}
	now := provider.Now()
	if provider.Timeline == nil {
		return operationsview.BuildJobReconnectSnapshot(operationsview.JobReconnectInput{
			Job:            current,
			TimelineLoaded: false,
			AfterVersion:   afterVersion,
			EvaluatedAt:    now,
		})
	}
	entries, err := provider.Timeline.List(ctx, jobID)
	if err != nil {
		return operationsview.BuildJobReconnectSnapshot(operationsview.JobReconnectInput{
			Job:            current,
			TimelineLoaded: false,
			AfterVersion:   afterVersion,
			EvaluatedAt:    now,
		})
	}
	return operationsview.BuildJobReconnectSnapshot(operationsview.JobReconnectInput{
		Job:            current,
		Timeline:       entries,
		TimelineLoaded: true,
		AfterVersion:   afterVersion,
		EvaluatedAt:    now,
	})
}
