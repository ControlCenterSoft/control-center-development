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
	ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate = errors.New(
		"invalid cluster lifecycle recovery rollback terminal revalidation gate",
	)
	ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate = errors.New(
		"stale cluster lifecycle recovery rollback terminal revalidation gate",
	)
	ErrLifecycleRecoveryRollbackTerminalRevalidationNotEligible = errors.New(
		"cluster lifecycle recovery rollback terminal intent is not eligible for revalidation",
	)
)

const LifecycleRecoveryRollbackTerminalRevalidationGateSchemaV1 = "clusterha.lifecycle-recovery-rollback-terminal-revalidation-gate/v1"

// LifecycleRecoveryRollbackTerminalRevalidationObservation binds one explicit
// terminal recovery intent to a fresh lifecycle and HA read. The read is
// evidence-only and cannot carry commands, role changes, membership changes or
// host operations.
type LifecycleRecoveryRollbackTerminalRevalidationObservation struct {
	IntentReceipt     LifecycleRecoveryRollbackTerminalIntentReceipt `json:"intent_receipt"`
	IntentRequest     LifecycleRecoveryRollbackTerminalIntentRequest `json:"intent_request"`
	CurrentCAS        LifecycleCASObservation                        `json:"current_cas"`
	CurrentMembership Snapshot                                       `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                     `json:"-"`
}

// LifecycleRecoveryRollbackTerminalRevalidationGate proves that an explicitly
// requested post-terminal revalidation still observes the exact safe
// lifecycle, membership and HA revision boundary. It only permits construction
// of a later admission with new lineage; it is not itself an admission.
type LifecycleRecoveryRollbackTerminalRevalidationGate struct {
	SchemaVersion                    string              `json:"schema_version"`
	RevalidationGateID               string              `json:"revalidation_gate_id"`
	TerminalIntentReceiptID          string              `json:"terminal_intent_receipt_id"`
	ReconciliationReceiptID          string              `json:"reconciliation_receipt_id"`
	RecoveryIntentID                 string              `json:"recovery_intent_id"`
	PreviousFreshRollbackAttemptID   string              `json:"previous_fresh_rollback_attempt_id"`
	PreviousRollbackAdmissionID      string              `json:"previous_rollback_admission_id"`
	NodeID                           string              `json:"node_id"`
	ClusterID                        string              `json:"cluster_id"`
	CurrentLifecycleState            nodelifecycle.State `json:"current_lifecycle_state"`
	CurrentLifecycleGeneration       uint64              `json:"current_lifecycle_generation"`
	CurrentLifecycleResourceVersion  string              `json:"current_lifecycle_resource_version"`
	CurrentClusterGeneration         uint64              `json:"current_cluster_generation"`
	CurrentMembershipSnapshotID      string              `json:"current_membership_snapshot_id"`
	CurrentRevisionEvidenceID        string              `json:"current_revision_evidence_id"`
	CurrentRevision                  TransitionRevision  `json:"current_revision"`
	QuorumRequired                   int                 `json:"quorum_required"`
	HealthyVotesObserved             int                 `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                 `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                 `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                `json:"standalone_downtime_bound"`
	SafetyBoundarySatisfied          bool                `json:"safety_boundary_satisfied"`
	FreshAdmissionEligible           bool                `json:"fresh_admission_eligible"`
	RequiresNewAdmissionLineage      bool                `json:"requires_new_admission_lineage"`
	PreviousAdmissionReusable        bool                `json:"previous_admission_reusable"`
	FreshAdmissionAuthorized         bool                `json:"fresh_admission_authorized"`
	FurtherAttemptAuthorized         bool                `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized         bool                `json:"automatic_retry_authorized"`
	LifecycleStateMutationAuthorized bool                `json:"lifecycle_state_mutation_authorized"`
	MembershipMutationAuthorized     bool                `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                `json:"failover_authorized"`
	GenericCommandAuthorized         bool                `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                `json:"host_mutation_authorized"`
}

func BuildLifecycleRecoveryRollbackTerminalRevalidationGate(
	observation LifecycleRecoveryRollbackTerminalRevalidationObservation,
) (LifecycleRecoveryRollbackTerminalRevalidationGate, error) {
	if err := RevalidateLifecycleRecoveryRollbackTerminalIntentReceipt(
		observation.IntentReceipt,
		observation.IntentRequest,
	); err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{},
			classifyLifecycleRecoveryRollbackTerminalRevalidationGateError(err)
	}
	intent := observation.IntentReceipt
	if intent.Action != LifecycleRecoveryRollbackTerminalIntentRevalidate ||
		!intent.FreshAdmissionRequested || intent.RecoveryClosureRequested ||
		!intent.SourceSafetyBoundarySatisfied ||
		!observation.IntentRequest.ReconciliationReceipt.SafetyBoundarySatisfied {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{},
			ErrLifecycleRecoveryRollbackTerminalRevalidationNotEligible
	}
	if err := validateLifecycleCASObservation(observation.CurrentCAS); err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: current lifecycle CAS: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}

	source := observation.IntentRequest.ReconciliationReceipt
	if observation.CurrentCAS.NodeID != intent.NodeID ||
		observation.CurrentCAS.State != source.ObservedLifecycleState ||
		observation.CurrentCAS.Generation != source.ObservedLifecycleGeneration ||
		observation.CurrentCAS.ResourceVersion != source.ObservedLifecycleResourceVersion {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: lifecycle CAS moved after terminal intent",
			ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}

	profile, members, err := validateSnapshot(observation.CurrentMembership)
	if err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: current membership: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}
	member, exists := members[intent.NodeID]
	if !exists || member.Kind != MemberController || !memberReady(member) {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: rollback target is not a current ready controller",
			ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}
	if err := validateTransitionRevisionEvidence(observation.CurrentEvidence); err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: current HA revision evidence: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}
	snapshotID, err := membershipSnapshotID(observation.CurrentMembership)
	if err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: current membership fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}
	if observation.CurrentMembership.ClusterID != intent.ClusterID ||
		observation.CurrentMembership.Generation != intent.CurrentClusterGeneration ||
		snapshotID != intent.CurrentMembershipSnapshotID ||
		observation.CurrentEvidence.ClusterID() != intent.ClusterID ||
		observation.CurrentEvidence.Generation() != intent.CurrentClusterGeneration ||
		observation.CurrentEvidence.MembershipSnapshotID() != snapshotID ||
		observation.CurrentEvidence.LeaderID() != observation.CurrentMembership.LeaderID ||
		observation.CurrentEvidence.EvidenceID() != intent.CurrentRevisionEvidenceID {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: membership or HA revision moved after terminal intent",
			ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}

	currentQuorum := quorum(len(observation.CurrentMembership.Members))
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	if currentQuorum != intent.CurrentQuorumRequired ||
		currentHealthy < currentQuorum || currentReady < currentQuorum {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, fmt.Errorf(
			"%w: quorum or minimum-ready boundary is no longer satisfied",
			ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}

	gate := LifecycleRecoveryRollbackTerminalRevalidationGate{
		SchemaVersion:                    LifecycleRecoveryRollbackTerminalRevalidationGateSchemaV1,
		TerminalIntentReceiptID:          intent.TerminalIntentReceiptID,
		ReconciliationReceiptID:          intent.ReconciliationReceiptID,
		RecoveryIntentID:                 intent.RecoveryIntentID,
		PreviousFreshRollbackAttemptID:   intent.FreshRollbackAttemptID,
		PreviousRollbackAdmissionID:      intent.PreviousRollbackAdmissionID,
		NodeID:                           intent.NodeID,
		ClusterID:                        intent.ClusterID,
		CurrentLifecycleState:            observation.CurrentCAS.State,
		CurrentLifecycleGeneration:       observation.CurrentCAS.Generation,
		CurrentLifecycleResourceVersion:  observation.CurrentCAS.ResourceVersion,
		CurrentClusterGeneration:         observation.CurrentMembership.Generation,
		CurrentMembershipSnapshotID:      snapshotID,
		CurrentRevisionEvidenceID:        observation.CurrentEvidence.EvidenceID(),
		CurrentRevision:                  observation.CurrentEvidence.Revision(),
		QuorumRequired:                   currentQuorum,
		HealthyVotesObserved:             currentHealthy,
		MinimumReadyNodes:                currentQuorum,
		ReadyNodesObserved:               currentReady,
		StandaloneDowntimeBound:          profile == ProfileStandalone,
		SafetyBoundarySatisfied:          true,
		FreshAdmissionEligible:           true,
		RequiresNewAdmissionLineage:      true,
		PreviousAdmissionReusable:        false,
		FreshAdmissionAuthorized:         false,
		FurtherAttemptAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		LifecycleStateMutationAuthorized: false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackTerminalRevalidationGateID(gate)
	if err != nil {
		return LifecycleRecoveryRollbackTerminalRevalidationGate{}, err
	}
	gate.RevalidationGateID = id
	return gate, nil
}

func RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate(
	gate LifecycleRecoveryRollbackTerminalRevalidationGate,
	observation LifecycleRecoveryRollbackTerminalRevalidationObservation,
) error {
	if err := validateLifecycleRecoveryRollbackTerminalRevalidationGate(gate); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(observation)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, gate) {
		return fmt.Errorf(
			"%w: immutable terminal revalidation evidence changed",
			ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}
	return nil
}

func validateLifecycleRecoveryRollbackTerminalRevalidationGate(
	gate LifecycleRecoveryRollbackTerminalRevalidationGate,
) error {
	if gate.SchemaVersion != LifecycleRecoveryRollbackTerminalRevalidationGateSchemaV1 ||
		gate.RevalidationGateID == "" || gate.TerminalIntentReceiptID == "" ||
		gate.ReconciliationReceiptID == "" || gate.RecoveryIntentID == "" ||
		gate.PreviousFreshRollbackAttemptID == "" || gate.PreviousRollbackAdmissionID == "" ||
		gate.NodeID == "" || gate.ClusterID == "" || gate.CurrentLifecycleGeneration == 0 ||
		gate.CurrentLifecycleResourceVersion == "" || gate.CurrentClusterGeneration == 0 ||
		gate.CurrentMembershipSnapshotID == "" || gate.CurrentRevisionEvidenceID == "" {
		return fmt.Errorf(
			"%w: terminal revalidation identity or evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}
	if gate.QuorumRequired <= 0 || gate.MinimumReadyNodes != gate.QuorumRequired ||
		gate.HealthyVotesObserved < gate.QuorumRequired ||
		gate.ReadyNodesObserved < gate.MinimumReadyNodes ||
		!gate.SafetyBoundarySatisfied || !gate.FreshAdmissionEligible ||
		!gate.RequiresNewAdmissionLineage || gate.PreviousAdmissionReusable ||
		gate.FreshAdmissionAuthorized || gate.FurtherAttemptAuthorized ||
		gate.AutomaticRetryAuthorized || gate.LifecycleStateMutationAuthorized ||
		gate.MembershipMutationAuthorized || gate.FailoverAuthorized ||
		gate.GenericCommandAuthorized || gate.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: terminal revalidation safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}
	if err := validateTransitionRevision(gate.CurrentRevision); err != nil {
		return fmt.Errorf(
			"%w: current revision: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}
	for field, value := range map[string]string{
		"revalidation_gate_id":               gate.RevalidationGateID,
		"terminal_intent_receipt_id":         gate.TerminalIntentReceiptID,
		"reconciliation_receipt_id":          gate.ReconciliationReceiptID,
		"recovery_intent_id":                 gate.RecoveryIntentID,
		"previous_fresh_rollback_attempt_id": gate.PreviousFreshRollbackAttemptID,
		"previous_rollback_admission_id":     gate.PreviousRollbackAdmissionID,
		"node_id":                            gate.NodeID,
		"cluster_id":                         gate.ClusterID,
		"current_lifecycle_resource_version": gate.CurrentLifecycleResourceVersion,
		"current_membership_snapshot_id":     gate.CurrentMembershipSnapshotID,
		"current_revision_evidence_id":       gate.CurrentRevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf(
				"%w: %v",
				ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
				err,
			)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackTerminalRevalidationGateID(gate)
	if err != nil {
		return err
	}
	if expectedID != gate.RevalidationGateID {
		return fmt.Errorf(
			"%w: terminal revalidation fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
		)
	}
	return nil
}

func classifyLifecycleRecoveryRollbackTerminalRevalidationGateError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalIntent) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttemptReconciliation) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate, err)
}

func lifecycleRecoveryRollbackTerminalRevalidationGateID(
	gate LifecycleRecoveryRollbackTerminalRevalidationGate,
) (string, error) {
	input := gate
	input.RevalidationGateID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackTerminalRevalidationGate,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrtrg-" + hex.EncodeToString(digest[:])[:24], nil
}
