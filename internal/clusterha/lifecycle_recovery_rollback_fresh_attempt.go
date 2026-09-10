package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecycleRecoveryRollbackFreshAttempt = errors.New(
		"invalid cluster lifecycle recovery rollback fresh attempt",
	)
	ErrStaleLifecycleRecoveryRollbackFreshAttempt = errors.New(
		"stale cluster lifecycle recovery rollback fresh attempt",
	)
)

const LifecycleRecoveryRollbackFreshAttemptSchemaV1 = "clusterha.lifecycle-recovery-rollback-fresh-attempt/v1"

// LifecycleRecoveryRollbackFreshAttemptRequest consumes one immutable fresh-
// admission gate and one final safety read immediately before a typed rollback
// CAS can be handed to the audited lifecycle executor. It carries no caller-
// selected command, role, membership or host-mutation material.
type LifecycleRecoveryRollbackFreshAttemptRequest struct {
	Gate              LifecycleRecoveryRollbackFreshAdmissionGate        `json:"gate"`
	GateObservation   LifecycleRecoveryRollbackFreshAdmissionObservation `json:"gate_observation"`
	CurrentCAS        LifecycleCASObservation                            `json:"current_cas"`
	CurrentMembership Snapshot                                           `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                         `json:"-"`
}

// LifecycleRecoveryRollbackFreshAttempt is a new single-use capability for one
// exact Draining -> Ready lifecycle CAS after the previous rollback admission
// was proven not applied. It never makes the previous admission reusable and
// grants no membership, failover, generic-command or direct host authority.
type LifecycleRecoveryRollbackFreshAttempt struct {
	SchemaVersion                    string                       `json:"schema_version"`
	FreshRollbackAttemptID           string                       `json:"fresh_rollback_attempt_id"`
	FreshAdmissionGateID             string                       `json:"fresh_admission_gate_id"`
	ReconciliationHandoffID          string                       `json:"reconciliation_handoff_id"`
	PreviousRollbackAdmissionID      string                       `json:"previous_rollback_admission_id"`
	RollbackPlanID                   string                       `json:"rollback_plan_id"`
	RecoveryHandoffID                string                       `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                       `json:"original_admission_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	QuorumRequired                   int                          `json:"quorum_required"`
	HealthyVotesObserved             int                          `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                          `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                          `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	RecoveryOnly                     bool                         `json:"recovery_only"`
	RollbackOnly                     bool                         `json:"rollback_only"`
	FreshAttemptAuthorized           bool                         `json:"fresh_attempt_authorized"`
	PreviousAdmissionReusable        bool                         `json:"previous_admission_reusable"`
	GenericRetryAuthorized           bool                         `json:"generic_retry_authorized"`
	CASBound                         bool                         `json:"cas_bound"`
	SingleUseByCAS                   bool                         `json:"single_use_by_cas"`
	AuditedChangeJobRequired         bool                         `json:"audited_change_job_required"`
	ExecutionAuthorized              bool                         `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                         `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                         `json:"failover_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackFreshAttempt performs a second exact safety
// read against the immutable gate. Only an unchanged lifecycle CAS, membership
// snapshot and HA revision can mint a new typed single-use attempt.
func BuildLifecycleRecoveryRollbackFreshAttempt(
	request LifecycleRecoveryRollbackFreshAttemptRequest,
) (LifecycleRecoveryRollbackFreshAttempt, error) {
	if err := RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(
		request.Gate,
		request.GateObservation,
	); err != nil {
		return LifecycleRecoveryRollbackFreshAttempt{}, classifyLifecycleRecoveryRollbackFreshAttemptError(err)
	}

	currentObservation := request.GateObservation
	currentObservation.CurrentCAS = request.CurrentCAS
	currentObservation.CurrentMembership = request.CurrentMembership
	currentObservation.CurrentEvidence = request.CurrentEvidence
	if err := RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(
		request.Gate,
		currentObservation,
	); err != nil {
		return LifecycleRecoveryRollbackFreshAttempt{}, classifyLifecycleRecoveryRollbackFreshAttemptError(err)
	}

	gate := request.Gate
	attempt := LifecycleRecoveryRollbackFreshAttempt{
		SchemaVersion:                    LifecycleRecoveryRollbackFreshAttemptSchemaV1,
		FreshAdmissionGateID:             gate.FreshAdmissionGateID,
		ReconciliationHandoffID:          gate.ReconciliationHandoffID,
		PreviousRollbackAdmissionID:      gate.PreviousRollbackAdmissionID,
		RollbackPlanID:                   gate.RollbackPlanID,
		RecoveryHandoffID:                gate.RecoveryHandoffID,
		OriginalAdmissionID:              gate.OriginalAdmissionID,
		NodeID:                           gate.NodeID,
		FromState:                        gate.FromState,
		TargetState:                      gate.TargetState,
		TransitionType:                   gate.TransitionType,
		ExpectedLifecycleGeneration:      gate.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       gate.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: gate.CurrentLifecycleResourceVersion,
		ClusterID:                        gate.ClusterID,
		ClusterGeneration:                gate.ClusterGeneration,
		MembershipSnapshotID:             gate.MembershipSnapshotID,
		RevisionEvidenceID:               gate.RevisionEvidenceID,
		Revision:                         gate.Revision,
		QuorumRequired:                   gate.QuorumRequired,
		HealthyVotesObserved:             gate.HealthyVotesObserved,
		MinimumReadyNodes:                gate.MinimumReadyNodes,
		ReadyNodesObserved:               gate.ReadyNodesObserved,
		StandaloneDowntimeBound:          gate.StandaloneDowntimeBound,
		RecoveryOnly:                     true,
		RollbackOnly:                     true,
		FreshAttemptAuthorized:           true,
		PreviousAdmissionReusable:        false,
		GenericRetryAuthorized:           false,
		CASBound:                         true,
		SingleUseByCAS:                   true,
		AuditedChangeJobRequired:         true,
		ExecutionAuthorized:              true,
		LifecycleStateMutationAuthorized: true,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackFreshAttemptID(attempt)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAttempt{}, err
	}
	attempt.FreshRollbackAttemptID = id
	return attempt, nil
}

// RevalidateLifecycleRecoveryRollbackFreshAttempt proves that a saved attempt
// still names the same gate and the same final safety read. Once the lifecycle
// CAS moves, revalidation fails stale instead of authorizing another retry.
func RevalidateLifecycleRecoveryRollbackFreshAttempt(
	attempt LifecycleRecoveryRollbackFreshAttempt,
	request LifecycleRecoveryRollbackFreshAttemptRequest,
) error {
	if err := validateLifecycleRecoveryRollbackFreshAttempt(attempt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackFreshAttempt(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, attempt) {
		return fmt.Errorf(
			"%w: immutable fresh rollback attempt changed",
			ErrStaleLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackFreshAttemptError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackReconciliation) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) ||
		errors.Is(err, ErrStaleLifecycleExecutionRecovery) ||
		errors.Is(err, ErrStaleLifecycleExecutionAdmission) ||
		errors.Is(err, ErrStaleLifecycleHandoff) ||
		errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackFreshAttempt, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackFreshAttempt, err)
}

func validateLifecycleRecoveryRollbackFreshAttempt(
	attempt LifecycleRecoveryRollbackFreshAttempt,
) error {
	if attempt.SchemaVersion != LifecycleRecoveryRollbackFreshAttemptSchemaV1 ||
		attempt.FreshRollbackAttemptID == "" || attempt.FreshAdmissionGateID == "" ||
		attempt.ReconciliationHandoffID == "" || attempt.PreviousRollbackAdmissionID == "" ||
		attempt.RollbackPlanID == "" || attempt.RecoveryHandoffID == "" ||
		attempt.OriginalAdmissionID == "" || attempt.NodeID == "" || attempt.ClusterID == "" ||
		attempt.ClusterGeneration == 0 || attempt.MembershipSnapshotID == "" ||
		attempt.RevisionEvidenceID == "" || attempt.ExpectedLifecycleGeneration == 0 ||
		attempt.PlannedLifecycleGeneration == 0 || attempt.ExpectedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: fresh rollback attempt identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	if attempt.FreshRollbackAttemptID == attempt.PreviousRollbackAdmissionID {
		return fmt.Errorf(
			"%w: fresh rollback attempt must not reuse the previous admission identity",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	if attempt.FromState != nodelifecycle.StateDraining ||
		attempt.TargetState != nodelifecycle.StateReady ||
		attempt.TransitionType != nodelifecycle.TransitionDesired ||
		attempt.PlannedLifecycleGeneration != attempt.ExpectedLifecycleGeneration+1 {
		return fmt.Errorf(
			"%w: fresh rollback lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	if attempt.QuorumRequired <= 0 || attempt.MinimumReadyNodes != attempt.QuorumRequired ||
		attempt.HealthyVotesObserved < attempt.QuorumRequired ||
		attempt.ReadyNodesObserved < attempt.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	if !attempt.RecoveryOnly || !attempt.RollbackOnly || !attempt.FreshAttemptAuthorized ||
		attempt.PreviousAdmissionReusable || attempt.GenericRetryAuthorized ||
		!attempt.CASBound || !attempt.SingleUseByCAS || !attempt.AuditedChangeJobRequired ||
		!attempt.ExecutionAuthorized || !attempt.LifecycleStateMutationAuthorized ||
		attempt.MembershipMutationAuthorized || attempt.FailoverAuthorized ||
		attempt.GenericCommandAuthorized || attempt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: fresh rollback attempt safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	if err := validateTransitionRevision(attempt.Revision); err != nil {
		return fmt.Errorf(
			"%w: revision: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
			err,
		)
	}
	for field, value := range map[string]string{
		"fresh_rollback_attempt_id":           attempt.FreshRollbackAttemptID,
		"fresh_admission_gate_id":             attempt.FreshAdmissionGateID,
		"reconciliation_handoff_id":           attempt.ReconciliationHandoffID,
		"previous_rollback_admission_id":      attempt.PreviousRollbackAdmissionID,
		"rollback_plan_id":                    attempt.RollbackPlanID,
		"recovery_handoff_id":                 attempt.RecoveryHandoffID,
		"original_admission_id":               attempt.OriginalAdmissionID,
		"node_id":                             attempt.NodeID,
		"cluster_id":                          attempt.ClusterID,
		"membership_snapshot_id":              attempt.MembershipSnapshotID,
		"revision_evidence_id":                attempt.RevisionEvidenceID,
		"expected_lifecycle_resource_version": attempt.ExpectedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackFreshAttemptID(attempt)
	if err != nil {
		return err
	}
	if expectedID != attempt.FreshRollbackAttemptID {
		return fmt.Errorf(
			"%w: fresh rollback attempt fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackFreshAttemptID(
	attempt LifecycleRecoveryRollbackFreshAttempt,
) (string, error) {
	input := attempt
	input.FreshRollbackAttemptID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttempt,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrfa-" + hex.EncodeToString(digest[:])[:24], nil
}
