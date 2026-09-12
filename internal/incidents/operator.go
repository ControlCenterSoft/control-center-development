package incidents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"control-center/internal/corecontracts"
)

var (
	// ErrOperatorIdentityRequired means an authenticated server-side identity was
	// not supplied. Actor identity must never be accepted from the request body.
	ErrOperatorIdentityRequired = errors.New("incident operator identity is required")
	// ErrOperatorAccessDenied is returned by policy adapters when the actor lacks
	// the exact read/mutation capability for the requested scope. Object reads
	// deliberately collapse this error to ErrNotFound to avoid an authorization
	// oracle for incident identifiers.
	ErrOperatorAccessDenied = errors.New("incident operator access denied")
	// ErrOperatorStepUpRequired lets an authorization policy require fresh MFA or
	// another bounded re-authentication step without granting any authority itself.
	ErrOperatorStepUpRequired = errors.New("incident operator step-up required")
	// ErrOperatorDependencyUnavailable is fail-closed: a missing policy,
	// revision generator, or atomic commit boundary must never become permission.
	ErrOperatorDependencyUnavailable = errors.New("incident operator dependency unavailable")
)

// OperatorCapability is an application-level capability. The HTTP/runtime
// adapter maps these capabilities to persisted RBAC permissions and any
// purpose-bound step-up policy; this package intentionally does not invent a
// second RBAC store.
type OperatorCapability string

const (
	CapabilityIncidentRead           OperatorCapability = "incidents.read"
	CapabilityIncidentList           OperatorCapability = "incidents.list"
	CapabilityIncidentAcknowledge    OperatorCapability = "incidents.acknowledge"
	CapabilityIncidentResolve        OperatorCapability = "incidents.resolve"
	CapabilityIncidentEvidenceUpdate OperatorCapability = "incidents.evidence.write"
)

// OperatorAccessPolicy is the only authorization/step-up boundary consumed by
// OperatorService. Implementations must derive permissions from the
// authenticated actor and server-side scope. Returning nil authorizes exactly
// the supplied capability; every error fails closed.
type OperatorAccessPolicy interface {
	AuthorizeList(context.Context, string, OperatorCapability, ListQuery) error
	AuthorizeIncident(context.Context, string, OperatorCapability, Incident) error
}

// ResourceVersionGenerator creates an opaque persistence token for the next
// incident revision. Clients cannot choose or increment resource versions.
type ResourceVersionGenerator interface {
	NextIncidentResourceVersion(context.Context, Incident) (string, error)
}

// OperatorAuditRecord is intentionally metadata-only. Free-form notes,
// resolution text, evidence payloads and credentials are excluded from this
// record; immutable evidence may refer to the incident object by exact revision.
type OperatorAuditRecord struct {
	Action                OperatorCapability `json:"action"`
	ActorID               string             `json:"actor_id"`
	IncidentID            string             `json:"incident_id"`
	ScopeID               string             `json:"scope_id"`
	OccurredAt            time.Time          `json:"occurred_at"`
	BeforeStatus          Status             `json:"before_status"`
	AfterStatus           Status             `json:"after_status"`
	BeforeGeneration      uint64             `json:"before_generation"`
	AfterGeneration       uint64             `json:"after_generation"`
	BeforeResourceVersion string             `json:"before_resource_version"`
	AfterResourceVersion  string             `json:"after_resource_version"`
}

// AtomicMutationCommitter persists the prepared mutation and its immutable
// Audit evidence in one atomic durability boundary. A storage adapter must not
// make the incident update visible as successful if the Audit append fails.
type AtomicMutationCommitter interface {
	CommitIncidentMutation(context.Context, PreparedMutation, OperatorAuditRecord) error
}

// OperatorService is the authenticated application boundary for incident reads
// and operator mutations. It does not expose generic execution authority.
type OperatorService struct {
	reader    Reader
	policy    OperatorAccessPolicy
	versions  ResourceVersionGenerator
	committer AtomicMutationCommitter
}

func NewOperatorService(reader Reader, policy OperatorAccessPolicy, versions ResourceVersionGenerator, committer AtomicMutationCommitter) *OperatorService {
	return &OperatorService{reader: reader, policy: policy, versions: versions, committer: committer}
}

func (s *OperatorService) Get(ctx context.Context, actorID, incidentID string) (Incident, error) {
	if err := validateOperatorActor(actorID); err != nil {
		return Incident{}, err
	}
	if s == nil || s.reader == nil || s.policy == nil {
		return Incident{}, ErrOperatorDependencyUnavailable
	}
	if strings.TrimSpace(incidentID) == "" {
		return Incident{}, ErrNotFound
	}
	incident, err := s.reader.Get(ctx, incidentID)
	if err != nil {
		return Incident{}, err
	}
	if err := incident.Validate(); err != nil {
		return Incident{}, fmt.Errorf("%w: stored incident is invalid", ErrOperatorDependencyUnavailable)
	}
	if err := s.policy.AuthorizeIncident(ctx, actorID, CapabilityIncidentRead, incident); err != nil {
		if errors.Is(err, ErrOperatorAccessDenied) {
			return Incident{}, ErrNotFound
		}
		return Incident{}, err
	}
	return incident, nil
}

func (s *OperatorService) List(ctx context.Context, actorID string, query ListQuery) (ListPage, error) {
	if err := validateOperatorActor(actorID); err != nil {
		return ListPage{}, err
	}
	if s == nil || s.reader == nil || s.policy == nil {
		return ListPage{}, ErrOperatorDependencyUnavailable
	}
	normalized, err := NormalizeListQuery(query)
	if err != nil {
		return ListPage{}, err
	}
	// Policy is evaluated before storage so an unauthorized broad query cannot
	// learn hidden incident counts or pagination boundaries.
	if err := s.policy.AuthorizeList(ctx, actorID, CapabilityIncidentList, normalized); err != nil {
		return ListPage{}, err
	}
	page, err := s.reader.List(ctx, normalized)
	if err != nil {
		return ListPage{}, err
	}
	for idx := range page.Items {
		if err := page.Items[idx].Validate(); err != nil {
			return ListPage{}, fmt.Errorf("%w: stored incident page contains invalid item", ErrOperatorDependencyUnavailable)
		}
		if err := s.policy.AuthorizeIncident(ctx, actorID, CapabilityIncidentRead, page.Items[idx]); err != nil {
			// The list policy must select a storage scope that is fully visible to
			// the actor. Returning a partially filtered page would leak counts and
			// corrupt keyset pagination, so any mismatch fails the whole request.
			return ListPage{}, fmt.Errorf("%w: list scope returned an unauthorized incident", ErrOperatorDependencyUnavailable)
		}
	}
	return page, nil
}

type AcknowledgeCommand struct {
	Precondition corecontracts.ObjectPrecondition `json:"precondition"`
	OccurredAt   time.Time                        `json:"occurred_at"`
	Note         string                           `json:"note,omitempty"`
}

type ResolveCommand struct {
	Precondition corecontracts.ObjectPrecondition `json:"precondition"`
	OccurredAt   time.Time                        `json:"occurred_at"`
	Resolution   string                           `json:"resolution"`
	Evidence     []EvidenceRef                    `json:"evidence,omitempty"`
}

type EvidenceUpdateCommand struct {
	Precondition corecontracts.ObjectPrecondition `json:"precondition"`
	OccurredAt   time.Time                        `json:"occurred_at"`
	Runbook      *RunbookRef                      `json:"runbook,omitempty"`
	Evidence     []EvidenceRef                    `json:"evidence,omitempty"`
	Note         string                           `json:"note"`
}

func (s *OperatorService) Acknowledge(ctx context.Context, actorID, incidentID string, command AcknowledgeCommand) (Incident, error) {
	return s.commitMutation(ctx, actorID, incidentID, CapabilityIncidentAcknowledge, command.OccurredAt, func(current Incident, resourceVersion string) (PreparedMutation, error) {
		return PrepareAcknowledgement(current, AcknowledgeRequest{
			Precondition:    command.Precondition,
			ActorID:         actorID,
			OccurredAt:      command.OccurredAt,
			ResourceVersion: resourceVersion,
			Note:            command.Note,
		})
	})
}

func (s *OperatorService) Resolve(ctx context.Context, actorID, incidentID string, command ResolveCommand) (Incident, error) {
	return s.commitMutation(ctx, actorID, incidentID, CapabilityIncidentResolve, command.OccurredAt, func(current Incident, resourceVersion string) (PreparedMutation, error) {
		return PrepareResolution(current, ResolveRequest{
			Precondition:    command.Precondition,
			ActorID:         actorID,
			OccurredAt:      command.OccurredAt,
			ResourceVersion: resourceVersion,
			Resolution:      command.Resolution,
			Evidence:        cloneEvidence(command.Evidence),
		})
	})
}

func (s *OperatorService) UpdateEvidence(ctx context.Context, actorID, incidentID string, command EvidenceUpdateCommand) (Incident, error) {
	return s.commitMutation(ctx, actorID, incidentID, CapabilityIncidentEvidenceUpdate, command.OccurredAt, func(current Incident, resourceVersion string) (PreparedMutation, error) {
		return PrepareEvidenceUpdate(current, EvidenceUpdateRequest{
			Precondition:    command.Precondition,
			ActorID:         actorID,
			OccurredAt:      command.OccurredAt,
			ResourceVersion: resourceVersion,
			Runbook:         cloneRunbook(command.Runbook),
			Evidence:        cloneEvidence(command.Evidence),
			Note:            command.Note,
		})
	})
}

type prepareOperatorMutation func(Incident, string) (PreparedMutation, error)

func (s *OperatorService) commitMutation(ctx context.Context, actorID, incidentID string, capability OperatorCapability, occurredAt time.Time, prepare prepareOperatorMutation) (Incident, error) {
	if err := validateOperatorActor(actorID); err != nil {
		return Incident{}, err
	}
	if s == nil || s.reader == nil || s.policy == nil || s.versions == nil || s.committer == nil || prepare == nil {
		return Incident{}, ErrOperatorDependencyUnavailable
	}
	if strings.TrimSpace(incidentID) == "" {
		return Incident{}, ErrNotFound
	}
	current, err := s.reader.Get(ctx, incidentID)
	if err != nil {
		return Incident{}, err
	}
	if err := current.Validate(); err != nil {
		return Incident{}, fmt.Errorf("%w: stored incident is invalid", ErrOperatorDependencyUnavailable)
	}
	if current.ObjectID != incidentID {
		return Incident{}, fmt.Errorf("%w: repository returned a different incident", ErrOperatorDependencyUnavailable)
	}
	if err := s.policy.AuthorizeIncident(ctx, actorID, capability, current); err != nil {
		if errors.Is(err, ErrOperatorAccessDenied) {
			return Incident{}, ErrNotFound
		}
		return Incident{}, err
	}
	resourceVersion, err := s.versions.NextIncidentResourceVersion(ctx, current)
	if err != nil || strings.TrimSpace(resourceVersion) == "" || resourceVersion == current.ResourceVersion {
		return Incident{}, fmt.Errorf("%w: resource version generation failed", ErrOperatorDependencyUnavailable)
	}
	prepared, err := prepare(current, resourceVersion)
	if err != nil {
		return Incident{}, err
	}
	if prepared.Next.ObjectID != current.ObjectID || prepared.Next.ScopeID != current.ScopeID {
		return Incident{}, fmt.Errorf("%w: prepared mutation changed incident identity or scope", ErrOperatorDependencyUnavailable)
	}
	audit := OperatorAuditRecord{
		Action:                capability,
		ActorID:               actorID,
		IncidentID:            current.ObjectID,
		ScopeID:               current.ScopeID,
		OccurredAt:            occurredAt.UTC(),
		BeforeStatus:          current.Status,
		AfterStatus:           prepared.Next.Status,
		BeforeGeneration:      current.Generation,
		AfterGeneration:       prepared.Next.Generation,
		BeforeResourceVersion: current.ResourceVersion,
		AfterResourceVersion:  prepared.Next.ResourceVersion,
	}
	if err := validateOperatorAuditRecord(audit); err != nil {
		return Incident{}, fmt.Errorf("%w: invalid audit record: %v", ErrOperatorDependencyUnavailable, err)
	}
	if err := s.committer.CommitIncidentMutation(ctx, prepared, audit); err != nil {
		return Incident{}, err
	}
	return prepared.Next, nil
}

func validateOperatorActor(actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return ErrOperatorIdentityRequired
	}
	if err := validateRef("operator.actor_id", actorID); err != nil {
		return fmt.Errorf("%w: invalid actor identity", ErrOperatorIdentityRequired)
	}
	return nil
}

func validateOperatorAuditRecord(record OperatorAuditRecord) error {
	if record.Action != CapabilityIncidentAcknowledge && record.Action != CapabilityIncidentResolve && record.Action != CapabilityIncidentEvidenceUpdate {
		return errors.New("unsupported operator audit action")
	}
	if err := validateRef("audit.actor_id", record.ActorID); err != nil {
		return err
	}
	if err := validateRef("audit.incident_id", record.IncidentID); err != nil {
		return err
	}
	if err := validateRef("audit.scope_id", record.ScopeID); err != nil {
		return err
	}
	if record.OccurredAt.IsZero() || !record.BeforeStatus.Valid() || !record.AfterStatus.Valid() {
		return errors.New("audit timestamp and statuses are required")
	}
	if record.BeforeResourceVersion == "" || record.AfterResourceVersion == "" || record.BeforeResourceVersion == record.AfterResourceVersion {
		return errors.New("audit record requires distinct resource versions")
	}
	if record.AfterGeneration < record.BeforeGeneration {
		return errors.New("audit generation cannot move backwards")
	}
	return nil
}

func cloneRunbook(input *RunbookRef) *RunbookRef {
	if input == nil {
		return nil
	}
	copy := *input
	return &copy
}
