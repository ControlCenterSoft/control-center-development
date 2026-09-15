package postgres

import (
	"context"
	"testing"
	"time"

	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/job"
)

func TestJobMutationsRejectZeroTimeBeforeDatabaseAccess(t *testing.T) {
	repository := &JobRepository{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "renew lease",
			call: func() error {
				_, err := repository.RenewLease(ctx, "job-1", "lease-1", time.Time{}, time.Minute)
				return err
			},
		},
		{
			name: "succeed",
			call: func() error {
				_, err := repository.Succeed(ctx, "job-1", "lease-1", events.Output{}, time.Time{})
				return err
			},
		},
		{
			name: "fail",
			call: func() error {
				_, err := repository.Fail(ctx, "job-1", "lease-1", "temporary", events.Output{}, job.RetryPolicy{BaseDelay: time.Second}, time.Time{})
				return err
			},
		},
		{
			name: "request cancel",
			call: func() error {
				_, err := repository.RequestCancel(ctx, "job-1", time.Time{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil {
				t.Fatal("zero-time mutation must fail before database access")
			}
		})
	}
}
