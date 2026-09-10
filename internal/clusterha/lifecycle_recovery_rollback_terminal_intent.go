package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrInvalidLifecycleRecoveryRollbackTerminalIntent = errors.New(
		"invalid cluster lifecycle recovery rollback terminal intent",
	)
	ErrStaleLifecycleRecoveryRollbackTerminalIntent = errors.New(
		"stale cluster lifecycle recovery rollback terminal intent",
	)
)

const LifecycleRecoveryRollbackTerminalIntentReceiptSchemaV1 = "clusterha.lifecycle-recovery-rollback-terminal-intent-receipt/v1"

type LifecycleRecoveryRollbackTerminalIntentAction string

const (
	LifecycleRecoveryRollbackTerminalIntentRevalidate LifecycleRecoveryRollbackTerminalIntentAction = "operator_revalidate_new_admission"
	LifecycleRecoveryRollbackTerminalIntentClose      LifecycleRecoveryRollbackTerminalIntentAction = "operator_close_recovery"
)

// LifecycleRecoveryRollbackTerminalIntentRequest binds an explicit operator
// decision to one definitely-not-applied fresh rollback reconciliation. It
// never carries caller-selected commands, roles, membership changes, or host
// operations.
type LifecycleRecoveryRollbackTerminalIntentRequest struct {
	ReconciliationReceipt     LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt     `json:"reconciliation_receipt"`
	ReconciliationObservation LifecycleRecoveryRollbackFreshAttemptReconciliationObservation `json:"reconciliation_observation"`
	RecoveryIntentID          string                                                         `json:"recovery_intent_id"`
	Action                    LifecycleRecoveryRollbackTerminalIntentAction                  `json:"action"`
	OperatorApproved          bool                                                           `json:"operator_approved"`
}

// LifecycleRecoveryRollbackTerminalIntentReceipt is immutable terminal
// evidence after the first fresh rollback attempt was proven not applied. It
// records either an operator request for a future fresh safety revalidation or
// an operator decision to close recovery. It is not a retry/admission token.
type LifecycleRecoveryRollbackTerminalIntentReceipt struct {
	SchemaVersion                    string                                                     `json:"schema_version"`
	TerminalIntentReceiptID          string                                                     `json:"terminal_intent_receipt_id"`
	ReconciliationReceiptID          string                                                     `json:"reconciliation_receipt_id"`
	CompletionReceiptID              string                                                     `json:"completion_receipt_id"`
	FreshRollbackAttemptID           string                                                     `json:"fresh_rollback_attempt_id"`
	FreshAdmissionGateID             string                                                     `json:"fresh_admission_gate_id"`
	PreviousRollbackAdmissionID      string                                                     `json:"previous_rollback_admission_id"`
	RollbackPlanID                   string                                                     `json:"rollback_plan_id"`
	RecoveryHandoffID                string                                                     `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                                                     `json:"original_admission_id"`
	RecoveryIntentID                 string                                                     `json:"recovery_intent_id"`
	NodeID                           string                                                     `json:"node_id"`
	ClusterID                        string                                                     `json:"cluster_id"`
	CurrentClusterGeneration         uint64                                                     `json:"current_cluster_generation"`
	CurrentMembershipSnapshotID      string                                                     `json:"current_membership_snapshot_id"`
	CurrentRevisionEvidenceID        string                                                     `json:"current_revision_evidence_id"`
	CurrentQuorumRequired            int                                                        `json:"current_quorum_required"`
	CurrentHealthyVotesObserved      int                                                        `json:"current_healthy_votes_observed"`
	CurrentReadyNodesObserved        int                                                        `json:"current_ready_nodes_observed"`
	SourceSafetyBoundarySatisfied    bool                                                       `json:"source_safety_boundary_satisfied"`
	SourceOutcome                    LifecycleRecoveryRollbackFreshAttemptReconciliationOutcome `json:"source_outcome"`
	Action                           LifecycleRecoveryRollbackTerminalIntentAction              `json:"action"`
	OperatorApproved                 bool                                                       `json:"operator_approved"`
	FreshAdmissionRequested          bool                                                       `json:"fresh_admission_requested"`
	RecoveryClosureRequested         bool                                                       `json:"recovery_closure_requested"`
	FreshAdmissionAuthorized         bool                                                       `json:"fresh_admission_authorized"`
	FurtherAttemptAuthorized         bool                                                       `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized         bool                                                       `json:"automatic_retry_authorized"`
	LifecycleStateMutationAuthorized bool                                                       `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                                                       `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                                                       `json:"failover_authorized"`
	GenericCommandAuthorized         bool                                                       `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                                                       `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackTerminalIntentReceipt seals a terminal operator
// decision after a definitely-not-applied reconciliation. A request to
// revalidate only records intent; a separate future gate must re-read lifecycle
// CAS, membership, quorum/minimum-ready, fencing and HA revision evidence before
// any admission can be created.
func BuildLifecycleRecoveryRollbackTerminalIntentReceipt(
	request LifecycleRecoveryRollbackTerminalIntentRequest,
) (LifecycleRecoveryRollbackTerminalIntentReceipt, error) {
	if err := RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(
		request.ReconciliationReceipt,
		request.ReconciliationObservation,
	); err != nil {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{},
			classifyLifecycleRecoveryRollbackTerminalIntentError(err)
	}

	source := request.ReconciliationReceipt
	if source.Outcome != LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied ||
		source.CompletionConfirmed || !source.FreshAdmissionRequired || source.ReconciliationRequired {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, fmt.Errorf(
			"%w: source reconciliation is not terminal definitely-not-applied evidence",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	if !request.OperatorApproved {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, fmt.Errorf(
			"%w: explicit operator approval is required",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	if err := validateIdentifier("recovery_intent_id", request.RecoveryIntentID); err != nil {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			err,
		)
	}
	if lifecycleRecoveryRollbackTerminalIntentReusesLineage(request.RecoveryIntentID, source) {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, fmt.Errorf(
			"%w: recovery intent must use new lineage",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}

	freshAdmissionRequested := false
	recoveryClosureRequested := false
	switch request.Action {
	case LifecycleRecoveryRollbackTerminalIntentRevalidate:
		freshAdmissionRequested = true
	case LifecycleRecoveryRollbackTerminalIntentClose:
		recoveryClosureRequested = true
	default:
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, fmt.Errorf(
			"%w: unsupported terminal recovery action %q",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			request.Action,
		)
	}

	receipt := LifecycleRecoveryRollbackTerminalIntentReceipt{
		SchemaVersion:                    LifecycleRecoveryRollbackTerminalIntentReceiptSchemaV1,
		ReconciliationReceiptID:          source.ReconciliationReceiptID,
		CompletionReceiptID:              source.CompletionReceiptID,
		FreshRollbackAttemptID:           source.FreshRollbackAttemptID,
		FreshAdmissionGateID:             source.FreshAdmissionGateID,
		PreviousRollbackAdmissionID:      source.PreviousRollbackAdmissionID,
		RollbackPlanID:                   source.RollbackPlanID,
		RecoveryHandoffID:                source.RecoveryHandoffID,
		OriginalAdmissionID:              source.OriginalAdmissionID,
		RecoveryIntentID:                 request.RecoveryIntentID,
		NodeID:                           source.NodeID,
		ClusterID:                        source.ClusterID,
		CurrentClusterGeneration:         source.CurrentClusterGeneration,
		CurrentMembershipSnapshotID:      source.CurrentMembershipSnapshotID,
		CurrentRevisionEvidenceID:        source.CurrentRevisionEvidenceID,
		CurrentQuorumRequired:            source.CurrentQuorumRequired,
		CurrentHealthyVotesObserved:      source.CurrentHealthyVotesObserved,
		CurrentReadyNodesObserved:        source.CurrentReadyNodesObserved,
		SourceSafetyBoundarySatisfied:    source.SafetyBoundarySatisfied,
		SourceOutcome:                    source.Outcome,
		Action:                           request.Action,
		OperatorApproved:                 true,
		FreshAdmissionRequested:          freshAdmissionRequested,
		RecoveryClosureRequested:         recoveryClosureRequested,
		FreshAdmissionAuthorized:         false,
		FurtherAttemptAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackTerminalIntentReceiptID(receipt)
	if err != nil {
		return LifecycleRecoveryRollbackTerminalIntentReceipt{}, err
	}
	receipt.TerminalIntentReceiptID = id
	return receipt, nil
}

func RevalidateLifecycleRecoveryRollbackTerminalIntentReceipt(
	receipt LifecycleRecoveryRollbackTerminalIntentReceipt,
	request LifecycleRecoveryRollbackTerminalIntentRequest,
) error {
	if err := validateLifecycleRecoveryRollbackTerminalIntentReceipt(receipt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackTerminalIntentReceipt(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf(
			"%w: immutable terminal recovery intent evidence changed",
			ErrStaleLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackTerminalIntentReusesLineage(
	intentID string,
	source LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
) bool {
	for _, priorID := range []string{
		source.ReconciliationReceiptID,
		source.CompletionReceiptID,
		source.FreshRollbackAttemptID,
		source.FreshAdmissionGateID,
		source.ReconciliationHandoffID,
		source.PreviousRollbackAdmissionID,
		source.RollbackPlanID,
		source.RecoveryHandoffID,
		source.OriginalAdmissionID,
		source.CurrentMembershipSnapshotID,
		source.CurrentRevisionEvidenceID,
	} {
		if intentID == priorID {
			return true
		}
	}
	return false
}

func validateLifecycleRecoveryRollbackTerminalIntentReceipt(
	receipt LifecycleRecoveryRollbackTerminalIntentReceipt,
) error {
	if receipt.SchemaVersion != LifecycleRecoveryRollbackTerminalIntentReceiptSchemaV1 ||
		receipt.TerminalIntentReceiptID == "" || receipt.ReconciliationReceiptID == "" ||
		receipt.CompletionReceiptID == "" || receipt.FreshRollbackAttemptID == "" ||
		receipt.FreshAdmissionGateID == "" || receipt.PreviousRollbackAdmissionID == "" ||
		receipt.RollbackPlanID == "" || receipt.RecoveryHandoffID == "" ||
		receipt.OriginalAdmissionID == "" || receipt.RecoveryIntentID == "" ||
		receipt.NodeID == "" || receipt.ClusterID == "" || receipt.CurrentClusterGeneration == 0 ||
		receipt.CurrentMembershipSnapshotID == "" || receipt.CurrentRevisionEvidenceID == "" ||
		receipt.CurrentQuorumRequired <= 0 || receipt.CurrentHealthyVotesObserved < 0 ||
		receipt.CurrentReadyNodesObserved < 0 {
		return fmt.Errorf(
			"%w: terminal recovery intent identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	if receipt.SourceOutcome != LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied ||
		!receipt.OperatorApproved {
		return fmt.Errorf(
			"%w: terminal recovery intent source or approval is invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	switch receipt.Action {
	case LifecycleRecoveryRollbackTerminalIntentRevalidate:
		if !receipt.FreshAdmissionRequested || receipt.RecoveryClosureRequested {
			return fmt.Errorf(
				"%w: revalidation intent decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			)
		}
	case LifecycleRecoveryRollbackTerminalIntentClose:
		if receipt.FreshAdmissionRequested || !receipt.RecoveryClosureRequested {
			return fmt.Errorf(
				"%w: recovery closure decision is inconsistent",
				ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unsupported terminal recovery action %q",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			receipt.Action,
		)
	}
	if receipt.FreshAdmissionAuthorized || receipt.FurtherAttemptAuthorized ||
		receipt.AutomaticRetryAuthorized || receipt.LifecycleStateMutationAuthorized ||
		receipt.MembershipMutationAuthorized || receipt.FailoverAuthorized ||
		receipt.GenericCommandAuthorized || receipt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: terminal recovery intent grants mutation authority",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	for field, value := range map[string]string{
		"terminal_intent_receipt_id":     receipt.TerminalIntentReceiptID,
		"reconciliation_receipt_id":      receipt.ReconciliationReceiptID,
		"completion_receipt_id":          receipt.CompletionReceiptID,
		"fresh_rollback_attempt_id":      receipt.FreshRollbackAttemptID,
		"fresh_admission_gate_id":        receipt.FreshAdmissionGateID,
		"previous_rollback_admission_id": receipt.PreviousRollbackAdmissionID,
		"rollback_plan_id":               receipt.RollbackPlanID,
		"recovery_handoff_id":            receipt.RecoveryHandoffID,
		"original_admission_id":          receipt.OriginalAdmissionID,
		"recovery_intent_id":             receipt.RecoveryIntentID,
		"node_id":                        receipt.NodeID,
		"cluster_id":                     receipt.ClusterID,
		"current_membership_snapshot_id": receipt.CurrentMembershipSnapshotID,
		"current_revision_evidence_id":   receipt.CurrentRevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackTerminalIntentReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.TerminalIntentReceiptID {
		return fmt.Errorf(
			"%w: terminal recovery intent fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackTerminalIntentError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackTerminalIntent, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackTerminalIntent, err)
}

func lifecycleRecoveryRollbackTerminalIntentReceiptID(
	receipt LifecycleRecoveryRollbackTerminalIntentReceipt,
) (string, error) {
	input := receipt
	input.TerminalIntentReceiptID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalIntent,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrti-" + hex.EncodeToString(digest[:])[:24], nil
}
