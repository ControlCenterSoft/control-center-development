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
	ErrInvalidLifecycleRecoveryRollbackReconciliation = errors.New("invalid cluster lifecycle recovery rollback reconciliation")
	ErrStaleLifecycleRecoveryRollbackReconciliation   = errors.New("stale cluster lifecycle recovery rollback reconciliation")
)

const LifecycleRecoveryRollbackReconciliationHandoffSchemaV1 = "clusterha.lifecycle-recovery-rollback-reconciliation-handoff/v1"

type LifecycleRecoveryRollbackOutcome string

const (
	LifecycleRecoveryRollbackApplied    LifecycleRecoveryRollbackOutcome = "applied"
	LifecycleRecoveryRollbackNotApplied LifecycleRecoveryRollbackOutcome = "not_applied"
	LifecycleRecoveryRollbackAmbiguous  LifecycleRecoveryRollbackOutcome = "ambiguous"
	LifecycleRecoveryRollbackSuperseded LifecycleRecoveryRollbackOutcome = "superseded"
)

// LifecycleRecoveryRollbackReconciliationObservation binds an immutable rollback
// admission to a fresh read of the lifecycle CAS and HA evidence after an
// execution result was lost or could not be proven. It never carries command
// material or grants authority to repeat the rollback.
type LifecycleRecoveryRollbackReconciliationObservation struct {
	Admission         LifecycleRecoveryRollbackAdmission        `json:"admission"`
	Source            LifecycleRecoveryRollbackAdmissionRequest `json:"source"`
	ObservedCAS       LifecycleCASObservation                   `json:"observed_cas"`
	CurrentMembership Snapshot                                  `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                `json:"-"`
}

// LifecycleRecoveryRollbackReconciliationHandoff classifies the exact current
// lifecycle state after an uncertain rollback attempt. A definitely-not-applied
// result may only request a completely fresh admission after revalidation; an
// applied result may only proceed to completion-receipt construction. Ambiguous,
// superseded or degraded safety evidence always remains reconciliation-only.
type LifecycleRecoveryRollbackReconciliationHandoff struct {
	SchemaVersion                    string                           `json:"schema_version"`
	ReconciliationHandoffID          string                           `json:"reconciliation_handoff_id"`
	RollbackAdmissionID              string                           `json:"rollback_admission_id"`
	RollbackPlanID                   string                           `json:"rollback_plan_id"`
	RecoveryHandoffID                string                           `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                           `json:"original_admission_id"`
	NodeID                           string                           `json:"node_id"`
	FromState                        nodelifecycle.State              `json:"from_state"`
	TargetState                      nodelifecycle.State              `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType     `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                           `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                           `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                           `json:"expected_lifecycle_resource_version"`
	ObservedNodeID                   string                           `json:"observed_node_id"`
	ObservedLifecycleState           nodelifecycle.State              `json:"observed_lifecycle_state"`
	ObservedLifecycleGeneration      uint64                           `json:"observed_lifecycle_generation"`
	ObservedLifecycleResourceVersion string                           `json:"observed_lifecycle_resource_version"`
	ClusterID                        string                           `json:"cluster_id"`
	ClusterGeneration                uint64                           `json:"cluster_generation"`
	MembershipSnapshotID             string                           `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                           `json:"revision_evidence_id"`
	Revision                         TransitionRevision               `json:"revision"`
	QuorumRequired                   int                              `json:"quorum_required"`
	QuorumObserved                   int                              `json:"quorum_observed"`
	HealthyVotesObserved             int                              `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                              `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                              `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                             `json:"standalone_downtime_bound"`
	Outcome                          LifecycleRecoveryRollbackOutcome `json:"outcome"`
	SafetyBoundarySatisfied          bool                             `json:"safety_boundary_satisfied"`
	CompletionReceiptRequired        bool                             `json:"completion_receipt_required"`
	FreshAdmissionRequired           bool                             `json:"fresh_admission_required"`
	ReconciliationRequired           bool                             `json:"reconciliation_required"`
	RetryAuthorized                  bool                             `json:"retry_authorized"`
	LifecycleStateMutationAuthorized bool                             `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                             `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                             `json:"failover_authorized"`
	GenericCommandAuthorized         bool                             `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                             `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackReconciliationHandoff creates evidence only.
// It intentionally never reuses the old single-use admission for a second CAS.
func BuildLifecycleRecoveryRollbackReconciliationHandoff(
	observation LifecycleRecoveryRollbackReconciliationObservation,
) (LifecycleRecoveryRollbackReconciliationHandoff, error) {
	if observation.Admission.RollbackAdmissionID == "" || observation.Source.Plan.RollbackPlanID == "" {
		return LifecycleRecoveryRollbackReconciliationHandoff{}, fmt.Errorf(
			"%w: rollback admission and source plan are required",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	if err := RevalidateLifecycleRecoveryRollbackAdmission(observation.Admission, observation.Source); err != nil {
		return LifecycleRecoveryRollbackReconciliationHandoff{}, classifyLifecycleRecoveryRollbackReconciliationError(err)
	}
	if err := revalidateRollbackReconciliationHA(observation); err != nil {
		return LifecycleRecoveryRollbackReconciliationHandoff{}, err
	}
	if err := validateLifecycleCASObservation(observation.ObservedCAS); err != nil {
		return LifecycleRecoveryRollbackReconciliationHandoff{}, fmt.Errorf(
			"%w: observed lifecycle CAS: %v",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
			err,
		)
	}

	admission := observation.Admission
	currentQuorum := quorum(len(observation.CurrentMembership.Members))
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	safetySatisfied := currentQuorum == admission.QuorumRequired &&
		currentHealthy >= admission.QuorumRequired &&
		currentReady >= admission.MinimumReadyNodes
	outcome := classifyLifecycleRecoveryRollbackOutcome(observation.ObservedCAS, admission)

	completionRequired := outcome == LifecycleRecoveryRollbackApplied && safetySatisfied
	freshAdmissionRequired := outcome == LifecycleRecoveryRollbackNotApplied && safetySatisfied
	reconciliationRequired := !completionRequired && !freshAdmissionRequired

	handoff := LifecycleRecoveryRollbackReconciliationHandoff{
		SchemaVersion:                    LifecycleRecoveryRollbackReconciliationHandoffSchemaV1,
		RollbackAdmissionID:              admission.RollbackAdmissionID,
		RollbackPlanID:                   admission.RollbackPlanID,
		RecoveryHandoffID:                admission.RecoveryHandoffID,
		OriginalAdmissionID:              admission.OriginalAdmissionID,
		NodeID:                           admission.NodeID,
		FromState:                        admission.FromState,
		TargetState:                      admission.TargetState,
		TransitionType:                   admission.TransitionType,
		ExpectedLifecycleGeneration:      admission.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       admission.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: admission.ExpectedLifecycleResourceVersion,
		ObservedNodeID:                   observation.ObservedCAS.NodeID,
		ObservedLifecycleState:           observation.ObservedCAS.State,
		ObservedLifecycleGeneration:      observation.ObservedCAS.Generation,
		ObservedLifecycleResourceVersion: observation.ObservedCAS.ResourceVersion,
		ClusterID:                        admission.ClusterID,
		ClusterGeneration:                admission.ClusterGeneration,
		MembershipSnapshotID:             admission.MembershipSnapshotID,
		RevisionEvidenceID:               admission.RevisionEvidenceID,
		Revision:                         admission.Revision,
		QuorumRequired:                   admission.QuorumRequired,
		QuorumObserved:                   currentQuorum,
		HealthyVotesObserved:             currentHealthy,
		MinimumReadyNodes:                admission.MinimumReadyNodes,
		ReadyNodesObserved:               currentReady,
		StandaloneDowntimeBound:          admission.StandaloneDowntimeBound,
		Outcome:                          outcome,
		SafetyBoundarySatisfied:          safetySatisfied,
		CompletionReceiptRequired:        completionRequired,
		FreshAdmissionRequired:           freshAdmissionRequired,
		ReconciliationRequired:           reconciliationRequired,
		RetryAuthorized:                  false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackReconciliationHandoffID(handoff)
	if err != nil {
		return LifecycleRecoveryRollbackReconciliationHandoff{}, err
	}
	handoff.ReconciliationHandoffID = id
	return handoff, nil
}

// RevalidateLifecycleRecoveryRollbackReconciliationHandoff proves that stored
// classification evidence still matches the exact lifecycle read and HA
// lineage. It cannot turn a fresh-admission request into retry authority.
func RevalidateLifecycleRecoveryRollbackReconciliationHandoff(
	handoff LifecycleRecoveryRollbackReconciliationHandoff,
	observation LifecycleRecoveryRollbackReconciliationObservation,
) error {
	if err := validateLifecycleRecoveryRollbackReconciliationHandoff(handoff); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(observation)
	if err != nil {
		return err
	}
	if rebuilt != handoff {
		return fmt.Errorf(
			"%w: immutable rollback reconciliation evidence changed",
			ErrStaleLifecycleRecoveryRollbackReconciliation,
		)
	}
	return nil
}

func revalidateRollbackReconciliationHA(observation LifecycleRecoveryRollbackReconciliationObservation) error {
	recoveryObservation := observation.Source.PlanRequest.RecoveryObservation
	recoveryObservation.CurrentMembership = observation.CurrentMembership
	recoveryObservation.CurrentEvidence = observation.CurrentEvidence
	if err := RevalidateLifecycleExecutionRecoveryHandoff(
		observation.Source.PlanRequest.RecoveryHandoff,
		recoveryObservation,
	); err != nil {
		return classifyLifecycleRecoveryRollbackReconciliationError(err)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackOutcome(
	observed LifecycleCASObservation,
	admission LifecycleRecoveryRollbackAdmission,
) LifecycleRecoveryRollbackOutcome {
	if observed.NodeID != admission.NodeID {
		return LifecycleRecoveryRollbackSuperseded
	}
	if observed.State == admission.TargetState &&
		observed.Generation == admission.PlannedLifecycleGeneration &&
		observed.ResourceVersion != admission.ExpectedLifecycleResourceVersion {
		return LifecycleRecoveryRollbackApplied
	}
	if observed.State == admission.FromState &&
		observed.Generation == admission.ExpectedLifecycleGeneration &&
		observed.ResourceVersion == admission.ExpectedLifecycleResourceVersion {
		return LifecycleRecoveryRollbackNotApplied
	}
	if (observed.State == admission.FromState || observed.State == admission.TargetState) &&
		(observed.Generation == admission.ExpectedLifecycleGeneration ||
			observed.Generation == admission.PlannedLifecycleGeneration) {
		return LifecycleRecoveryRollbackAmbiguous
	}
	return LifecycleRecoveryRollbackSuperseded
}

func classifyLifecycleRecoveryRollbackReconciliationError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) ||
		errors.Is(err, ErrStaleLifecycleExecutionRecovery) ||
		errors.Is(err, ErrStaleLifecycleExecutionAdmission) ||
		errors.Is(err, ErrStaleLifecycleHandoff) ||
		errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackReconciliation, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackReconciliation, err)
}

func validateLifecycleRecoveryRollbackReconciliationHandoff(
	handoff LifecycleRecoveryRollbackReconciliationHandoff,
) error {
	if handoff.SchemaVersion != LifecycleRecoveryRollbackReconciliationHandoffSchemaV1 ||
		handoff.ReconciliationHandoffID == "" || handoff.RollbackAdmissionID == "" ||
		handoff.RollbackPlanID == "" || handoff.RecoveryHandoffID == "" ||
		handoff.OriginalAdmissionID == "" || handoff.NodeID == "" || handoff.ObservedNodeID == "" ||
		handoff.ClusterID == "" || handoff.ClusterGeneration == 0 || handoff.MembershipSnapshotID == "" ||
		handoff.RevisionEvidenceID == "" || handoff.ExpectedLifecycleGeneration == 0 ||
		handoff.PlannedLifecycleGeneration == 0 || handoff.ExpectedLifecycleResourceVersion == "" ||
		handoff.ObservedLifecycleGeneration == 0 || handoff.ObservedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: reconciliation identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	if handoff.FromState != nodelifecycle.StateDraining || handoff.TargetState != nodelifecycle.StateReady ||
		handoff.TransitionType != nodelifecycle.TransitionDesired ||
		handoff.PlannedLifecycleGeneration != handoff.ExpectedLifecycleGeneration+1 ||
		!handoff.ObservedLifecycleState.Valid() {
		return fmt.Errorf(
			"%w: reconciliation lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	if handoff.QuorumRequired <= 0 || handoff.QuorumObserved <= 0 ||
		handoff.MinimumReadyNodes != handoff.QuorumRequired ||
		handoff.HealthyVotesObserved < 0 || handoff.ReadyNodesObserved < 0 {
		return fmt.Errorf(
			"%w: reconciliation quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	if handoff.RetryAuthorized || handoff.LifecycleStateMutationAuthorized ||
		handoff.MembershipMutationAuthorized || handoff.FailoverAuthorized ||
		handoff.GenericCommandAuthorized || handoff.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: reconciliation safety flags grant mutation authority",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	if err := validateRollbackReconciliationDecision(handoff); err != nil {
		return err
	}
	if err := validateTransitionRevision(handoff.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleRecoveryRollbackReconciliation, err)
	}
	for field, value := range map[string]string{
		"reconciliation_handoff_id":           handoff.ReconciliationHandoffID,
		"rollback_admission_id":               handoff.RollbackAdmissionID,
		"rollback_plan_id":                    handoff.RollbackPlanID,
		"recovery_handoff_id":                 handoff.RecoveryHandoffID,
		"original_admission_id":               handoff.OriginalAdmissionID,
		"node_id":                             handoff.NodeID,
		"observed_node_id":                    handoff.ObservedNodeID,
		"cluster_id":                          handoff.ClusterID,
		"membership_snapshot_id":              handoff.MembershipSnapshotID,
		"revision_evidence_id":                handoff.RevisionEvidenceID,
		"expected_lifecycle_resource_version": handoff.ExpectedLifecycleResourceVersion,
		"observed_lifecycle_resource_version": handoff.ObservedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackReconciliation, err)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackReconciliationHandoffID(handoff)
	if err != nil {
		return err
	}
	if expectedID != handoff.ReconciliationHandoffID {
		return fmt.Errorf(
			"%w: rollback reconciliation fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	return nil
}

func validateRollbackReconciliationDecision(
	handoff LifecycleRecoveryRollbackReconciliationHandoff,
) error {
	safetyDerived := handoff.QuorumObserved == handoff.QuorumRequired &&
		handoff.HealthyVotesObserved >= handoff.QuorumRequired &&
		handoff.ReadyNodesObserved >= handoff.MinimumReadyNodes
	if handoff.SafetyBoundarySatisfied != safetyDerived {
		return fmt.Errorf(
			"%w: safety-boundary flag does not match observed votes",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}

	observed := LifecycleCASObservation{
		NodeID:          handoff.ObservedNodeID,
		State:           handoff.ObservedLifecycleState,
		Generation:      handoff.ObservedLifecycleGeneration,
		ResourceVersion: handoff.ObservedLifecycleResourceVersion,
	}
	admission := LifecycleRecoveryRollbackAdmission{
		NodeID:                           handoff.NodeID,
		FromState:                        handoff.FromState,
		TargetState:                      handoff.TargetState,
		ExpectedLifecycleGeneration:      handoff.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       handoff.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: handoff.ExpectedLifecycleResourceVersion,
	}
	if classifyLifecycleRecoveryRollbackOutcome(observed, admission) != handoff.Outcome {
		return fmt.Errorf(
			"%w: reconciliation outcome does not match lifecycle evidence",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}

	wantCompletion := handoff.Outcome == LifecycleRecoveryRollbackApplied && handoff.SafetyBoundarySatisfied
	wantFreshAdmission := handoff.Outcome == LifecycleRecoveryRollbackNotApplied && handoff.SafetyBoundarySatisfied
	wantReconciliation := !wantCompletion && !wantFreshAdmission
	if handoff.CompletionReceiptRequired != wantCompletion ||
		handoff.FreshAdmissionRequired != wantFreshAdmission ||
		handoff.ReconciliationRequired != wantReconciliation {
		return fmt.Errorf(
			"%w: reconciliation action flags do not match outcome and safety evidence",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackReconciliationHandoffID(
	handoff LifecycleRecoveryRollbackReconciliationHandoff,
) (string, error) {
	input := handoff
	input.ReconciliationHandoffID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackReconciliation,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrh-" + hex.EncodeToString(digest[:])[:24], nil
}
