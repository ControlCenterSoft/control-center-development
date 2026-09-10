package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecycleRecoveryRollbackPlan = errors.New("invalid cluster lifecycle recovery rollback plan")
	ErrStaleLifecycleRecoveryRollbackPlan   = errors.New("stale cluster lifecycle recovery rollback plan")
	ErrUnsafeLifecycleRecoveryRollbackPlan  = errors.New("unsafe cluster lifecycle recovery rollback plan")
	ErrLifecycleRecoveryRollbackNotRequired = errors.New("cluster lifecycle recovery rollback is not required")
)

const LifecycleRecoveryRollbackPlanSchemaV1 = "clusterha.lifecycle-recovery-rollback-plan/v1"

// LifecycleRecoveryRollbackPlanRequest binds a previously sealed recovery
// handoff to the exact, freshly re-read recovery observation. No caller-selected
// command or host operation can be supplied through this contract.
type LifecycleRecoveryRollbackPlanRequest struct {
	RecoveryHandoff     LifecycleExecutionRecoveryHandoff     `json:"recovery_handoff"`
	RecoveryObservation LifecycleExecutionRecoveryObservation `json:"recovery_observation"`
}

// LifecycleRecoveryRollbackPlan is proof-only admission for planning the exact
// inverse of a partially applied Ready -> Draining lifecycle CAS. It does not
// authorize the rollback CAS itself; a later audited executor must obtain a
// fresh, single-use execution capability after revalidation.
type LifecycleRecoveryRollbackPlan struct {
	SchemaVersion                    string                         `json:"schema_version"`
	RollbackPlanID                   string                         `json:"rollback_plan_id"`
	RecoveryHandoffID                string                         `json:"recovery_handoff_id"`
	AdmissionID                      string                         `json:"admission_id"`
	LifecycleHandoffID               string                         `json:"lifecycle_handoff_id"`
	OriginalLifecyclePlanID          string                         `json:"original_lifecycle_plan_id"`
	NodeID                           string                         `json:"node_id"`
	ObservedLifecycleState           nodelifecycle.State            `json:"observed_lifecycle_state"`
	RollbackTargetState              nodelifecycle.State            `json:"rollback_target_state"`
	RollbackTransitionType           nodelifecycle.TransitionType   `json:"rollback_transition_type"`
	ExpectedLifecycleGeneration      uint64                         `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                         `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                         `json:"expected_lifecycle_resource_version"`
	ClusterID                        string                         `json:"cluster_id"`
	ClusterGeneration                uint64                         `json:"cluster_generation"`
	MembershipSnapshotID             string                         `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                         `json:"revision_evidence_id"`
	Revision                         TransitionRevision             `json:"revision"`
	CurrentQuorum                    int                            `json:"current_quorum"`
	CurrentHealthyVotes              int                            `json:"current_healthy_votes"`
	CurrentReadyVotes                int                            `json:"current_ready_votes"`
	RequiredEvidence                 []nodelifecycle.EvidenceCheck `json:"required_evidence"`
	StandaloneDowntimeBound          bool                           `json:"standalone_downtime_bound"`
	Accepted                         bool                           `json:"accepted"`
	PlanOnly                         bool                           `json:"plan_only"`
	AuditedChangeJobRequired         bool                           `json:"audited_change_job_required"`
	ExecutionAuthorized              bool                           `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                           `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                           `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                           `json:"failover_authorized"`
	GenericCommandAuthorized         bool                           `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                           `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackPlan seals a bounded rollback-planning proof.
// This first slice intentionally supports only the partially applied
// Ready -> Draining desired transition because its inverse is an explicit,
// typed Draining -> Ready desired transition in the node lifecycle contract.
func BuildLifecycleRecoveryRollbackPlan(
	request LifecycleRecoveryRollbackPlanRequest,
) (LifecycleRecoveryRollbackPlan, error) {
	if err := RevalidateLifecycleExecutionRecoveryHandoff(
		request.RecoveryHandoff,
		request.RecoveryObservation,
	); err != nil {
		return LifecycleRecoveryRollbackPlan{}, classifyLifecycleRecoveryRollbackError(err)
	}
	handoff := request.RecoveryHandoff
	if !handoff.RollbackPlanningRequired {
		return LifecycleRecoveryRollbackPlan{}, ErrLifecycleRecoveryRollbackNotRequired
	}
	if handoff.FromState != nodelifecycle.StateReady ||
		handoff.TargetState != nodelifecycle.StateDraining ||
		handoff.TransitionType != nodelifecycle.TransitionDesired {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: only partially applied ready-to-draining desired transitions are supported",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if handoff.ObservedLifecycleState != nodelifecycle.StateDraining ||
		handoff.ObservedLifecycleGeneration != handoff.ExpectedLifecycleGeneration ||
		handoff.ObservedLifecycleResourceVersion == handoff.ExpectedLifecycleResourceVersion {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: recovery evidence is not a bounded partial drain CAS",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if handoff.ObservedLifecycleGeneration == math.MaxUint64 {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: lifecycle generation cannot advance",
			ErrUnsafeLifecycleRecoveryRollbackPlan,
		)
	}

	membership := request.RecoveryObservation.CurrentMembership
	profile, members, err := validateSnapshot(membership)
	if err != nil {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: current membership: %v",
			ErrUnsafeLifecycleRecoveryRollbackPlan,
			err,
		)
	}
	member, exists := members[handoff.NodeID]
	if !exists || member.Kind != MemberController || !memberReady(member) {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: rollback target controller is not a current ready voter",
			ErrUnsafeLifecycleRecoveryRollbackPlan,
		)
	}
	currentQuorum := quorum(len(membership.Members))
	currentHealthy := healthyVotes(membership.Members)
	currentReady := readyVotes(membership.Members)
	if currentHealthy < currentQuorum || currentReady < currentQuorum {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: rollback planning requires healthy and ready quorum",
			ErrUnsafeLifecycleRecoveryRollbackPlan,
		)
	}
	if profile == ProfileStandalone && !handoff.StandaloneDowntimeBound {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: standalone recovery lost downtime acknowledgement",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if profile != ProfileStandalone && handoff.StandaloneDowntimeBound {
		return LifecycleRecoveryRollbackPlan{}, fmt.Errorf(
			"%w: HA recovery cannot carry standalone downtime evidence",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}

	plan := LifecycleRecoveryRollbackPlan{
		SchemaVersion:                    LifecycleRecoveryRollbackPlanSchemaV1,
		RecoveryHandoffID:                handoff.RecoveryHandoffID,
		AdmissionID:                      handoff.AdmissionID,
		LifecycleHandoffID:               handoff.LifecycleHandoffID,
		OriginalLifecyclePlanID:          handoff.LifecyclePlanID,
		NodeID:                           handoff.NodeID,
		ObservedLifecycleState:           handoff.ObservedLifecycleState,
		RollbackTargetState:              handoff.FromState,
		RollbackTransitionType:           nodelifecycle.TransitionDesired,
		ExpectedLifecycleGeneration:      handoff.ObservedLifecycleGeneration,
		PlannedLifecycleGeneration:       handoff.ObservedLifecycleGeneration + 1,
		ExpectedLifecycleResourceVersion: handoff.ObservedLifecycleResourceVersion,
		ClusterID:                        handoff.ClusterID,
		ClusterGeneration:                handoff.ClusterGeneration,
		MembershipSnapshotID:             handoff.MembershipSnapshotID,
		RevisionEvidenceID:               handoff.RevisionEvidenceID,
		Revision:                         handoff.Revision,
		CurrentQuorum:                    currentQuorum,
		CurrentHealthyVotes:              currentHealthy,
		CurrentReadyVotes:                currentReady,
		RequiredEvidence: []nodelifecycle.EvidenceCheck{
			nodelifecycle.CheckOperationCancelled,
			nodelifecycle.CheckSchedulingEnabled,
			nodelifecycle.CheckReadinessPassed,
		},
		StandaloneDowntimeBound:          handoff.StandaloneDowntimeBound,
		Accepted:                         true,
		PlanOnly:                         true,
		AuditedChangeJobRequired:         true,
		ExecutionAuthorized:              false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	planID, err := lifecycleRecoveryRollbackPlanID(plan)
	if err != nil {
		return LifecycleRecoveryRollbackPlan{}, err
	}
	plan.RollbackPlanID = planID
	return plan, nil
}

// RevalidateLifecycleRecoveryRollbackPlan proves that neither recovery,
// lifecycle CAS nor HA membership/journal evidence moved after planning.
func RevalidateLifecycleRecoveryRollbackPlan(
	plan LifecycleRecoveryRollbackPlan,
	request LifecycleRecoveryRollbackPlanRequest,
) error {
	if err := validateLifecycleRecoveryRollbackPlan(plan); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackPlan(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, plan) {
		return fmt.Errorf(
			"%w: immutable rollback planning evidence changed",
			ErrStaleLifecycleRecoveryRollbackPlan,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackError(err error) error {
	switch {
	case errors.Is(err, ErrStaleLifecycleExecutionRecovery),
		errors.Is(err, ErrStaleLifecycleExecutionAdmission),
		errors.Is(err, ErrStaleLifecycleHandoff):
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackPlan, err)
	default:
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackPlan, err)
	}
}

func validateLifecycleRecoveryRollbackPlan(plan LifecycleRecoveryRollbackPlan) error {
	if plan.SchemaVersion != LifecycleRecoveryRollbackPlanSchemaV1 || plan.RollbackPlanID == "" ||
		plan.RecoveryHandoffID == "" || plan.AdmissionID == "" || plan.LifecycleHandoffID == "" ||
		plan.OriginalLifecyclePlanID == "" || plan.NodeID == "" || plan.ClusterID == "" ||
		plan.ClusterGeneration == 0 || plan.MembershipSnapshotID == "" || plan.RevisionEvidenceID == "" ||
		plan.ExpectedLifecycleGeneration == 0 || plan.PlannedLifecycleGeneration == 0 ||
		plan.ExpectedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: rollback plan identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if plan.ObservedLifecycleState != nodelifecycle.StateDraining ||
		plan.RollbackTargetState != nodelifecycle.StateReady ||
		plan.RollbackTransitionType != nodelifecycle.TransitionDesired ||
		plan.PlannedLifecycleGeneration != plan.ExpectedLifecycleGeneration+1 {
		return fmt.Errorf(
			"%w: rollback lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if plan.CurrentQuorum <= 0 || plan.CurrentHealthyVotes < plan.CurrentQuorum ||
		plan.CurrentReadyVotes < plan.CurrentQuorum {
		return fmt.Errorf(
			"%w: rollback quorum evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if !plan.Accepted || !plan.PlanOnly || !plan.AuditedChangeJobRequired || plan.ExecutionAuthorized ||
		plan.LifecycleStateMutationAuthorized || plan.MembershipMutationAuthorized || plan.FailoverAuthorized ||
		plan.GenericCommandAuthorized || plan.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: rollback safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	wantEvidence := []nodelifecycle.EvidenceCheck{
		nodelifecycle.CheckOperationCancelled,
		nodelifecycle.CheckSchedulingEnabled,
		nodelifecycle.CheckReadinessPassed,
	}
	if !reflect.DeepEqual(plan.RequiredEvidence, wantEvidence) {
		return fmt.Errorf(
			"%w: rollback required evidence changed",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	if err := validateTransitionRevision(plan.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleRecoveryRollbackPlan, err)
	}
	for field, value := range map[string]string{
		"rollback_plan_id":                    plan.RollbackPlanID,
		"recovery_handoff_id":                 plan.RecoveryHandoffID,
		"admission_id":                        plan.AdmissionID,
		"lifecycle_handoff_id":                plan.LifecycleHandoffID,
		"original_lifecycle_plan_id":          plan.OriginalLifecyclePlanID,
		"node_id":                             plan.NodeID,
		"cluster_id":                          plan.ClusterID,
		"membership_snapshot_id":              plan.MembershipSnapshotID,
		"revision_evidence_id":                plan.RevisionEvidenceID,
		"expected_lifecycle_resource_version": plan.ExpectedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackPlan, err)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackPlanID(plan)
	if err != nil {
		return err
	}
	if expectedID != plan.RollbackPlanID {
		return fmt.Errorf(
			"%w: rollback plan fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackPlan,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackPlanID(plan LifecycleRecoveryRollbackPlan) (string, error) {
	input := plan
	input.RollbackPlanID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackPlan,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrp-" + hex.EncodeToString(digest[:])[:24], nil
}
