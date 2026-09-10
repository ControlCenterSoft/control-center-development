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
	ErrInvalidLifecycleExecutionCompletion = errors.New("invalid cluster lifecycle execution completion")
	ErrStaleLifecycleExecutionCompletion   = errors.New("stale cluster lifecycle execution completion")
	ErrLifecycleExecutionRecoveryRequired  = errors.New("cluster lifecycle execution recovery required")
)

const LifecycleExecutionCompletionReceiptSchemaV1 = "clusterha.lifecycle-execution-completion-receipt/v1"

// LifecycleExecutionCompletionObservation is the bounded evidence collected
// immediately after an admitted lifecycle compare-and-swap. It contains no
// command material and cannot authorize another transition.
type LifecycleExecutionCompletionObservation struct {
	Admission         LifecycleExecutionAdmission   `json:"admission"`
	Source            LifecycleExecutionObservation `json:"source"`
	PostCAS           LifecycleCASObservation       `json:"post_cas"`
	CurrentMembership Snapshot                      `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence    `json:"-"`
}

// LifecycleExecutionCompletionReceipt proves that one exact admitted
// lifecycle CAS reached the sealed target state and generation while the HA
// lineage stayed unchanged. It is evidence only: every later lifecycle,
// membership or failover action requires a fresh safety decision.
type LifecycleExecutionCompletionReceipt struct {
	SchemaVersion                    string                       `json:"schema_version"`
	ReceiptID                        string                       `json:"receipt_id"`
	AdmissionID                      string                       `json:"admission_id"`
	HandoffID                        string                       `json:"handoff_id"`
	LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	PreviousLifecycleGeneration      uint64                       `json:"previous_lifecycle_generation"`
	AppliedLifecycleGeneration       uint64                       `json:"applied_lifecycle_generation"`
	PreviousLifecycleResourceVersion string                       `json:"previous_lifecycle_resource_version"`
	AppliedLifecycleResourceVersion  string                       `json:"applied_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	CompletionVerified               bool                         `json:"completion_verified"`
	RecoveryRequired                 bool                         `json:"recovery_required"`
	FurtherLifecycleAuthorized       bool                         `json:"further_lifecycle_authorized"`
	MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                         `json:"failover_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

func BuildLifecycleExecutionCompletionReceipt(
	observation LifecycleExecutionCompletionObservation,
) (LifecycleExecutionCompletionReceipt, error) {
	if observation.Admission.AdmissionID == "" || observation.Source.Handoff.HandoffID == "" {
		return LifecycleExecutionCompletionReceipt{}, fmt.Errorf(
			"%w: admission and source handoff are required",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	if observation.Source.Handoff.HandoffID != observation.Admission.HandoffID {
		return LifecycleExecutionCompletionReceipt{}, fmt.Errorf(
			"%w: source handoff does not match admission",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	if _, err := RevalidateLifecycleExecutionAdmission(observation.Admission, observation.Source); err != nil {
		return LifecycleExecutionCompletionReceipt{}, classifyLifecycleExecutionCompletionError(err)
	}
	if _, err := RevalidateLifecycleHandoff(
		observation.Source.Handoff,
		observation.CurrentMembership,
		observation.CurrentEvidence,
	); err != nil {
		return LifecycleExecutionCompletionReceipt{}, classifyLifecycleExecutionCompletionError(err)
	}
	if err := validateLifecycleCASObservation(observation.PostCAS); err != nil {
		return LifecycleExecutionCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS observation: %v",
			ErrInvalidLifecycleExecutionCompletion,
			err,
		)
	}
	if !lifecyclePostCASMatchesAdmission(observation.PostCAS, observation.Admission) {
		return LifecycleExecutionCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS state does not prove the exact admitted transition",
			ErrLifecycleExecutionRecoveryRequired,
		)
	}

	receipt := LifecycleExecutionCompletionReceipt{
		SchemaVersion:                    LifecycleExecutionCompletionReceiptSchemaV1,
		AdmissionID:                      observation.Admission.AdmissionID,
		HandoffID:                        observation.Admission.HandoffID,
		LifecyclePlanID:                  observation.Admission.LifecyclePlanID,
		NodeID:                           observation.Admission.NodeID,
		FromState:                        observation.Admission.FromState,
		TargetState:                      observation.Admission.TargetState,
		TransitionType:                   observation.Admission.TransitionType,
		PreviousLifecycleGeneration:      observation.Admission.ExpectedLifecycleGeneration,
		AppliedLifecycleGeneration:       observation.PostCAS.Generation,
		PreviousLifecycleResourceVersion: observation.Admission.ExpectedLifecycleResourceVersion,
		AppliedLifecycleResourceVersion:  observation.PostCAS.ResourceVersion,
		ClusterID:                        observation.Admission.ClusterID,
		ClusterGeneration:                observation.Admission.ClusterGeneration,
		MembershipSnapshotID:             observation.Admission.MembershipSnapshotID,
		RevisionEvidenceID:               observation.Admission.RevisionEvidenceID,
		Revision:                         observation.Admission.Revision,
		StandaloneDowntimeBound:          observation.Admission.StandaloneDowntimeBound,
		CompletionVerified:               true,
		RecoveryRequired:                 false,
		FurtherLifecycleAuthorized:       false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	receiptID, err := lifecycleExecutionCompletionReceiptID(receipt)
	if err != nil {
		return LifecycleExecutionCompletionReceipt{}, err
	}
	receipt.ReceiptID = receiptID
	return receipt, nil
}

// RevalidateLifecycleExecutionCompletionReceipt proves that a stored receipt
// still matches the exact post-CAS and HA evidence. It never upgrades the
// receipt into authority for another lifecycle, membership or failover action.
func RevalidateLifecycleExecutionCompletionReceipt(
	receipt LifecycleExecutionCompletionReceipt,
	observation LifecycleExecutionCompletionObservation,
) error {
	if err := validateLifecycleExecutionCompletionReceipt(receipt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleExecutionCompletionReceipt(observation)
	if err != nil {
		return err
	}
	if rebuilt != receipt {
		return fmt.Errorf(
			"%w: immutable completion evidence changed",
			ErrStaleLifecycleExecutionCompletion,
		)
	}
	return nil
}

func lifecyclePostCASMatchesAdmission(
	post LifecycleCASObservation,
	admission LifecycleExecutionAdmission,
) bool {
	return post.NodeID == admission.NodeID &&
		post.State == admission.TargetState &&
		post.Generation == admission.PlannedLifecycleGeneration &&
		post.ResourceVersion != admission.ExpectedLifecycleResourceVersion
}

func classifyLifecycleExecutionCompletionError(err error) error {
	switch {
	case errors.Is(err, ErrStaleLifecycleExecutionAdmission), errors.Is(err, ErrStaleLifecycleHandoff):
		return fmt.Errorf("%w: %v", ErrStaleLifecycleExecutionCompletion, err)
	default:
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionCompletion, err)
	}
}

func validateLifecycleExecutionCompletionReceipt(receipt LifecycleExecutionCompletionReceipt) error {
	if receipt.SchemaVersion != LifecycleExecutionCompletionReceiptSchemaV1 ||
		receipt.ReceiptID == "" || receipt.AdmissionID == "" || receipt.HandoffID == "" ||
		receipt.LifecyclePlanID == "" || receipt.NodeID == "" || receipt.ClusterID == "" ||
		receipt.ClusterGeneration == 0 || receipt.MembershipSnapshotID == "" ||
		receipt.RevisionEvidenceID == "" || receipt.PreviousLifecycleGeneration == 0 ||
		receipt.AppliedLifecycleGeneration == 0 || receipt.PreviousLifecycleResourceVersion == "" ||
		receipt.AppliedLifecycleResourceVersion == "" || !receipt.FromState.Valid() || !receipt.TargetState.Valid() {
		return fmt.Errorf(
			"%w: receipt identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	if !receipt.CompletionVerified || receipt.RecoveryRequired || receipt.FurtherLifecycleAuthorized ||
		receipt.MembershipMutationAuthorized || receipt.FailoverAuthorized ||
		receipt.GenericCommandAuthorized || receipt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: receipt safety flags are invalid",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	if receipt.AppliedLifecycleResourceVersion == receipt.PreviousLifecycleResourceVersion {
		return fmt.Errorf(
			"%w: lifecycle resource version did not advance",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	if err := validateTransitionRevision(receipt.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleExecutionCompletion, err)
	}
	for field, value := range map[string]string{
		"receipt_id":                          receipt.ReceiptID,
		"admission_id":                        receipt.AdmissionID,
		"handoff_id":                          receipt.HandoffID,
		"lifecycle_plan_id":                   receipt.LifecyclePlanID,
		"node_id":                             receipt.NodeID,
		"cluster_id":                          receipt.ClusterID,
		"membership_snapshot_id":              receipt.MembershipSnapshotID,
		"revision_evidence_id":                receipt.RevisionEvidenceID,
		"previous_lifecycle_resource_version": receipt.PreviousLifecycleResourceVersion,
		"applied_lifecycle_resource_version":  receipt.AppliedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionCompletion, err)
		}
	}
	expectedID, err := lifecycleExecutionCompletionReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReceiptID {
		return fmt.Errorf(
			"%w: receipt fingerprint mismatch",
			ErrInvalidLifecycleExecutionCompletion,
		)
	}
	return nil
}

func lifecycleExecutionCompletionReceiptID(
	receipt LifecycleExecutionCompletionReceipt,
) (string, error) {
	input := struct {
		SchemaVersion                    string                       `json:"schema_version"`
		AdmissionID                      string                       `json:"admission_id"`
		HandoffID                        string                       `json:"handoff_id"`
		LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
		NodeID                           string                       `json:"node_id"`
		FromState                        nodelifecycle.State          `json:"from_state"`
		TargetState                      nodelifecycle.State          `json:"target_state"`
		TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
		PreviousLifecycleGeneration      uint64                       `json:"previous_lifecycle_generation"`
		AppliedLifecycleGeneration       uint64                       `json:"applied_lifecycle_generation"`
		PreviousLifecycleResourceVersion string                       `json:"previous_lifecycle_resource_version"`
		AppliedLifecycleResourceVersion  string                       `json:"applied_lifecycle_resource_version"`
		ClusterID                        string                       `json:"cluster_id"`
		ClusterGeneration                uint64                       `json:"cluster_generation"`
		MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
		RevisionEvidenceID               string                       `json:"revision_evidence_id"`
		Revision                         TransitionRevision           `json:"revision"`
		StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
		CompletionVerified               bool                         `json:"completion_verified"`
		RecoveryRequired                 bool                         `json:"recovery_required"`
		FurtherLifecycleAuthorized       bool                         `json:"further_lifecycle_authorized"`
		MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
		FailoverAuthorized               bool                         `json:"failover_authorized"`
		GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
		HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
	}{
		SchemaVersion:                    receipt.SchemaVersion,
		AdmissionID:                      receipt.AdmissionID,
		HandoffID:                        receipt.HandoffID,
		LifecyclePlanID:                  receipt.LifecyclePlanID,
		NodeID:                           receipt.NodeID,
		FromState:                        receipt.FromState,
		TargetState:                      receipt.TargetState,
		TransitionType:                   receipt.TransitionType,
		PreviousLifecycleGeneration:      receipt.PreviousLifecycleGeneration,
		AppliedLifecycleGeneration:       receipt.AppliedLifecycleGeneration,
		PreviousLifecycleResourceVersion: receipt.PreviousLifecycleResourceVersion,
		AppliedLifecycleResourceVersion:  receipt.AppliedLifecycleResourceVersion,
		ClusterID:                        receipt.ClusterID,
		ClusterGeneration:                receipt.ClusterGeneration,
		MembershipSnapshotID:             receipt.MembershipSnapshotID,
		RevisionEvidenceID:               receipt.RevisionEvidenceID,
		Revision:                         receipt.Revision,
		StandaloneDowntimeBound:          receipt.StandaloneDowntimeBound,
		CompletionVerified:               receipt.CompletionVerified,
		RecoveryRequired:                 receipt.RecoveryRequired,
		FurtherLifecycleAuthorized:       receipt.FurtherLifecycleAuthorized,
		MembershipMutationAuthorized:     receipt.MembershipMutationAuthorized,
		FailoverAuthorized:               receipt.FailoverAuthorized,
		GenericCommandAuthorized:         receipt.GenericCommandAuthorized,
		HostMutationAuthorized:           receipt.HostMutationAuthorized,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: receipt fingerprint: %v",
			ErrInvalidLifecycleExecutionCompletion,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chc-" + hex.EncodeToString(digest[:])[:24], nil
}
