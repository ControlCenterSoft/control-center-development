package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate = errors.New(
		"invalid cluster lifecycle recovery rollback fresh admission gate",
	)
	ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate = errors.New(
		"stale cluster lifecycle recovery rollback fresh admission gate",
	)
	ErrLifecycleRecoveryRollbackFreshAdmissionNotEligible = errors.New(
		"cluster lifecycle recovery rollback is not eligible for fresh admission",
	)
)

const LifecycleRecoveryRollbackFreshAdmissionGateSchemaV1 = "clusterha.lifecycle-recovery-rollback-fresh-admission-gate/v1"

// LifecycleRecoveryRollbackFreshAdmissionObservation consumes one exact
// not-applied reconciliation handoff and binds it to a second current lifecycle
// CAS and HA read. It carries no command material and grants no execution,
// membership, failover or host-mutation authority.
type LifecycleRecoveryRollbackFreshAdmissionObservation struct {
	ReconciliationHandoff     LifecycleRecoveryRollbackReconciliationHandoff     `json:"reconciliation_handoff"`
	ReconciliationObservation LifecycleRecoveryRollbackReconciliationObservation `json:"reconciliation_observation"`
	CurrentCAS                LifecycleCASObservation                            `json:"current_cas"`
	CurrentMembership         Snapshot                                           `json:"current_membership"`
	CurrentEvidence           TransitionRevisionEvidence                         `json:"-"`
}

// LifecycleRecoveryRollbackFreshAdmissionGate is evidence that the old
// single-use rollback admission was definitely not consumed and that the exact
// lifecycle/HA safety boundary still holds on a fresh read. The gate never
// re-authorizes the old admission; a later bounded contract must construct a
// new attempt lineage.
type LifecycleRecoveryRollbackFreshAdmissionGate struct {
	SchemaVersion                    string                       `json:"schema_version"`
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
	CurrentLifecycleResourceVersion  string                       `json:"current_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	QuorumRequired                   int                          `json:"quorum_required"`
	QuorumObserved                   int                          `json:"quorum_observed"`
	HealthyVotesObserved             int                          `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                          `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                          `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	Eligible                         bool                         `json:"eligible"`
	FreshAdmissionRequired           bool                         `json:"fresh_admission_required"`
	PreviousAdmissionReusable        bool                         `json:"previous_admission_reusable"`
	RetryAuthorized                  bool                         `json:"retry_authorized"`
	ExecutionAuthorized              bool                         `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                         `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                         `json:"failover_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackFreshAdmissionGate consumes only an exact
// not-applied reconciliation outcome. It re-reads and revalidates the lifecycle
// CAS, membership and HA journal boundary but deliberately returns evidence
// rather than an executable admission.
func BuildLifecycleRecoveryRollbackFreshAdmissionGate(
	observation LifecycleRecoveryRollbackFreshAdmissionObservation,
) (LifecycleRecoveryRollbackFreshAdmissionGate, error) {
	handoff := observation.ReconciliationHandoff
	if handoff.ReconciliationHandoffID == "" || handoff.RollbackAdmissionID == "" {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: reconciliation handoff is required",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if err := RevalidateLifecycleRecoveryRollbackReconciliationHandoff(
		handoff,
		observation.ReconciliationObservation,
	); err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{},
			classifyLifecycleRecoveryRollbackFreshAdmissionGateError(err)
	}
	if handoff.Outcome != LifecycleRecoveryRollbackNotApplied ||
		!handoff.SafetyBoundarySatisfied ||
		!handoff.FreshAdmissionRequired ||
		handoff.CompletionReceiptRequired ||
		handoff.ReconciliationRequired {
		return LifecycleRecoveryRollbackFreshAdmissionGate{},
			ErrLifecycleRecoveryRollbackFreshAdmissionNotEligible
	}
	if err := validateLifecycleCASObservation(observation.CurrentCAS); err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: current lifecycle CAS: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
			err,
		)
	}
	if !rollbackFreshAdmissionCASMatchesHandoff(observation.CurrentCAS, handoff) {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: lifecycle CAS moved after reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if err := revalidateRollbackFreshAdmissionHA(observation); err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, err
	}

	profile, members, err := validateSnapshot(observation.CurrentMembership)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: current membership: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
			err,
		)
	}
	member, exists := members[handoff.NodeID]
	if !exists || member.Kind != MemberController || !memberReady(member) {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: rollback target controller is not a current ready voter",
			ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	currentQuorum := quorum(len(observation.CurrentMembership.Members))
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	if currentQuorum != handoff.QuorumRequired ||
		currentHealthy < handoff.QuorumRequired ||
		currentReady < handoff.MinimumReadyNodes {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: quorum or minimum-ready boundary changed after reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if (profile == ProfileStandalone) != handoff.StandaloneDowntimeBound {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: standalone/HA recovery boundary changed after reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if err := validateTransitionRevisionEvidence(observation.CurrentEvidence); err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, fmt.Errorf(
			"%w: current HA revision evidence: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
			err,
		)
	}

	gate := LifecycleRecoveryRollbackFreshAdmissionGate{
		SchemaVersion:                    LifecycleRecoveryRollbackFreshAdmissionGateSchemaV1,
		ReconciliationHandoffID:          handoff.ReconciliationHandoffID,
		PreviousRollbackAdmissionID:      handoff.RollbackAdmissionID,
		RollbackPlanID:                   handoff.RollbackPlanID,
		RecoveryHandoffID:                handoff.RecoveryHandoffID,
		OriginalAdmissionID:              handoff.OriginalAdmissionID,
		NodeID:                           handoff.NodeID,
		FromState:                        handoff.FromState,
		TargetState:                      handoff.TargetState,
		TransitionType:                   handoff.TransitionType,
		ExpectedLifecycleGeneration:      handoff.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       handoff.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: handoff.ExpectedLifecycleResourceVersion,
		CurrentLifecycleResourceVersion:  observation.CurrentCAS.ResourceVersion,
		ClusterID:                        handoff.ClusterID,
		ClusterGeneration:                observation.CurrentEvidence.Generation(),
		MembershipSnapshotID:             observation.CurrentEvidence.MembershipSnapshotID(),
		RevisionEvidenceID:               observation.CurrentEvidence.EvidenceID(),
		Revision:                         observation.CurrentEvidence.Revision(),
		QuorumRequired:                   handoff.QuorumRequired,
		QuorumObserved:                   currentQuorum,
		HealthyVotesObserved:             currentHealthy,
		MinimumReadyNodes:                handoff.MinimumReadyNodes,
		ReadyNodesObserved:               currentReady,
		StandaloneDowntimeBound:          handoff.StandaloneDowntimeBound,
		Eligible:                         true,
		FreshAdmissionRequired:           true,
		PreviousAdmissionReusable:        false,
		RetryAuthorized:                  false,
		ExecutionAuthorized:              false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackFreshAdmissionGateID(gate)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAdmissionGate{}, err
	}
	gate.FreshAdmissionGateID = id
	return gate, nil
}

// RevalidateLifecycleRecoveryRollbackFreshAdmissionGate proves that a stored
// gate still matches the same reconciliation evidence and current reads. It
// cannot convert the previous admission into retry authority.
func RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(
	gate LifecycleRecoveryRollbackFreshAdmissionGate,
	observation LifecycleRecoveryRollbackFreshAdmissionObservation,
) error {
	if err := validateLifecycleRecoveryRollbackFreshAdmissionGate(gate); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if err != nil {
		return err
	}
	if rebuilt != gate {
		return fmt.Errorf(
			"%w: immutable fresh-admission gate evidence changed",
			ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	return nil
}

func rollbackFreshAdmissionCASMatchesHandoff(
	current LifecycleCASObservation,
	handoff LifecycleRecoveryRollbackReconciliationHandoff,
) bool {
	return current.NodeID == handoff.NodeID &&
		current.State == handoff.FromState &&
		current.Generation == handoff.ExpectedLifecycleGeneration &&
		current.ResourceVersion == handoff.ExpectedLifecycleResourceVersion
}

func revalidateRollbackFreshAdmissionHA(
	observation LifecycleRecoveryRollbackFreshAdmissionObservation,
) error {
	recoveryObservation := observation.ReconciliationObservation.Source.PlanRequest.RecoveryObservation
	recoveryObservation.CurrentMembership = observation.CurrentMembership
	recoveryObservation.CurrentEvidence = observation.CurrentEvidence
	if err := RevalidateLifecycleExecutionRecoveryHandoff(
		observation.ReconciliationObservation.Source.PlanRequest.RecoveryHandoff,
		recoveryObservation,
	); err != nil {
		return classifyLifecycleRecoveryRollbackFreshAdmissionGateError(err)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackFreshAdmissionGateError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackReconciliation) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) ||
		errors.Is(err, ErrStaleLifecycleExecutionRecovery) ||
		errors.Is(err, ErrStaleLifecycleExecutionAdmission) ||
		errors.Is(err, ErrStaleLifecycleHandoff) ||
		errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate, err)
}

func validateLifecycleRecoveryRollbackFreshAdmissionGate(
	gate LifecycleRecoveryRollbackFreshAdmissionGate,
) error {
	if gate.SchemaVersion != LifecycleRecoveryRollbackFreshAdmissionGateSchemaV1 ||
		gate.FreshAdmissionGateID == "" || gate.ReconciliationHandoffID == "" ||
		gate.PreviousRollbackAdmissionID == "" || gate.RollbackPlanID == "" ||
		gate.RecoveryHandoffID == "" || gate.OriginalAdmissionID == "" ||
		gate.NodeID == "" || gate.ClusterID == "" || gate.ClusterGeneration == 0 ||
		gate.MembershipSnapshotID == "" || gate.RevisionEvidenceID == "" ||
		gate.ExpectedLifecycleGeneration == 0 || gate.PlannedLifecycleGeneration == 0 ||
		gate.ExpectedLifecycleResourceVersion == "" || gate.CurrentLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: fresh-admission gate identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if gate.FromState != nodelifecycle.StateDraining ||
		gate.TargetState != nodelifecycle.StateReady ||
		gate.TransitionType != nodelifecycle.TransitionDesired ||
		gate.PlannedLifecycleGeneration != gate.ExpectedLifecycleGeneration+1 ||
		gate.CurrentLifecycleResourceVersion != gate.ExpectedLifecycleResourceVersion {
		return fmt.Errorf(
			"%w: fresh-admission lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if gate.QuorumRequired <= 0 || gate.QuorumObserved != gate.QuorumRequired ||
		gate.MinimumReadyNodes != gate.QuorumRequired ||
		gate.HealthyVotesObserved < gate.QuorumRequired ||
		gate.ReadyNodesObserved < gate.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: fresh-admission quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if !gate.Eligible || !gate.FreshAdmissionRequired || gate.PreviousAdmissionReusable ||
		gate.RetryAuthorized || gate.ExecutionAuthorized ||
		gate.LifecycleStateMutationAuthorized || gate.MembershipMutationAuthorized ||
		gate.FailoverAuthorized || gate.GenericCommandAuthorized || gate.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: fresh-admission safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	if err := validateTransitionRevision(gate.Revision); err != nil {
		return fmt.Errorf(
			"%w: revision: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
			err,
		)
	}
	for field, value := range map[string]string{
		"fresh_admission_gate_id":             gate.FreshAdmissionGateID,
		"reconciliation_handoff_id":           gate.ReconciliationHandoffID,
		"previous_rollback_admission_id":      gate.PreviousRollbackAdmissionID,
		"rollback_plan_id":                    gate.RollbackPlanID,
		"recovery_handoff_id":                 gate.RecoveryHandoffID,
		"original_admission_id":               gate.OriginalAdmissionID,
		"node_id":                             gate.NodeID,
		"cluster_id":                          gate.ClusterID,
		"membership_snapshot_id":              gate.MembershipSnapshotID,
		"revision_evidence_id":                gate.RevisionEvidenceID,
		"expected_lifecycle_resource_version": gate.ExpectedLifecycleResourceVersion,
		"current_lifecycle_resource_version":  gate.CurrentLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackFreshAdmissionGateID(gate)
	if err != nil {
		return err
	}
	if expectedID != gate.FreshAdmissionGateID {
		return fmt.Errorf(
			"%w: fresh-admission gate fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackFreshAdmissionGateID(
	gate LifecycleRecoveryRollbackFreshAdmissionGate,
) (string, error) {
	input := gate
	input.FreshAdmissionGateID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrg-" + hex.EncodeToString(digest[:])[:24], nil
}
