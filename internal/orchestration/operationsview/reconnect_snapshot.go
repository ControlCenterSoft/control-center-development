package operationsview

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"control-center/internal/orchestration/job"
)

const (
	JobReconnectSnapshotContractVersion = "ui.operations-job-reconnect/v1"
	MaxReconnectTimelineEvents          = 200
)

var ErrInvalidJobReconnectSnapshot = errors.New("invalid job reconnect snapshot")

type JobReconnectState string

const (
	JobReconnectCurrent        JobReconnectState = "current"
	JobReconnectReloadRequired JobReconnectState = "reload_required"
	JobReconnectUnavailable    JobReconnectState = "unavailable"
)

// JobReconnectInput describes a read-only reconnect against one already
// persisted durable Job. AfterVersion is the last Job version observed by the
// client; zero means that no resume cursor is available.
type JobReconnectInput struct {
	Job            job.Job
	Timeline       []job.TimelineEntry
	TimelineLoaded bool
	AfterVersion   uint64
	EvaluatedAt    time.Time
}

// JobReconnectSnapshot is a bounded resume response for an operational UI.
// It deliberately excludes raw Job input/output, lease tokens, worker IDs,
// idempotency keys and failure strings. It never grants retry/cancel/execution
// authority.
type JobReconnectSnapshot struct {
	ContractVersion           string              `json:"contract_version"`
	State                     JobReconnectState   `json:"state"`
	SourceAvailable           bool                `json:"source_available"`
	JobID                     string              `json:"job_id"`
	JobVersion                uint64              `json:"job_version"`
	Status                    job.Status          `json:"status"`
	Attempt                   int                 `json:"attempt"`
	MaxAttempts               int                 `json:"max_attempts"`
	UpdatedAt                 time.Time           `json:"updated_at"`
	Terminal                  bool                `json:"terminal"`
	AfterVersion              uint64              `json:"after_version"`
	ResumeVersion             uint64              `json:"resume_version"`
	TimelineHeadVersion       uint64              `json:"timeline_head_version,omitempty"`
	TimelineHeadStatus        job.Status          `json:"timeline_head_status,omitempty"`
	TimelineHeadAttempt       int                 `json:"timeline_head_attempt,omitempty"`
	Events                    []job.TimelineEntry `json:"events"`
	ReloadRequired            bool                `json:"reload_required"`
	EvaluatedAt               time.Time           `json:"evaluated_at"`
	ExecutionAuthorized       bool                `json:"execution_authorized"`
	ProductionMutationAllowed bool                `json:"production_mutation_allowed"`
}

func BuildJobReconnectSnapshot(input JobReconnectInput) (JobReconnectSnapshot, error) {
	if err := validateReconnectJob(input.Job); err != nil {
		return JobReconnectSnapshot{}, err
	}
	if input.EvaluatedAt.IsZero() {
		return JobReconnectSnapshot{}, invalidReconnect("evaluated_at is required")
	}
	now := input.EvaluatedAt.UTC()
	if input.Job.UpdatedAt.After(now) {
		return JobReconnectSnapshot{}, invalidReconnect("job evidence is future-dated")
	}

	base := JobReconnectSnapshot{
		ContractVersion:           JobReconnectSnapshotContractVersion,
		State:                     JobReconnectUnavailable,
		SourceAvailable:           input.TimelineLoaded,
		JobID:                     input.Job.ID,
		JobVersion:                input.Job.Version,
		Status:                    input.Job.Status,
		Attempt:                   input.Job.Attempt,
		MaxAttempts:               input.Job.MaxAttempts,
		UpdatedAt:                 input.Job.UpdatedAt.UTC(),
		Terminal:                  input.Job.Status.Terminal(),
		AfterVersion:              input.AfterVersion,
		ResumeVersion:             input.Job.Version,
		Events:                    []job.TimelineEntry{},
		ReloadRequired:            !input.TimelineLoaded,
		EvaluatedAt:               now,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	if !input.TimelineLoaded {
		if len(input.Timeline) != 0 {
			return JobReconnectSnapshot{}, invalidReconnect("timeline entries supplied while source is unavailable")
		}
		return base, nil
	}

	timeline, err := validatedReconnectTimeline(input.Job, input.Timeline, now)
	if err != nil {
		return JobReconnectSnapshot{}, err
	}
	head := timeline[len(timeline)-1]
	base.TimelineHeadVersion = head.JobVersion
	base.TimelineHeadStatus = head.Status
	base.TimelineHeadAttempt = head.Attempt

	if input.AfterVersion > input.Job.Version {
		base.State = JobReconnectReloadRequired
		base.ReloadRequired = true
		return base, nil
	}

	delta := make([]job.TimelineEntry, 0)
	for _, entry := range timeline {
		if entry.JobVersion > input.AfterVersion {
			delta = append(delta, entry)
		}
	}
	if len(delta) > MaxReconnectTimelineEvents {
		base.State = JobReconnectReloadRequired
		base.ReloadRequired = true
		return base, nil
	}
	base.State = JobReconnectCurrent
	base.Events = delta
	return base, nil
}

// ValidateJobReconnectSnapshot validates a stored or API-facing snapshot. It
// checks only self-contained invariants; authoritative Job/timeline freshness
// must still be established by BuildJobReconnectSnapshot at read time.
func ValidateJobReconnectSnapshot(snapshot JobReconnectSnapshot) error {
	if snapshot.ContractVersion != JobReconnectSnapshotContractVersion {
		return invalidReconnect("contract_version is invalid")
	}
	if snapshot.ExecutionAuthorized || snapshot.ProductionMutationAllowed {
		return invalidReconnect("reconnect snapshot must not grant mutation authority")
	}
	if strings.TrimSpace(snapshot.JobID) == "" || snapshot.JobID != strings.TrimSpace(snapshot.JobID) {
		return invalidReconnect("job_id must be canonical")
	}
	if snapshot.JobVersion == 0 || snapshot.ResumeVersion != snapshot.JobVersion || snapshot.MaxAttempts < 1 || snapshot.Attempt < 0 || snapshot.Attempt > snapshot.MaxAttempts {
		return invalidReconnect("job version or attempt bounds are invalid")
	}
	if !validReconnectStatus(snapshot.Status) || snapshot.Terminal != snapshot.Status.Terminal() {
		return invalidReconnect("job status evidence is invalid")
	}
	if snapshot.UpdatedAt.IsZero() || snapshot.EvaluatedAt.IsZero() || snapshot.UpdatedAt.After(snapshot.EvaluatedAt) {
		return invalidReconnect("snapshot timestamps are invalid")
	}
	if snapshot.Events == nil {
		return invalidReconnect("events must be a non-nil bounded collection")
	}

	switch snapshot.State {
	case JobReconnectUnavailable:
		if snapshot.SourceAvailable || !snapshot.ReloadRequired || len(snapshot.Events) != 0 || snapshot.TimelineHeadVersion != 0 || snapshot.TimelineHeadStatus != "" || snapshot.TimelineHeadAttempt != 0 {
			return invalidReconnect("unavailable snapshot carries timeline evidence")
		}
		return nil
	case JobReconnectReloadRequired:
		if !snapshot.SourceAvailable || !snapshot.ReloadRequired || len(snapshot.Events) != 0 {
			return invalidReconnect("reload_required snapshot is inconsistent")
		}
	case JobReconnectCurrent:
		if !snapshot.SourceAvailable || snapshot.ReloadRequired || snapshot.AfterVersion > snapshot.JobVersion {
			return invalidReconnect("current snapshot cursor/source state is inconsistent")
		}
	default:
		return invalidReconnect("unsupported reconnect state %q", snapshot.State)
	}

	if snapshot.TimelineHeadVersion == 0 || !validReconnectStatus(snapshot.TimelineHeadStatus) || snapshot.TimelineHeadAttempt < 0 || snapshot.TimelineHeadAttempt > snapshot.MaxAttempts {
		return invalidReconnect("timeline head evidence is invalid")
	}
	if snapshot.TimelineHeadVersion > snapshot.JobVersion || snapshot.TimelineHeadStatus != snapshot.Status || snapshot.TimelineHeadAttempt != snapshot.Attempt {
		return invalidReconnect("timeline head does not match current job lifecycle state")
	}

	previousVersion := snapshot.AfterVersion
	for _, entry := range snapshot.Events {
		if err := job.ValidateTimelineEntry(entry); err != nil {
			return invalidReconnect("invalid timeline entry: %v", err)
		}
		if entry.JobID != snapshot.JobID || entry.JobVersion <= previousVersion || entry.JobVersion > snapshot.JobVersion || entry.OccurredAt.After(snapshot.EvaluatedAt) {
			return invalidReconnect("timeline delta is not a monotonic exact-job continuation")
		}
		previousVersion = entry.JobVersion
	}
	if len(snapshot.Events) > MaxReconnectTimelineEvents {
		return invalidReconnect("timeline delta exceeds bounded reconnect limit")
	}
	return nil
}

func validatedReconnectTimeline(current job.Job, source []job.TimelineEntry, now time.Time) ([]job.TimelineEntry, error) {
	if len(source) == 0 {
		return nil, invalidReconnect("loaded timeline must contain lifecycle evidence")
	}
	copy := append([]job.TimelineEntry(nil), source...)
	sort.Slice(copy, func(i, j int) bool {
		if copy[i].JobVersion != copy[j].JobVersion {
			return copy[i].JobVersion < copy[j].JobVersion
		}
		return copy[i].OccurredAt.Before(copy[j].OccurredAt)
	})
	var previousVersion uint64
	var previousTime time.Time
	for index, entry := range copy {
		if err := job.ValidateTimelineEntry(entry); err != nil {
			return nil, invalidReconnect("timeline[%d] is invalid: %v", index, err)
		}
		if entry.JobID != current.ID {
			return nil, invalidReconnect("timeline[%d] belongs to a different job", index)
		}
		if entry.JobVersion > current.Version || entry.OccurredAt.After(current.UpdatedAt) || entry.OccurredAt.After(now) {
			return nil, invalidReconnect("timeline[%d] is newer than authoritative job evidence", index)
		}
		if previousVersion != 0 && entry.JobVersion <= previousVersion {
			return nil, invalidReconnect("timeline contains duplicate or non-monotonic job versions")
		}
		if !previousTime.IsZero() && entry.OccurredAt.Before(previousTime) {
			return nil, invalidReconnect("timeline timestamps move backwards")
		}
		previousVersion = entry.JobVersion
		previousTime = entry.OccurredAt
	}
	head := copy[len(copy)-1]
	if head.Status != current.Status || head.Attempt != current.Attempt {
		return nil, invalidReconnect("timeline head does not match authoritative job lifecycle state")
	}
	return copy, nil
}

func validateReconnectJob(current job.Job) error {
	if strings.TrimSpace(current.ID) == "" || current.ID != strings.TrimSpace(current.ID) ||
		strings.TrimSpace(current.ChangeID) == "" || current.ChangeID != strings.TrimSpace(current.ChangeID) ||
		strings.TrimSpace(current.ActionName) == "" || current.ActionName != strings.TrimSpace(current.ActionName) {
		return invalidReconnect("canonical job identity fields are required")
	}
	if !validReconnectStatus(current.Status) || current.Version == 0 || current.MaxAttempts < 1 || current.Attempt < 0 || current.Attempt > current.MaxAttempts {
		return invalidReconnect("job execution state is invalid")
	}
	if current.CreatedAt.IsZero() || current.UpdatedAt.IsZero() || current.UpdatedAt.Before(current.CreatedAt) {
		return invalidReconnect("job timestamps are invalid")
	}
	return nil
}

func validReconnectStatus(status job.Status) bool {
	switch status {
	case job.StatusQueued,
		job.StatusRunning,
		job.StatusRetryWait,
		job.StatusCancelRequested,
		job.StatusCancelled,
		job.StatusSucceeded,
		job.StatusFailed:
		return true
	default:
		return false
	}
}

func invalidReconnect(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidJobReconnectSnapshot, fmt.Sprintf(format, values...))
}
