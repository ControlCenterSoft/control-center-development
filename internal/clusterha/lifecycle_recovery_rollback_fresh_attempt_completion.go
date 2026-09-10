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
	ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion = errors.New(
		"invalid cluster lifecycle recovery rollback fresh attempt completion",
	)
	ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion = errors.New(
		"stale cluster lifecycle recovery rollback fresh attempt completion",
	)
)

const LifecycleRecoveryRollbackFreshAttemptCompletionReceiptSchemaV1 =
	"clusterha.lifecycle-recovery-rollback-fresh-attempt-completion-receipt/v1"

type LifecycleRecoveryRollbackFreshAttemptCommitOutcome string

const (
	LifecycleRecoveryRollbackFreshAttemptApplied   LifecycleRecoveryRollbackFreshAttemptCommitOutcome = "applied"
	LifecycleRecoveryRollbackFreshAttemptCASLost   LifecycleRecoveryRollbackFreshAttemptCommitOutcome = "cas_lost"
	LifecycleRecoveryRollbackFreshAttemptAmbiguous LifecycleRecoveryRollbackFreshAttemptCommitOutcome = "ambiguous"
)

// LifecycleRecoveryRollbackFreshAttemptCASResult is the narrow result returned
// by the audited typed lifecycle-CAS boundary. It contains no command, callback,
// role, membership or host-mutation material.
type LifecycleRecoveryRollbackFreshAttemptCASResult struct {
	Outcome LifecycleRecoveryRollbackFreshAttemptCommitOutcome `json:"outcome"`
	PostCAS LifecycleCASObservation                             `json:"post_cas"`
}

// LifecycleRecoveryRollbackFreshAttemptCompletionObservation binds one exact
// fresh attempt to a final pre-CAS safety read and the typed CAS result. The
// final read must still match the attempt immediately before the CAS boundary.
type LifecycleRecoveryRollbackFreshAttemptCompletionObservation struct {
	Attempt           LifecycleRecoveryRollbackFreshAttempt         `json:"attempt"`
	Source            LifecycleRecoveryRollbackFreshAttemptRequest  `json:"source"`
	FinalCAS          LifecycleCASObservation                        `json:"final_cas"`
	CurrentMembership Snapshot                                       `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                     `json:"-"`
	CASResult         LifecycleRecoveryRollbackFreshAttemptCASResult `json:"cas_result"`
}

// LifecycleRecoveryRollbackFreshAttemptCompletionReceipt is immutable evidence
// of the bounded fresh rollback CAS result. It never authorizes a retry. A lost
// CAS or ambiguous durability result is reconciliation-only.
type LifecycleRecoveryRollbackFreshAttemptCompletionReceipt struct {
	SchemaVersion                    string                                            `json:"schema_version"`
	ReceiptID                        string                                            `json:"receipt_id"`
	FreshRollbackAttemptID           string                                            `json:"fresh_rollback_attempt_id"`
	FreshAdmissionGateID             string                                            `json:"fresh_admission_gate_id"`
	ReconciliationHandoffID          string                                            `json:"reconciliation_handoff_id"`
	PreviousRollbackAdmissionID      string                                            `json:"previous_rollback_admission_id"`
	RollbackPlanID                   string                                            `json:"rollback_plan_id"`
	RecoveryHandoffID                string                                            `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                                            `json:"original_admission_id"`
	NodeID                           string                                            `json:"node_id"`
	FromState                        nodelifecycle.State                               `json:"from_state"`
	TargetState                      nodelifecycle.State                               `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType                      `json:"transition_type"`
	PreviousLifecycleGeneration      uint64                                            `json:"previous_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                                            `json:"planned_lifecycle_generation"`
	PreviousLifecycleResourceVersion string                                            `json:"previous_lifecycle_resource_version"`
	ObservedLifecycleState           nodelifecycle.State                               `json:"observed_lifecycle_state"`
	ObservedLifecycleGeneration      uint64                                            `json:"observed_lifecycle_generation"`
	ObservedLifecycleResourceVersion string                                            `json:"observed_lifecycle_resource_version"`
	ClusterID                        string                                            `json:"cluster_id"`
	ClusterGeneration                uint64                                            `json:"cluster_generation"`
	MembershipSnapshotID             string                                            `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                                            `json:"revision_evidence_id"`
	Revision                         TransitionRevision                                `json:"revision"`
	QuorumRequired                   int                                               `json:"quorum_required"`
	HealthyVotesObserved             int                                               `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                                               `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                                               `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                                              `json:"standalone_downtime_bound"`
	Outcome                          LifecycleRecoveryRollbackFreshAttemptCommitOutcome `json:"outcome"`
	AppliedProven                    bool                                              `json:"applied_proven"`
	CASLostProven                    bool                                              `json:"cas_lost_proven"`
	CompletionVerified               bool                                              `json:"completion_verified"`
	ReconciliationRequired           bool                                              `json:"reconciliation_required"`
	FurtherAttemptAuthorized         bool                                              `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized         bool                                              `json:"automatic_retry_authorized"`
	LifecycleStateMutationAuthorized bool                                              `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                                              `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                                              `json:"failover_authorized"`
	GenericCommandAuthorized         bool                                              `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                                              `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt performs one
// final safety revalidation and seals the typed CAS result. It never retries the
// CAS and never converts completion evidence into further mutation authority.
func BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(
	observation LifecycleRecoveryRollbackFreshAttemptCompletionObservation,
) (LifecycleRecoveryRollbackFreshAttemptCompletionReceipt, error) {
	if observation.Attempt.FreshRollbackAttemptID == "" {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{}, fmt.Errorf(
			"%w: fresh rollback attempt is required",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}

	finalSource := observation.Source
	finalSource.CurrentCAS = observation.FinalCAS
	finalSource.CurrentMembership = observation.CurrentMembership
	finalSource.CurrentEvidence = observation.CurrentEvidence
	if err := RevalidateLifecycleRecoveryRollbackFreshAttempt(observation.Attempt, finalSource); err != nil {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{},
			classifyLifecycleRecoveryRollbackFreshAttemptCompletionError(err)
	}
	if err := validateLifecycleCASObservation(observation.CASResult.PostCAS); err != nil {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS observation: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			err,
		)
	}
	if observation.CASResult.PostCAS.NodeID != observation.Attempt.NodeID {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS node does not match fresh rollback attempt",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	if err := validateFreshRollbackAttemptCASOutcome(observation.Attempt, observation.CASResult); err != nil {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{}, err
	}

	attempt := observation.Attempt
	post := observation.CASResult.PostCAS
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	applied := observation.CASResult.Outcome == LifecycleRecoveryRollbackFreshAttemptApplied
	casLost := observation.CASResult.Outcome == LifecycleRecoveryRollbackFreshAttemptCASLost
	receipt := LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{
		SchemaVersion:                    LifecycleRecoveryRollbackFreshAttemptCompletionReceiptSchemaV1,
		FreshRollbackAttemptID:           attempt.FreshRollbackAttemptID,
		FreshAdmissionGateID:             attempt.FreshAdmissionGateID,
		ReconciliationHandoffID:          attempt.ReconciliationHandoffID,
		PreviousRollbackAdmissionID:      attempt.PreviousRollbackAdmissionID,
		RollbackPlanID:                   attempt.RollbackPlanID,
		RecoveryHandoffID:                attempt.RecoveryHandoffID,
		OriginalAdmissionID:              attempt.OriginalAdmissionID,
		NodeID:                           attempt.NodeID,
		FromState:                        attempt.FromState,
		TargetState:                      attempt.TargetState,
		TransitionType:                   attempt.TransitionType,
		PreviousLifecycleGeneration:      attempt.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       attempt.PlannedLifecycleGeneration,
		PreviousLifecycleResourceVersion: attempt.ExpectedLifecycleResourceVersion,
		ObservedLifecycleState:           post.State,
		ObservedLifecycleGeneration:      post.Generation,
		ObservedLifecycleResourceVersion: post.ResourceVersion,
		ClusterID:                        attempt.ClusterID,
		ClusterGeneration:                attempt.ClusterGeneration,
		MembershipSnapshotID:             attempt.MembershipSnapshotID,
		RevisionEvidenceID:               attempt.RevisionEvidenceID,
		Revision:                         attempt.Revision,
		QuorumRequired:                   attempt.QuorumRequired,
		HealthyVotesObserved:             currentHealthy,
		MinimumReadyNodes:                attempt.MinimumReadyNodes,
		ReadyNodesObserved:               currentReady,
		StandaloneDowntimeBound:          attempt.StandaloneDowntimeBound,
		Outcome:                          observation.CASResult.Outcome,
		AppliedProven:                    applied,
		CASLostProven:                    casLost,
		CompletionVerified:               applied,
		ReconciliationRequired:           !applied,
		FurtherAttemptAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackFreshAttemptCompletionReceiptID(receipt)
	if err != nil {
		return LifecycleRecoveryRollbackFreshAttemptCompletionReceipt{}, err
	}
	receipt.ReceiptID = id
	return receipt, nil
}

func RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(
	receipt LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
	observation LifecycleRecoveryRollbackFreshAttemptCompletionObservation,
) error {
	if err := validateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(receipt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(observation)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf(
			"%w: immutable fresh rollback completion evidence changed",
			ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	return nil
}

func validateFreshRollbackAttemptCASOutcome(
	attempt LifecycleRecoveryRollbackFreshAttempt,
	result LifecycleRecoveryRollbackFreshAttemptCASResult,
) error {
	post := result.PostCAS
	preconditionStillPresent := post.State == attempt.FromState &&
		post.Generation == attempt.ExpectedLifecycleGeneration &&
		post.ResourceVersion == attempt.ExpectedLifecycleResourceVersion
	applied := post.State == attempt.TargetState &&
		post.Generation == attempt.PlannedLifecycleGeneration &&
		post.ResourceVersion != attempt.ExpectedLifecycleResourceVersion

	switch result.Outcome {
	case LifecycleRecoveryRollbackFreshAttemptApplied:
		if !applied {
			return fmt.Errorf(
				"%w: applied result does not prove the exact typed rollback CAS",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptCASLost:
		if applied || preconditionStillPresent {
			return fmt.Errorf(
				"%w: cas-lost result does not prove loss of the exact precondition",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptAmbiguous:
		// Unknown durability is deliberately reconciliation-only regardless of
		// what a single post-CAS read happens to observe.
	default:
		return fmt.Errorf(
			"%w: unsupported typed CAS outcome %q",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			result.Outcome,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackFreshAttemptCompletionError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackReconciliation) ||
		errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion, err)
}

func validateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(
	receipt LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
) error {
	if receipt.SchemaVersion != LifecycleRecoveryRollbackFreshAttemptCompletionReceiptSchemaV1 ||
		receipt.ReceiptID == "" || receipt.FreshRollbackAttemptID == "" ||
		receipt.FreshAdmissionGateID == "" || receipt.ReconciliationHandoffID == "" ||
		receipt.PreviousRollbackAdmissionID == "" || receipt.RollbackPlanID == "" ||
		receipt.RecoveryHandoffID == "" || receipt.OriginalAdmissionID == "" ||
		receipt.NodeID == "" || receipt.ClusterID == "" || receipt.ClusterGeneration == 0 ||
		receipt.MembershipSnapshotID == "" || receipt.RevisionEvidenceID == "" ||
		receipt.PreviousLifecycleGeneration == 0 || receipt.PlannedLifecycleGeneration == 0 ||
		receipt.PreviousLifecycleResourceVersion == "" ||
		receipt.ObservedLifecycleGeneration == 0 || receipt.ObservedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: completion identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	if receipt.FromState != nodelifecycle.StateDraining ||
		receipt.TargetState != nodelifecycle.StateReady ||
		receipt.TransitionType != nodelifecycle.TransitionDesired ||
		receipt.PlannedLifecycleGeneration != receipt.PreviousLifecycleGeneration+1 ||
		!receipt.ObservedLifecycleState.Valid() {
		return fmt.Errorf(
			"%w: completion lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	if receipt.QuorumRequired <= 0 || receipt.MinimumReadyNodes != receipt.QuorumRequired ||
		receipt.HealthyVotesObserved < receipt.QuorumRequired ||
		receipt.ReadyNodesObserved < receipt.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: completion quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	if receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.LifecycleStateMutationAuthorized || receipt.MembershipMutationAuthorized ||
		receipt.FailoverAuthorized || receipt.GenericCommandAuthorized || receipt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: completion evidence grants mutation authority",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	if err := validateFreshRollbackAttemptCompletionDecision(receipt); err != nil {
		return err
	}
	if err := validateTransitionRevision(receipt.Revision); err != nil {
		return fmt.Errorf(
			"%w: revision: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			err,
		)
	}
	for field, value := range map[string]string{
		"receipt_id":                          receipt.ReceiptID,
		"fresh_rollback_attempt_id":           receipt.FreshRollbackAttemptID,
		"fresh_admission_gate_id":             receipt.FreshAdmissionGateID,
		"reconciliation_handoff_id":           receipt.ReconciliationHandoffID,
		"previous_rollback_admission_id":      receipt.PreviousRollbackAdmissionID,
		"rollback_plan_id":                    receipt.RollbackPlanID,
		"recovery_handoff_id":                 receipt.RecoveryHandoffID,
		"original_admission_id":               receipt.OriginalAdmissionID,
		"node_id":                             receipt.NodeID,
		"cluster_id":                          receipt.ClusterID,
		"membership_snapshot_id":              receipt.MembershipSnapshotID,
		"revision_evidence_id":                receipt.RevisionEvidenceID,
		"previous_lifecycle_resource_version": receipt.PreviousLifecycleResourceVersion,
		"observed_lifecycle_resource_version": receipt.ObservedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackFreshAttemptCompletionReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReceiptID {
		return fmt.Errorf(
			"%w: fresh rollback completion fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
		)
	}
	return nil
}

func validateFreshRollbackAttemptCompletionDecision(
	receipt LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
) error {
	switch receipt.Outcome {
	case LifecycleRecoveryRollbackFreshAttemptApplied:
		if !receipt.AppliedProven || receipt.CASLostProven || !receipt.CompletionVerified ||
			receipt.ReconciliationRequired || receipt.ObservedLifecycleState != receipt.TargetState ||
			receipt.ObservedLifecycleGeneration != receipt.PlannedLifecycleGeneration ||
			receipt.ObservedLifecycleResourceVersion == receipt.PreviousLifecycleResourceVersion {
			return fmt.Errorf(
				"%w: applied completion decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptCASLost:
		if receipt.AppliedProven || !receipt.CASLostProven || receipt.CompletionVerified ||
			!receipt.ReconciliationRequired {
			return fmt.Errorf(
				"%w: cas-lost completion decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
		if receipt.ObservedLifecycleState == receipt.FromState &&
			receipt.ObservedLifecycleGeneration == receipt.PreviousLifecycleGeneration &&
			receipt.ObservedLifecycleResourceVersion == receipt.PreviousLifecycleResourceVersion {
			return fmt.Errorf(
				"%w: cas-lost completion retained the exact precondition",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
	case LifecycleRecoveryRollbackFreshAttemptAmbiguous:
		if receipt.AppliedProven || receipt.CASLostProven || receipt.CompletionVerified ||
			!receipt.ReconciliationRequired {
			return fmt.Errorf(
				"%w: ambiguous completion decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unsupported completion outcome %q",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			receipt.Outcome,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackFreshAttemptCompletionReceiptID(
	receipt LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
) (string, error) {
	input := receipt
	input.ReceiptID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrfc-" + hex.EncodeToString(digest[:])[:24], nil
}
