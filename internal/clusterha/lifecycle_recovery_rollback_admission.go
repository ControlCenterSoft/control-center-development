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
	ErrInvalidLifecycleRecoveryRollbackAdmission = errors.New("invalid cluster lifecycle recovery rollback admission")
	ErrStaleLifecycleRecoveryRollbackAdmission   = errors.New("stale cluster lifecycle recovery rollback admission")
)

const LifecycleRecoveryRollbackAdmissionSchemaV1 = "clusterha.lifecycle-recovery-rollback-admission/v1"

// LifecycleRecoveryRollbackAdmissionRequest binds the sealed rollback plan to
// a fresh recovery observation and the exact lifecycle CAS precondition that a
// later audited executor must consume atomically. It contains no host command,
// callback or caller-selected execution material.
type LifecycleRecoveryRollbackAdmissionRequest struct {
	Plan        LifecycleRecoveryRollbackPlan        `json:"plan"`
	PlanRequest LifecycleRecoveryRollbackPlanRequest `json:"plan_request"`
	CurrentCAS  LifecycleCASObservation              `json:"current_cas"`
}

// LifecycleRecoveryRollbackAdmission is a single-use-by-CAS capability for the
// exact typed Draining -> Ready recovery transition. It grants no membership,
// failover, generic-command or direct host-mutation authority.
type LifecycleRecoveryRollbackAdmission struct {
	SchemaVersion                    string                        `json:"schema_version"`
	RollbackAdmissionID              string                        `json:"rollback_admission_id"`
	RollbackPlanID                   string                        `json:"rollback_plan_id"`
	RecoveryHandoffID                string                        `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                        `json:"original_admission_id"`
	OriginalLifecyclePlanID          string                        `json:"original_lifecycle_plan_id"`
	NodeID                           string                        `json:"node_id"`
	FromState                        nodelifecycle.State           `json:"from_state"`
	TargetState                      nodelifecycle.State           `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType  `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                        `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                        `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                        `json:"expected_lifecycle_resource_version"`
	ClusterID                        string                        `json:"cluster_id"`
	ClusterGeneration                uint64                        `json:"cluster_generation"`
	MembershipSnapshotID             string                        `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                        `json:"revision_evidence_id"`
	Revision                         TransitionRevision            `json:"revision"`
	QuorumRequired                   int                           `json:"quorum_required"`
	HealthyVotesObserved             int                           `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                           `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                           `json:"ready_nodes_observed"`
	RequiredEvidence                 []nodelifecycle.EvidenceCheck `json:"required_evidence"`
	StandaloneDowntimeBound          bool                          `json:"standalone_downtime_bound"`
	RecoveryOnly                     bool                          `json:"recovery_only"`
	RollbackOnly                     bool                          `json:"rollback_only"`
	CASBound                         bool                          `json:"cas_bound"`
	SingleUseByCAS                   bool                          `json:"single_use_by_cas"`
	AuditedChangeJobRequired         bool                          `json:"audited_change_job_required"`
	ExecutionAuthorized              bool                          `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                          `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                          `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                          `json:"failover_authorized"`
	GenericCommandAuthorized         bool                          `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                          `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackAdmission issues a bounded recovery capability
// only after the recovery plan, membership/journal lineage and current lifecycle
// CAS have all been revalidated from fresh evidence.
func BuildLifecycleRecoveryRollbackAdmission(
	request LifecycleRecoveryRollbackAdmissionRequest,
) (LifecycleRecoveryRollbackAdmission, error) {
	if err := RevalidateLifecycleRecoveryRollbackPlan(request.Plan, request.PlanRequest); err != nil {
		return LifecycleRecoveryRollbackAdmission{}, classifyLifecycleRecoveryRollbackAdmissionError(err)
	}
	if err := validateRollbackAdmissionCAS(request.CurrentCAS); err != nil {
		return LifecycleRecoveryRollbackAdmission{}, err
	}
	plan := request.Plan
	if !rollbackAdmissionCASMatchesPlan(request.CurrentCAS, plan) {
		return LifecycleRecoveryRollbackAdmission{}, fmt.Errorf(
			"%w: lifecycle CAS moved or the rollback capability was already consumed",
			ErrStaleLifecycleRecoveryRollbackAdmission,
		)
	}
	if plan.CurrentHealthyVotes < plan.CurrentQuorum || plan.CurrentReadyVotes < plan.CurrentQuorum {
		return LifecycleRecoveryRollbackAdmission{}, fmt.Errorf(
			"%w: healthy quorum and minimum-ready constraints are no longer satisfied",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}

	admission := LifecycleRecoveryRollbackAdmission{
		SchemaVersion:                    LifecycleRecoveryRollbackAdmissionSchemaV1,
		RollbackPlanID:                   plan.RollbackPlanID,
		RecoveryHandoffID:                plan.RecoveryHandoffID,
		OriginalAdmissionID:              plan.AdmissionID,
		OriginalLifecyclePlanID:          plan.OriginalLifecyclePlanID,
		NodeID:                           plan.NodeID,
		FromState:                        plan.ObservedLifecycleState,
		TargetState:                      plan.RollbackTargetState,
		TransitionType:                   plan.RollbackTransitionType,
		ExpectedLifecycleGeneration:      plan.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       plan.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: plan.ExpectedLifecycleResourceVersion,
		ClusterID:                        plan.ClusterID,
		ClusterGeneration:                plan.ClusterGeneration,
		MembershipSnapshotID:             plan.MembershipSnapshotID,
		RevisionEvidenceID:               plan.RevisionEvidenceID,
		Revision:                         plan.Revision,
		QuorumRequired:                   plan.CurrentQuorum,
		HealthyVotesObserved:             plan.CurrentHealthyVotes,
		MinimumReadyNodes:                plan.CurrentQuorum,
		ReadyNodesObserved:               plan.CurrentReadyVotes,
		RequiredEvidence:                 append([]nodelifecycle.EvidenceCheck(nil), plan.RequiredEvidence...),
		StandaloneDowntimeBound:          plan.StandaloneDowntimeBound,
		RecoveryOnly:                     true,
		RollbackOnly:                     true,
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
	id, err := lifecycleRecoveryRollbackAdmissionID(admission)
	if err != nil {
		return LifecycleRecoveryRollbackAdmission{}, err
	}
	admission.RollbackAdmissionID = id
	return admission, nil
}

// RevalidateLifecycleRecoveryRollbackAdmission proves that the capability still
// names the exact recovery plan, HA revision and unconsumed lifecycle CAS. A
// successful result permits only that typed lifecycle CAS.
func RevalidateLifecycleRecoveryRollbackAdmission(
	admission LifecycleRecoveryRollbackAdmission,
	request LifecycleRecoveryRollbackAdmissionRequest,
) error {
	if err := validateLifecycleRecoveryRollbackAdmission(admission); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackAdmission(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, admission) {
		return fmt.Errorf(
			"%w: immutable rollback admission evidence changed",
			ErrStaleLifecycleRecoveryRollbackAdmission,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackAdmissionError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) ||
		errors.Is(err, ErrStaleLifecycleExecutionRecovery) ||
		errors.Is(err, ErrStaleLifecycleExecutionAdmission) ||
		errors.Is(err, ErrStaleLifecycleHandoff) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackAdmission, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackAdmission, err)
}

func validateRollbackAdmissionCAS(observation LifecycleCASObservation) error {
	if observation.NodeID == "" || observation.ResourceVersion == "" ||
		observation.Generation == 0 || !observation.State.Valid() {
		return fmt.Errorf(
			"%w: lifecycle CAS observation is incomplete or invalid",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	if err := validateIdentifier("rollback_cas_node_id", observation.NodeID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackAdmission, err)
	}
	if err := validateIdentifier("rollback_cas_resource_version", observation.ResourceVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackAdmission, err)
	}
	return nil
}

func rollbackAdmissionCASMatchesPlan(
	observation LifecycleCASObservation,
	plan LifecycleRecoveryRollbackPlan,
) bool {
	return observation.NodeID == plan.NodeID &&
		observation.State == plan.ObservedLifecycleState &&
		observation.Generation == plan.ExpectedLifecycleGeneration &&
		observation.ResourceVersion == plan.ExpectedLifecycleResourceVersion
}

func validateLifecycleRecoveryRollbackAdmission(admission LifecycleRecoveryRollbackAdmission) error {
	if admission.SchemaVersion != LifecycleRecoveryRollbackAdmissionSchemaV1 ||
		admission.RollbackAdmissionID == "" || admission.RollbackPlanID == "" ||
		admission.RecoveryHandoffID == "" || admission.OriginalAdmissionID == "" ||
		admission.OriginalLifecyclePlanID == "" || admission.NodeID == "" || admission.ClusterID == "" ||
		admission.ClusterGeneration == 0 || admission.MembershipSnapshotID == "" ||
		admission.RevisionEvidenceID == "" || admission.ExpectedLifecycleGeneration == 0 ||
		admission.PlannedLifecycleGeneration == 0 || admission.ExpectedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: rollback admission identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	if admission.FromState != nodelifecycle.StateDraining ||
		admission.TargetState != nodelifecycle.StateReady ||
		admission.TransitionType != nodelifecycle.TransitionDesired ||
		admission.PlannedLifecycleGeneration != admission.ExpectedLifecycleGeneration+1 {
		return fmt.Errorf(
			"%w: rollback lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	if admission.QuorumRequired <= 0 || admission.MinimumReadyNodes != admission.QuorumRequired ||
		admission.HealthyVotesObserved < admission.QuorumRequired ||
		admission.ReadyNodesObserved < admission.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	if !admission.RecoveryOnly || !admission.RollbackOnly || !admission.CASBound ||
		!admission.SingleUseByCAS || !admission.AuditedChangeJobRequired ||
		!admission.ExecutionAuthorized || !admission.LifecycleStateMutationAuthorized ||
		admission.MembershipMutationAuthorized || admission.FailoverAuthorized ||
		admission.GenericCommandAuthorized || admission.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: rollback admission safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	wantEvidence := []nodelifecycle.EvidenceCheck{
		nodelifecycle.CheckOperationCancelled,
		nodelifecycle.CheckSchedulingEnabled,
		nodelifecycle.CheckReadinessPassed,
	}
	if !reflect.DeepEqual(admission.RequiredEvidence, wantEvidence) {
		return fmt.Errorf(
			"%w: rollback required evidence changed",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	if err := validateTransitionRevision(admission.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleRecoveryRollbackAdmission, err)
	}
	for field, value := range map[string]string{
		"rollback_admission_id":               admission.RollbackAdmissionID,
		"rollback_plan_id":                    admission.RollbackPlanID,
		"recovery_handoff_id":                 admission.RecoveryHandoffID,
		"original_admission_id":               admission.OriginalAdmissionID,
		"original_lifecycle_plan_id":          admission.OriginalLifecyclePlanID,
		"node_id":                             admission.NodeID,
		"cluster_id":                          admission.ClusterID,
		"membership_snapshot_id":              admission.MembershipSnapshotID,
		"revision_evidence_id":                admission.RevisionEvidenceID,
		"expected_lifecycle_resource_version": admission.ExpectedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackAdmission, err)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackAdmissionID(admission)
	if err != nil {
		return err
	}
	if expectedID != admission.RollbackAdmissionID {
		return fmt.Errorf(
			"%w: rollback admission fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackAdmissionID(
	admission LifecycleRecoveryRollbackAdmission,
) (string, error) {
	input := admission
	input.RollbackAdmissionID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackAdmission,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrra-" + hex.EncodeToString(digest[:])[:24], nil
}
