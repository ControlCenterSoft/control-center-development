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
	ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission = errors.New(
		"invalid cluster lifecycle recovery rollback terminal fresh admission",
	)
	ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission = errors.New(
		"stale cluster lifecycle recovery rollback terminal fresh admission",
	)
)

const LifecycleRecoveryRollbackTerminalFreshAdmissionSchemaV1 = "clusterha.lifecycle-recovery-rollback-terminal-fresh-admission/v1"

// LifecycleRecoveryRollbackTerminalFreshAdmissionRequest consumes one exact
// terminal revalidation gate and performs a final lifecycle/HA read immediately
// before minting a new single-use rollback admission. It carries no caller-
// selected command, role, membership, failover or host-mutation material.
type LifecycleRecoveryRollbackTerminalFreshAdmissionRequest struct {
	Gate              LifecycleRecoveryRollbackTerminalRevalidationGate        `json:"gate"`
	GateObservation   LifecycleRecoveryRollbackTerminalRevalidationObservation `json:"gate_observation"`
	CurrentCAS        LifecycleCASObservation                                  `json:"current_cas"`
	CurrentMembership Snapshot                                                 `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                               `json:"-"`
}

// LifecycleRecoveryRollbackTerminalFreshAdmission is a new lineage-bound,
// single-use-by-CAS capability for exactly one Draining -> Ready rollback after
// terminal operator intent and fresh safety revalidation. Previous admissions
// and attempts remain permanently non-reusable.
type LifecycleRecoveryRollbackTerminalFreshAdmission struct {
	SchemaVersion                    string                        `json:"schema_version"`
	TerminalFreshAdmissionID         string                        `json:"terminal_fresh_admission_id"`
	RevalidationGateID               string                        `json:"revalidation_gate_id"`
	TerminalIntentReceiptID          string                        `json:"terminal_intent_receipt_id"`
	ReconciliationReceiptID          string                        `json:"reconciliation_receipt_id"`
	RecoveryIntentID                 string                        `json:"recovery_intent_id"`
	PreviousFreshRollbackAttemptID   string                        `json:"previous_fresh_rollback_attempt_id"`
	PreviousRollbackAdmissionID      string                        `json:"previous_rollback_admission_id"`
	NodeID                           string                        `json:"node_id"`
	ClusterID                        string                        `json:"cluster_id"`
	FromState                        nodelifecycle.State           `json:"from_state"`
	TargetState                      nodelifecycle.State           `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType  `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                        `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                        `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                        `json:"expected_lifecycle_resource_version"`
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
	NewAdmissionLineage              bool                          `json:"new_admission_lineage"`
	PreviousAdmissionReusable        bool                          `json:"previous_admission_reusable"`
	PreviousAttemptReusable          bool                          `json:"previous_attempt_reusable"`
	AutomaticRetryAuthorized         bool                          `json:"automatic_retry_authorized"`
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

func BuildLifecycleRecoveryRollbackTerminalFreshAdmission(
	request LifecycleRecoveryRollbackTerminalFreshAdmissionRequest,
) (LifecycleRecoveryRollbackTerminalFreshAdmission, error) {
	if err := RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate(
		request.Gate,
		request.GateObservation,
	); err != nil {
		return LifecycleRecoveryRollbackTerminalFreshAdmission{},
			classifyLifecycleRecoveryRollbackTerminalFreshAdmissionError(err)
	}

	currentObservation := request.GateObservation
	currentObservation.CurrentCAS = request.CurrentCAS
	currentObservation.CurrentMembership = request.CurrentMembership
	currentObservation.CurrentEvidence = request.CurrentEvidence
	if err := RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate(
		request.Gate,
		currentObservation,
	); err != nil {
		return LifecycleRecoveryRollbackTerminalFreshAdmission{},
			classifyLifecycleRecoveryRollbackTerminalFreshAdmissionError(err)
	}

	gate := request.Gate
	if gate.CurrentLifecycleState != nodelifecycle.StateDraining ||
		gate.CurrentLifecycleGeneration == math.MaxUint64 {
		return LifecycleRecoveryRollbackTerminalFreshAdmission{}, fmt.Errorf(
			"%w: terminal rollback lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if !gate.SafetyBoundarySatisfied || !gate.FreshAdmissionEligible ||
		!gate.RequiresNewAdmissionLineage || gate.PreviousAdmissionReusable ||
		gate.FreshAdmissionAuthorized || gate.FurtherAttemptAuthorized ||
		gate.AutomaticRetryAuthorized || gate.LifecycleStateMutationAuthorized ||
		gate.MembershipMutationAuthorized || gate.FailoverAuthorized ||
		gate.GenericCommandAuthorized || gate.HostMutationAuthorized {
		return LifecycleRecoveryRollbackTerminalFreshAdmission{}, fmt.Errorf(
			"%w: terminal revalidation gate does not preserve the bounded safety boundary",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}

	admission := LifecycleRecoveryRollbackTerminalFreshAdmission{
		SchemaVersion:                    LifecycleRecoveryRollbackTerminalFreshAdmissionSchemaV1,
		RevalidationGateID:               gate.RevalidationGateID,
		TerminalIntentReceiptID:          gate.TerminalIntentReceiptID,
		ReconciliationReceiptID:          gate.ReconciliationReceiptID,
		RecoveryIntentID:                 gate.RecoveryIntentID,
		PreviousFreshRollbackAttemptID:   gate.PreviousFreshRollbackAttemptID,
		PreviousRollbackAdmissionID:      gate.PreviousRollbackAdmissionID,
		NodeID:                           gate.NodeID,
		ClusterID:                        gate.ClusterID,
		FromState:                        gate.CurrentLifecycleState,
		TargetState:                      nodelifecycle.StateReady,
		TransitionType:                   nodelifecycle.TransitionDesired,
		ExpectedLifecycleGeneration:      gate.CurrentLifecycleGeneration,
		PlannedLifecycleGeneration:       gate.CurrentLifecycleGeneration + 1,
		ExpectedLifecycleResourceVersion: gate.CurrentLifecycleResourceVersion,
		ClusterGeneration:                gate.CurrentClusterGeneration,
		MembershipSnapshotID:             gate.CurrentMembershipSnapshotID,
		RevisionEvidenceID:               gate.CurrentRevisionEvidenceID,
		Revision:                         gate.CurrentRevision,
		QuorumRequired:                   gate.QuorumRequired,
		HealthyVotesObserved:             gate.HealthyVotesObserved,
		MinimumReadyNodes:                gate.MinimumReadyNodes,
		ReadyNodesObserved:               gate.ReadyNodesObserved,
		RequiredEvidence: []nodelifecycle.EvidenceCheck{
			nodelifecycle.CheckOperationCancelled,
			nodelifecycle.CheckSchedulingEnabled,
			nodelifecycle.CheckReadinessPassed,
		},
		StandaloneDowntimeBound:          gate.StandaloneDowntimeBound,
		RecoveryOnly:                     true,
		RollbackOnly:                     true,
		NewAdmissionLineage:              true,
		PreviousAdmissionReusable:        false,
		PreviousAttemptReusable:          false,
		AutomaticRetryAuthorized:         false,
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
	id, err := lifecycleRecoveryRollbackTerminalFreshAdmissionID(admission)
	if err != nil {
		return LifecycleRecoveryRollbackTerminalFreshAdmission{}, err
	}
	admission.TerminalFreshAdmissionID = id
	return admission, nil
}

func RevalidateLifecycleRecoveryRollbackTerminalFreshAdmission(
	admission LifecycleRecoveryRollbackTerminalFreshAdmission,
	request LifecycleRecoveryRollbackTerminalFreshAdmissionRequest,
) error {
	if err := validateLifecycleRecoveryRollbackTerminalFreshAdmission(admission); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, admission) {
		return fmt.Errorf(
			"%w: immutable terminal fresh admission evidence changed",
			ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackTerminalFreshAdmissionError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission, err)
}

func validateLifecycleRecoveryRollbackTerminalFreshAdmission(
	admission LifecycleRecoveryRollbackTerminalFreshAdmission,
) error {
	if admission.SchemaVersion != LifecycleRecoveryRollbackTerminalFreshAdmissionSchemaV1 ||
		admission.TerminalFreshAdmissionID == "" || admission.RevalidationGateID == "" ||
		admission.TerminalIntentReceiptID == "" || admission.ReconciliationReceiptID == "" ||
		admission.RecoveryIntentID == "" || admission.PreviousFreshRollbackAttemptID == "" ||
		admission.PreviousRollbackAdmissionID == "" || admission.NodeID == "" ||
		admission.ClusterID == "" || admission.ExpectedLifecycleGeneration == 0 ||
		admission.PlannedLifecycleGeneration == 0 || admission.ExpectedLifecycleResourceVersion == "" ||
		admission.ClusterGeneration == 0 || admission.MembershipSnapshotID == "" ||
		admission.RevisionEvidenceID == "" {
		return fmt.Errorf(
			"%w: terminal fresh admission identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if admission.TerminalFreshAdmissionID == admission.RevalidationGateID ||
		admission.TerminalFreshAdmissionID == admission.TerminalIntentReceiptID ||
		admission.TerminalFreshAdmissionID == admission.RecoveryIntentID ||
		admission.TerminalFreshAdmissionID == admission.PreviousFreshRollbackAttemptID ||
		admission.TerminalFreshAdmissionID == admission.PreviousRollbackAdmissionID {
		return fmt.Errorf(
			"%w: terminal fresh admission must use new lineage",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if admission.FromState != nodelifecycle.StateDraining ||
		admission.TargetState != nodelifecycle.StateReady ||
		admission.TransitionType != nodelifecycle.TransitionDesired ||
		admission.PlannedLifecycleGeneration != admission.ExpectedLifecycleGeneration+1 {
		return fmt.Errorf(
			"%w: terminal fresh admission lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if admission.QuorumRequired <= 0 || admission.MinimumReadyNodes != admission.QuorumRequired ||
		admission.HealthyVotesObserved < admission.QuorumRequired ||
		admission.ReadyNodesObserved < admission.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: terminal fresh admission quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if !admission.RecoveryOnly || !admission.RollbackOnly || !admission.NewAdmissionLineage ||
		admission.PreviousAdmissionReusable || admission.PreviousAttemptReusable ||
		admission.AutomaticRetryAuthorized || !admission.CASBound || !admission.SingleUseByCAS ||
		!admission.AuditedChangeJobRequired || !admission.ExecutionAuthorized ||
		!admission.LifecycleStateMutationAuthorized || admission.MembershipMutationAuthorized ||
		admission.FailoverAuthorized || admission.GenericCommandAuthorized ||
		admission.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: terminal fresh admission safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	wantEvidence := []nodelifecycle.EvidenceCheck{
		nodelifecycle.CheckOperationCancelled,
		nodelifecycle.CheckSchedulingEnabled,
		nodelifecycle.CheckReadinessPassed,
	}
	if !reflect.DeepEqual(admission.RequiredEvidence, wantEvidence) {
		return fmt.Errorf(
			"%w: terminal fresh admission required evidence changed",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	if err := validateTransitionRevision(admission.Revision); err != nil {
		return fmt.Errorf(
			"%w: revision: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
			err,
		)
	}
	for field, value := range map[string]string{
		"terminal_fresh_admission_id":         admission.TerminalFreshAdmissionID,
		"revalidation_gate_id":                admission.RevalidationGateID,
		"terminal_intent_receipt_id":          admission.TerminalIntentReceiptID,
		"reconciliation_receipt_id":           admission.ReconciliationReceiptID,
		"recovery_intent_id":                  admission.RecoveryIntentID,
		"previous_fresh_rollback_attempt_id":  admission.PreviousFreshRollbackAttemptID,
		"previous_rollback_admission_id":      admission.PreviousRollbackAdmissionID,
		"node_id":                             admission.NodeID,
		"cluster_id":                          admission.ClusterID,
		"expected_lifecycle_resource_version": admission.ExpectedLifecycleResourceVersion,
		"membership_snapshot_id":              admission.MembershipSnapshotID,
		"revision_evidence_id":                admission.RevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackTerminalFreshAdmissionID(admission)
	if err != nil {
		return err
	}
	if expectedID != admission.TerminalFreshAdmissionID {
		return fmt.Errorf(
			"%w: terminal fresh admission fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackTerminalFreshAdmissionID(
	admission LifecycleRecoveryRollbackTerminalFreshAdmission,
) (string, error) {
	input := admission
	input.TerminalFreshAdmissionID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrtfa-" + hex.EncodeToString(digest[:])[:24], nil
}
