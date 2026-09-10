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
	ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation = errors.New(
		"invalid cluster lifecycle recovery rollback fresh attempt reconciliation",
	)
	ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation = errors.New(
		"stale cluster lifecycle recovery rollback fresh attempt reconciliation",
	)
)

const LifecycleRecoveryRollbackFreshAttemptReconciliationReceiptSchemaV1 = "clusterha.lifecycle-recovery-rollback-fresh-attempt-reconciliation-receipt/v1"

type LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome string

const (
	LifecycleRecoveryRollbackFreshAttemptReconciledApplied    LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome = "applied"
	LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome = "definitely_not_applied"
	LifecycleRecoveryRollbackFreshAttemptReconciledSuperseded LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome = "superseded"
	LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous  LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome = "ambiguous"
)

// LifecycleRecoveryRollbackFreshAttemptReconciliationObservation binds one
// reconciliation-required completion receipt to a fresh authoritative lifecycle
// and HA read. It never carries caller-selected commands, roles, membership
// changes, or host operations.
type LifecycleRecoveryRollbackFreshAttemptReconciliationObservation struct {
	CompletionReceipt     LifecycleRecoveryRollbackFreshAttemptCompletionReceipt     `json:"completion_receipt"`
	CompletionObservation LifecycleRecoveryRollbackFreshAttemptCompletionObservation `json:"completion_observation"`
	CurrentCAS            LifecycleCASObservation                                    `json:"current_cas"`
	CurrentMembership     Snapshot                                                   `json:"current_membership"`
	CurrentEvidence       TransitionRevisionEvidence                                 `json:"-"`
}

// LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt is immutable
// evidence that classifies the durable state after a cas_lost or ambiguous fresh
// rollback attempt. Even a definitely-not-applied result requires a separate
// fresh admission and never authorizes an automatic retry.
type LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt struct {
	SchemaVersion                    string                                                     `json:"schema_version"`
	ReconciliationReceiptID          string                                                     `json:"reconciliation_receipt_id"`
	CompletionReceiptID              string                                                     `json:"completion_receipt_id"`
	FreshRollbackAttemptID           string                                                     `json:"fresh_rollback_attempt_id"`
	FreshAdmissionGateID             string                                                     `json:"fresh_admission_gate_id"`
	ReconciliationHandoffID          string                                                     `json:"reconciliation_handoff_id"`
	PreviousRollbackAdmissionID      string                                                     `json:"previous_rollback_admission_id"`
	RollbackPlanID                   string                                                     `json:"rollback_plan_id"`
	RecoveryHandoffID                string                                                     `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                                                     `json:"original_admission_id"`
	NodeID                           string                                                     `json:"node_id"`
	FromState                        nodelifecycle.State                                        `json:"from_state"`
	TargetState                      nodelifecycle.State                                        `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType                               `json:"transition_type"`
	PreviousLifecycleGeneration      uint64                                                     `json:"previous_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                                                     `json:"planned_lifecycle_generation"`
	PreviousLifecycleResourceVersion string                                                     `json:"previous_lifecycle_resource_version"`
	ObservedLifecycleState           nodelifecycle.State                                        `json:"observed_lifecycle_state"`
	ObservedLifecycleGeneration      uint64                                                     `json:"observed_lifecycle_generation"`
	ObservedLifecycleResourceVersion string                                                     `json:"observed_lifecycle_resource_version"`
	ClusterID                        string                                                     `json:"cluster_id"`
	CurrentClusterGeneration         uint64                                                     `json:"current_cluster_generation"`
	CurrentMembershipSnapshotID      string                                                     `json:"current_membership_snapshot_id"`
	CurrentRevisionEvidenceID        string                                                     `json:"current_revision_evidence_id"`
	CurrentRevision                  TransitionRevision                                         `json:"current_revision"`
	CurrentQuorumRequired            int                                                        `json:"current_quorum_required"`
	CurrentHealthyVotesObserved      int                                                        `json:"current_healthy_votes_observed"`
	CurrentReadyNodesObserved        int                                                        `json:"current_ready_nodes_observed"`
	SafetyBoundarySatisfied          bool                                                       `json:"safety_boundary_satisfied"`
	HARevisionAdvanced               bool                                                       `json:"ha_revision_advanced"`
	Outcome                          LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome `json:"outcome"`
	CompletionConfirmed              bool                                                       `json:"completion_confirmed"`
	FreshAdmissionRequired           bool                                                       `json:"fresh_admission_required"`
	ReconciliationRequired           bool                                                       `json:"reconciliation_required"`
	FurtherAttemptAuthorized         bool                                                       `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized         bool                                                       `json:"automatic_retry_authorized"`
	LifecycleStateMutationAuthorized bool                                                       `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                                                       `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                                                       `json:"failover_authorized"`
	GenericCommandAuthorized         bool                                                       `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                                                       `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt classifies a
// fresh authoritative read after uncertain CAS durability. It produces evidence
// only and never retries or authorizes a new lifecycle mutation.
func BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(
	observation LifecycleRecoveryRollbackFreshAttemptReconciliationObservation,
) (LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt, error) {
	receipt := observation.CompletionReceipt
	if !receipt.ReconciliationRequired ||
		(receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptCASLost &&
			receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptAmbiguous) {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{}, fmt.Errorf(
			"%w: completion receipt is not reconciliation-required",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if err := RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(
		receipt,
		observation.CompletionObservation,
	); err != nil {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{},
			classifyLifecycleRecoveryRollbackFreshAttemptReconciliationError(err)
	}
	if err := validateLifecycleCASObservation(observation.CurrentCAS); err != nil {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{}, fmt.Errorf(
			"%w: current lifecycle CAS: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	if observation.CurrentCAS.NodeID != receipt.NodeID {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{}, fmt.Errorf(
			"%w: current lifecycle CAS names a different node",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}

	currentSnapshotID, err := validateFreshAttemptReconciliationHA(observation, receipt)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{}, err
	}

	currentRevision := observation.CurrentEvidence.Revision()
	currentQuorum := quorum(len(observation.CurrentMembership.Members))
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	safetySatisfied := currentHealthy >= currentQuorum && currentReady >= currentQuorum
	outcome := classifyFreshAttemptReconciliationOutcome(receipt, observation.CurrentCAS)
	completionConfirmed := outcome == LifecycleRecoveryRollbackFreshAttemptReconciledApplied
	freshAdmissionRequired := outcome == LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied
	reconciliationRequired := outcome == LifecycleRecoveryRollbackFreshAttemptReconciledSuperseded ||
		outcome == LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous

	result := LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{
		SchemaVersion:                    LifecycleRecoveryRollbackFreshAttemptReconciliationReceiptSchemaV1,
		CompletionReceiptID:              receipt.ReceiptID,
		FreshRollbackAttemptID:           receipt.FreshRollbackAttemptID,
		FreshAdmissionGateID:             receipt.FreshAdmissionGateID,
		ReconciliationHandoffID:          receipt.ReconciliationHandoffID,
		PreviousRollbackAdmissionID:      receipt.PreviousRollbackAdmissionID,
		RollbackPlanID:                   receipt.RollbackPlanID,
		RecoveryHandoffID:                receipt.RecoveryHandoffID,
		OriginalAdmissionID:              receipt.OriginalAdmissionID,
		NodeID:                           receipt.NodeID,
		FromState:                        receipt.FromState,
		TargetState:                      receipt.TargetState,
		TransitionType:                   receipt.TransitionType,
		PreviousLifecycleGeneration:      receipt.PreviousLifecycleGeneration,
		PlannedLifecycleGeneration:       receipt.PlannedLifecycleGeneration,
		PreviousLifecycleResourceVersion: receipt.PreviousLifecycleResourceVersion,
		ObservedLifecycleState:           observation.CurrentCAS.State,
		ObservedLifecycleGeneration:      observation.CurrentCAS.Generation,
		ObservedLifecycleResourceVersion: observation.CurrentCAS.ResourceVersion,
		ClusterID:                        receipt.ClusterID,
		CurrentClusterGeneration:         observation.CurrentMembership.Generation,
		CurrentMembershipSnapshotID:      currentSnapshotID,
		CurrentRevisionEvidenceID:        observation.CurrentEvidence.EvidenceID(),
		CurrentRevision:                  currentRevision,
		CurrentQuorumRequired:            currentQuorum,
		CurrentHealthyVotesObserved:      currentHealthy,
		CurrentReadyNodesObserved:        currentReady,
		SafetyBoundarySatisfied:          safetySatisfied,
		HARevisionAdvanced:               freshAttemptReconciliationHAAdvanced(receipt, observation.CurrentEvidence),
		Outcome:                          outcome,
		CompletionConfirmed:              completionConfirmed,
		FreshAdmissionRequired:           freshAdmissionRequired,
		ReconciliationRequired:           reconciliationRequired,
		FurtherAttemptAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackFreshAttemptReconciliationReceiptID(result)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt{}, err
	}
	result.ReconciliationReceiptID = id
	return result, nil
}

func RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(
	receipt LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
	observation LifecycleRecoveryRollbackFreshAttemptReconciliationObservation,
) error {
	if err := validateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(receipt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf(
			"%w: immutable fresh rollback reconciliation evidence changed",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	return nil
}

func validateFreshAttemptReconciliationHA(
	observation LifecycleRecoveryRollbackFreshAttemptReconciliationObservation,
	completion LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
) (string, error) {
	if _, _, err := validateSnapshot(observation.CurrentMembership); err != nil {
		return "", fmt.Errorf(
			"%w: current membership: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	if err := validateTransitionRevisionEvidence(observation.CurrentEvidence); err != nil {
		return "", fmt.Errorf(
			"%w: current revision evidence: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	currentSnapshotID, err := membershipSnapshotID(observation.CurrentMembership)
	if err != nil {
		return "", fmt.Errorf(
			"%w: current membership snapshot: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	if observation.CurrentMembership.ClusterID != completion.ClusterID ||
		observation.CurrentEvidence.ClusterID() != completion.ClusterID {
		return "", fmt.Errorf(
			"%w: cluster identity changed during reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if observation.CurrentEvidence.MembershipSnapshotID() != currentSnapshotID ||
		observation.CurrentEvidence.Generation() != observation.CurrentMembership.Generation ||
		observation.CurrentEvidence.LeaderID() != observation.CurrentMembership.LeaderID {
		return "", fmt.Errorf(
			"%w: current membership and revision evidence disagree",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if observation.CurrentMembership.Generation < completion.ClusterGeneration {
		return "", fmt.Errorf(
			"%w: cluster generation regressed during reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	currentRevision := observation.CurrentEvidence.Revision()
	if currentRevision.LeaderEpoch < completion.Revision.LeaderEpoch ||
		currentRevision.JournalSequence < completion.Revision.JournalSequence {
		return "", fmt.Errorf(
			"%w: HA revision regressed during reconciliation",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if currentRevision.JournalSequence == completion.Revision.JournalSequence &&
		currentRevision.ResourceVersion != completion.Revision.ResourceVersion {
		return "", fmt.Errorf(
			"%w: HA resource version changed without journal advancement",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	return currentSnapshotID, nil
}

func classifyFreshAttemptReconciliationOutcome(
	completion LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
	current LifecycleCASObservation,
) LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome {
	exactPrecondition := current.State == completion.FromState &&
		current.Generation == completion.PreviousLifecycleGeneration &&
		current.ResourceVersion == completion.PreviousLifecycleResourceVersion
	exactApplied := current.State == completion.TargetState &&
		current.Generation == completion.PlannedLifecycleGeneration &&
		current.ResourceVersion != completion.PreviousLifecycleResourceVersion

	if exactApplied {
		return LifecycleRecoveryRollbackFreshAttemptReconciledApplied
	}
	if exactPrecondition {
		if completion.Outcome == LifecycleRecoveryRollbackFreshAttemptCASLost {
			// CASLost already proved the exact precondition absent. Seeing the old
			// resource version again is an ABA/regression signal, not retry proof.
			return LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous
		}
		if completion.ObservedLifecycleState == completion.FromState &&
			completion.ObservedLifecycleGeneration == completion.PreviousLifecycleGeneration &&
			completion.ObservedLifecycleResourceVersion == completion.PreviousLifecycleResourceVersion {
			return LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied
		}
		return LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous
	}
	if current.ResourceVersion == completion.PreviousLifecycleResourceVersion {
		// A semantic lifecycle change with the same storage resource version is
		// inconsistent evidence and must remain reconciliation-only.
		return LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous
	}
	return LifecycleRecoveryRollbackFreshAttemptReconciledSuperseded
}

func freshAttemptReconciliationHAAdvanced(
	completion LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
	current TransitionRevisionEvidence,
) bool {
	revision := current.Revision()
	return current.Generation() != completion.ClusterGeneration ||
		current.EvidenceID() != completion.RevisionEvidenceID ||
		revision.LeaderEpoch != completion.Revision.LeaderEpoch ||
		revision.JournalSequence != completion.Revision.JournalSequence ||
		revision.ResourceVersion != completion.Revision.ResourceVersion
}

func classifyLifecycleRecoveryRollbackFreshAttemptReconciliationError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackReconciliation) ||
		errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation, err)
}

func validateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(
	receipt LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
) error {
	if receipt.SchemaVersion != LifecycleRecoveryRollbackFreshAttemptReconciliationReceiptSchemaV1 ||
		receipt.ReconciliationReceiptID == "" || receipt.CompletionReceiptID == "" ||
		receipt.FreshRollbackAttemptID == "" || receipt.FreshAdmissionGateID == "" ||
		receipt.ReconciliationHandoffID == "" || receipt.PreviousRollbackAdmissionID == "" ||
		receipt.RollbackPlanID == "" || receipt.RecoveryHandoffID == "" ||
		receipt.OriginalAdmissionID == "" || receipt.NodeID == "" || receipt.ClusterID == "" ||
		receipt.PreviousLifecycleGeneration == 0 || receipt.PlannedLifecycleGeneration == 0 ||
		receipt.PreviousLifecycleResourceVersion == "" || receipt.ObservedLifecycleGeneration == 0 ||
		receipt.ObservedLifecycleResourceVersion == "" || receipt.CurrentClusterGeneration == 0 ||
		receipt.CurrentMembershipSnapshotID == "" || receipt.CurrentRevisionEvidenceID == "" {
		return fmt.Errorf(
			"%w: reconciliation identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if receipt.FromState != nodelifecycle.StateDraining ||
		receipt.TargetState != nodelifecycle.StateReady ||
		receipt.TransitionType != nodelifecycle.TransitionDesired ||
		receipt.PlannedLifecycleGeneration != receipt.PreviousLifecycleGeneration+1 ||
		!receipt.ObservedLifecycleState.Valid() {
		return fmt.Errorf(
			"%w: reconciliation lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if receipt.CurrentQuorumRequired <= 0 || receipt.CurrentHealthyVotesObserved < 0 ||
		receipt.CurrentReadyNodesObserved < 0 {
		return fmt.Errorf(
			"%w: reconciliation quorum evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.LifecycleStateMutationAuthorized || receipt.MembershipMutationAuthorized ||
		receipt.FailoverAuthorized || receipt.GenericCommandAuthorized || receipt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: reconciliation evidence grants mutation authority",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	if err := validateFreshAttemptReconciliationDecision(receipt); err != nil {
		return err
	}
	if err := validateTransitionRevision(receipt.CurrentRevision); err != nil {
		return fmt.Errorf(
			"%w: current revision: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	for field, value := range map[string]string{
		"reconciliation_receipt_id":           receipt.ReconciliationReceiptID,
		"completion_receipt_id":               receipt.CompletionReceiptID,
		"fresh_rollback_attempt_id":           receipt.FreshRollbackAttemptID,
		"fresh_admission_gate_id":             receipt.FreshAdmissionGateID,
		"reconciliation_handoff_id":           receipt.ReconciliationHandoffID,
		"previous_rollback_admission_id":      receipt.PreviousRollbackAdmissionID,
		"rollback_plan_id":                    receipt.RollbackPlanID,
		"recovery_handoff_id":                 receipt.RecoveryHandoffID,
		"original_admission_id":               receipt.OriginalAdmissionID,
		"node_id":                             receipt.NodeID,
		"cluster_id":                          receipt.ClusterID,
		"previous_lifecycle_resource_version": receipt.PreviousLifecycleResourceVersion,
		"observed_lifecycle_resource_version": receipt.ObservedLifecycleResourceVersion,
		"current_membership_snapshot_id":      receipt.CurrentMembershipSnapshotID,
		"current_revision_evidence_id":        receipt.CurrentRevisionEvidenceID,
		"current_revision_resource_version":   receipt.CurrentRevision.ResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackFreshAttemptReconciliationReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReconciliationReceiptID {
		return fmt.Errorf(
			"%w: fresh rollback reconciliation fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
		)
	}
	return nil
}

func validateFreshAttemptReconciliationDecision(
	receipt LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
) error {
	switch receipt.Outcome {
	case LifecycleRecoveryRollbackFreshAttemptReconciledApplied:
		if !receipt.CompletionConfirmed || receipt.FreshAdmissionRequired ||
			receipt.ReconciliationRequired || receipt.ObservedLifecycleState != receipt.TargetState ||
			receipt.ObservedLifecycleGeneration != receipt.PlannedLifecycleGeneration ||
			receipt.ObservedLifecycleResourceVersion == receipt.PreviousLifecycleResourceVersion {
			return fmt.Errorf(
				"%w: applied reconciliation decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied:
		if receipt.CompletionConfirmed || !receipt.FreshAdmissionRequired || receipt.ReconciliationRequired ||
			receipt.ObservedLifecycleState != receipt.FromState ||
			receipt.ObservedLifecycleGeneration != receipt.PreviousLifecycleGeneration ||
			receipt.ObservedLifecycleResourceVersion != receipt.PreviousLifecycleResourceVersion {
			return fmt.Errorf(
				"%w: definitely-not-applied reconciliation decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptReconciledSuperseded,
		LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous:
		if receipt.CompletionConfirmed || receipt.FreshAdmissionRequired || !receipt.ReconciliationRequired {
			return fmt.Errorf(
				"%w: reconciliation-only decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unsupported reconciliation outcome %q",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			receipt.Outcome,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackFreshAttemptReconciliationReceiptID(
	receipt LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
) (string, error) {
	input := receipt
	input.ReconciliationReceiptID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrfr-" + hex.EncodeToString(digest[:])[:24], nil
}
