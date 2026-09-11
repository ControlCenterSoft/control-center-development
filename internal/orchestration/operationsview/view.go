package operationsview

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"control-center/internal/orchestration/change"
	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/policy"
)

const ContractVersion = "ui.operations-change-job/v1"

var ErrInvalidView = errors.New("invalid operations change/job view")

type EvidenceAvailability string

const (
	EvidenceUnavailable EvidenceAvailability = "unavailable"
	EvidenceAvailable   EvidenceAvailability = "available"
)

type TimelineCompleteness string

const TimelineSnapshotOnly TimelineCompleteness = "snapshot_only"

type View struct {
	ContractVersion   string         `json:"contract_version"`
	GeneratedAt       time.Time      `json:"generated_at"`
	Change            ChangeSummary  `json:"change"`
	Job               *JobSummary    `json:"job,omitempty"`
	ResultEvidence    ResultEvidence `json:"result_evidence"`
	Timeline          Timeline       `json:"timeline"`
	AttentionRequired bool           `json:"attention_required"`
}

type ChangeSummary struct {
	ID                string      `json:"id"`
	Action            string      `json:"action"`
	Requester         string      `json:"requester"`
	RevisionID        string      `json:"revision_id"`
	Risk              policy.Risk `json:"risk"`
	State             change.State `json:"state"`
	ApprovalsRecorded int         `json:"approvals_recorded"`
	ApprovalsRequired int         `json:"approvals_required"`
	Version           uint64      `json:"version"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type JobSummary struct {
	ID          string     `json:"id"`
	Status      job.Status `json:"status"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   string     `json:"last_error,omitempty"`
	Version     uint64     `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ResultEvidence struct {
	Availability     EvidenceAvailability `json:"availability"`
	ActualStateCount int                  `json:"actual_state_count"`
	HealthCount      int                  `json:"health_count"`
	AuditEventCount  int                  `json:"audit_event_count"`
}

type Timeline struct {
	Completeness TimelineCompleteness `json:"completeness"`
	Items        []TimelineItem        `json:"items"`
}

type TimelineItem struct {
	Kind    string    `json:"kind"`
	State   string    `json:"state"`
	At      time.Time `json:"at"`
	Version uint64    `json:"version"`
}

// Build creates the 0.31 read model without inventing transition history.
// Only the latest durable Change and Job snapshots are represented in the
// timeline until an authoritative event-history source is wired separately.
// Sensitive Job fields such as input, lease token and idempotency key are never
// copied into the UI contract.
func Build(snapshot change.Snapshot, execution *job.Job, generatedAt time.Time) (View, error) {
	if generatedAt.IsZero() {
		return View{}, fmt.Errorf("%w: generated_at is required", ErrInvalidView)
	}
	if err := validateChange(snapshot); err != nil {
		return View{}, err
	}

	view := View{
		ContractVersion: ContractVersion,
		GeneratedAt:     generatedAt.UTC(),
		Change: ChangeSummary{
			ID:                snapshot.ID,
			Action:            snapshot.Action,
			Requester:         snapshot.Requester,
			RevisionID:        snapshot.RevisionID,
			Risk:              snapshot.Risk,
			State:             snapshot.State,
			ApprovalsRecorded: len(snapshot.Approvals),
			ApprovalsRequired: snapshot.Decision.Requirement.Minimum,
			Version:           snapshot.Version,
			UpdatedAt:         snapshot.UpdatedAt.UTC(),
		},
		ResultEvidence: ResultEvidence{Availability: EvidenceUnavailable},
		Timeline: Timeline{
			Completeness: TimelineSnapshotOnly,
			Items: []TimelineItem{{
				Kind: "change_snapshot", State: string(snapshot.State), At: snapshot.UpdatedAt.UTC(), Version: snapshot.Version,
			}},
		},
	}

	if execution == nil {
		if changeRequiresJob(snapshot.State) {
			return View{}, fmt.Errorf("%w: change state %q requires a linked job", ErrInvalidView, snapshot.State)
		}
		view.AttentionRequired = snapshot.State == change.StateRejected || snapshot.State == change.StateFailed
		return view, nil
	}
	if err := validateJob(snapshot, *execution); err != nil {
		return View{}, err
	}

	view.Job = &JobSummary{
		ID:          execution.ID,
		Status:      execution.Status,
		Attempt:     execution.Attempt,
		MaxAttempts: execution.MaxAttempts,
		LastError:   execution.LastError,
		Version:     execution.Version,
		CreatedAt:   execution.CreatedAt.UTC(),
		UpdatedAt:   execution.UpdatedAt.UTC(),
	}
	view.Timeline.Items = append(view.Timeline.Items, TimelineItem{
		Kind: "job_snapshot", State: string(execution.Status), At: execution.UpdatedAt.UTC(), Version: execution.Version,
	})
	sort.SliceStable(view.Timeline.Items, func(i, j int) bool {
		if view.Timeline.Items[i].At.Equal(view.Timeline.Items[j].At) {
			return view.Timeline.Items[i].Kind < view.Timeline.Items[j].Kind
		}
		return view.Timeline.Items[i].At.Before(view.Timeline.Items[j].At)
	})

	if execution.Output != nil {
		view.ResultEvidence.ActualStateCount = len(execution.Output.ActualStates)
		view.ResultEvidence.HealthCount = len(execution.Output.Health)
		view.ResultEvidence.AuditEventCount = len(execution.Output.AuditEvents)
		if view.ResultEvidence.ActualStateCount+view.ResultEvidence.HealthCount+view.ResultEvidence.AuditEventCount > 0 {
			view.ResultEvidence.Availability = EvidenceAvailable
		}
	}

	view.AttentionRequired = execution.Status == job.StatusFailed ||
		snapshot.State == change.StateFailed ||
		(execution.Status == job.StatusSucceeded && view.ResultEvidence.Availability != EvidenceAvailable)
	return view, nil
}

func validateChange(snapshot change.Snapshot) error {
	if strings.TrimSpace(snapshot.ID) == "" || strings.TrimSpace(snapshot.Action) == "" || strings.TrimSpace(snapshot.Requester) == "" || strings.TrimSpace(snapshot.RevisionID) == "" {
		return fmt.Errorf("%w: change identity, action, requester and revision are required", ErrInvalidView)
	}
	if snapshot.ID != strings.TrimSpace(snapshot.ID) || snapshot.Action != strings.TrimSpace(snapshot.Action) || snapshot.Requester != strings.TrimSpace(snapshot.Requester) || snapshot.RevisionID != strings.TrimSpace(snapshot.RevisionID) {
		return fmt.Errorf("%w: change identity fields must be canonical", ErrInvalidView)
	}
	if snapshot.Version == 0 || snapshot.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: change version and updated_at are required", ErrInvalidView)
	}
	if !snapshot.Risk.Valid() || snapshot.Decision.Risk != snapshot.Risk {
		return fmt.Errorf("%w: change risk evidence is inconsistent", ErrInvalidView)
	}
	if err := snapshot.Decision.Validate(); err != nil {
		return fmt.Errorf("%w: invalid policy decision: %v", ErrInvalidView, err)
	}
	switch snapshot.State {
	case change.StatePendingApproval, change.StateApproved, change.StateQueued, change.StateExecuting, change.StateVerifying, change.StateSucceeded, change.StateFailed, change.StateCancelled, change.StateRejected:
	default:
		return fmt.Errorf("%w: unsupported change state %q", ErrInvalidView, snapshot.State)
	}
	return nil
}

func validateJob(snapshot change.Snapshot, execution job.Job) error {
	if strings.TrimSpace(execution.ID) == "" || execution.ID != strings.TrimSpace(execution.ID) {
		return fmt.Errorf("%w: canonical job id is required", ErrInvalidView)
	}
	if execution.ChangeID != snapshot.ID || execution.ActionName != snapshot.Action {
		return fmt.Errorf("%w: job linkage does not match change identity", ErrInvalidView)
	}
	if execution.MaxAttempts <= 0 || execution.Attempt < 0 || execution.Attempt > execution.MaxAttempts {
		return fmt.Errorf("%w: invalid job attempt bounds", ErrInvalidView)
	}
	if execution.Version == 0 || execution.CreatedAt.IsZero() || execution.UpdatedAt.IsZero() || execution.UpdatedAt.Before(execution.CreatedAt) {
		return fmt.Errorf("%w: invalid job version or timestamps", ErrInvalidView)
	}
	switch execution.Status {
	case job.StatusQueued, job.StatusRunning, job.StatusRetryWait, job.StatusCancelRequested, job.StatusCancelled, job.StatusSucceeded, job.StatusFailed:
	default:
		return fmt.Errorf("%w: unsupported job status %q", ErrInvalidView, execution.Status)
	}
	if snapshot.State == change.StatePendingApproval || snapshot.State == change.StateRejected {
		return fmt.Errorf("%w: state %q cannot have an executable job", ErrInvalidView, snapshot.State)
	}
	return nil
}

func changeRequiresJob(state change.State) bool {
	switch state {
	case change.StateQueued, change.StateExecuting, change.StateVerifying, change.StateSucceeded:
		return true
	default:
		return false
	}
}
