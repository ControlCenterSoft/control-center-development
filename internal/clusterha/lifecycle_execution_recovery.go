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
	ErrInvalidLifecycleExecutionRecovery     = errors.New("invalid cluster lifecycle execution recovery handoff")
	ErrStaleLifecycleExecutionRecovery       = errors.New("stale cluster lifecycle execution recovery handoff")
	ErrLifecycleExecutionRecoveryNotRequired = errors.New("cluster lifecycle execution recovery is not required")
)

const LifecycleExecutionRecoveryHandoffSchemaV1 = "clusterha.lifecycle-execution-recovery-handoff/v1"

// LifecycleExecutionRecoveryObservation binds one failed or ambiguous lifecycle
// CAS outcome to the exact admission and fresh HA lineage. It carries no command
// material and cannot authorize a retry, failover, membership change or host
// mutation.
type LifecycleExecutionRecoveryObservation struct {
	Admission         LifecycleExecutionAdmission   `json:"admission"`
	Source            LifecycleExecutionObservation `json:"source"`
	ObservedCAS       LifecycleCASObservation       `json:"observed_cas"`
	CurrentMembership Snapshot                      `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence    `json:"-"`
}

// LifecycleExecutionRecoveryHandoff is immutable evidence for recovery
// assessment after an admitted lifecycle CAS did not yield a provable exact
// completion. RollbackPlanningRequired is evidence, not execution authority.
type LifecycleExecutionRecoveryHandoff struct {
	SchemaVersion                    string                       `json:"schema_version"`
	RecoveryHandoffID                string                       `json:"recovery_handoff_id"`
	AdmissionID                      string                       `json:"admission_id"`
	LifecycleHandoffID               string                       `json:"lifecycle_handoff_id"`
	LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
	ObservedLifecycleState           nodelifecycle.State          `json:"observed_lifecycle_state"`
	ObservedLifecycleGeneration      uint64                       `json:"observed_lifecycle_generation"`
	ObservedLifecycleResourceVersion string                       `json:"observed_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	RecoveryAssessmentRequired       bool                         `json:"recovery_assessment_required"`
	RollbackPlanningRequired         bool                         `json:"rollback_planning_required"`
	RecoveryExecutionAuthorized      bool                         `json:"recovery_execution_authorized"`
	FurtherLifecycleAuthorized       bool                         `json:"further_lifecycle_authorized"`
	MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                         `json:"failover_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

// BuildLifecycleExecutionRecoveryHandoff seals fail-closed recovery evidence.
// The original admission is revalidated against its immutable source while the
// HA handoff is separately revalidated against the current membership/journal
// lineage. A fully completed transition is rejected because it belongs to the
// completion-receipt path instead.
func BuildLifecycleExecutionRecoveryHandoff(
	observation LifecycleExecutionRecoveryObservation,
) (LifecycleExecutionRecoveryHandoff, error) {
	if observation.Admission.AdmissionID == "" || observation.Source.Handoff.HandoffID == "" {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: admission and source handoff are required",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if observation.Source.Handoff.HandoffID != observation.Admission.HandoffID {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: source handoff does not match admission",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if _, err := RevalidateLifecycleExecutionAdmission(observation.Admission, observation.Source); err != nil {
		return LifecycleExecutionRecoveryHandoff{}, classifyLifecycleExecutionRecoveryError(err)
	}
	if _, err := RevalidateLifecycleHandoff(
		observation.Source.Handoff,
		observation.CurrentMembership,
		observation.CurrentEvidence,
	); err != nil {
		return LifecycleExecutionRecoveryHandoff{}, classifyLifecycleExecutionRecoveryError(err)
	}
	if err := validateLifecycleCASObservation(observation.ObservedCAS); err != nil {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: observed lifecycle CAS: %v",
			ErrInvalidLifecycleExecutionRecovery,
			err,
		)
	}
	if observation.ObservedCAS.NodeID != observation.Admission.NodeID {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: observed lifecycle node changed",
			ErrStaleLifecycleExecutionRecovery,
		)
	}
	if observation.ObservedCAS.State != observation.Admission.FromState &&
		observation.ObservedCAS.State != observation.Admission.TargetState {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: observed lifecycle state escaped admitted boundary",
			ErrStaleLifecycleExecutionRecovery,
		)
	}
	if observation.ObservedCAS.Generation != observation.Admission.ExpectedLifecycleGeneration &&
		observation.ObservedCAS.Generation != observation.Admission.PlannedLifecycleGeneration {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: observed lifecycle generation escaped admitted boundary",
			ErrStaleLifecycleExecutionRecovery,
		)
	}
	if lifecyclePostCASMatchesAdmission(observation.ObservedCAS, observation.Admission) {
		return LifecycleExecutionRecoveryHandoff{}, fmt.Errorf(
			"%w: exact completion is already provable",
			ErrLifecycleExecutionRecoveryNotRequired,
		)
	}

	rollbackPlanningRequired := !lifecycleCASMatchesAdmission(observation.ObservedCAS, observation.Admission)
	handoff := LifecycleExecutionRecoveryHandoff{
		SchemaVersion:                    LifecycleExecutionRecoveryHandoffSchemaV1,
		AdmissionID:                      observation.Admission.AdmissionID,
		LifecycleHandoffID:               observation.Admission.HandoffID,
		LifecyclePlanID:                  observation.Admission.LifecyclePlanID,
		NodeID:                           observation.Admission.NodeID,
		FromState:                        observation.Admission.FromState,
		TargetState:                      observation.Admission.TargetState,
		TransitionType:                   observation.Admission.TransitionType,
		ExpectedLifecycleGeneration:      observation.Admission.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       observation.Admission.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: observation.Admission.ExpectedLifecycleResourceVersion,
		ObservedLifecycleState:           observation.ObservedCAS.State,
		ObservedLifecycleGeneration:      observation.ObservedCAS.Generation,
		ObservedLifecycleResourceVersion: observation.ObservedCAS.ResourceVersion,
		ClusterID:                        observation.Admission.ClusterID,
		ClusterGeneration:                observation.Admission.ClusterGeneration,
		MembershipSnapshotID:             observation.Admission.MembershipSnapshotID,
		RevisionEvidenceID:               observation.Admission.RevisionEvidenceID,
		Revision:                         observation.Admission.Revision,
		StandaloneDowntimeBound:          observation.Admission.StandaloneDowntimeBound,
		RecoveryAssessmentRequired:       true,
		RollbackPlanningRequired:         rollbackPlanningRequired,
		RecoveryExecutionAuthorized:      false,
		FurtherLifecycleAuthorized:       false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	handoffID, err := lifecycleExecutionRecoveryHandoffID(handoff)
	if err != nil {
		return LifecycleExecutionRecoveryHandoff{}, err
	}
	handoff.RecoveryHandoffID = handoffID
	return handoff, nil
}

// RevalidateLifecycleExecutionRecoveryHandoff proves that stored recovery
// evidence still matches the same observed lifecycle state and current HA
// lineage. It never upgrades recovery evidence into mutation authority.
func RevalidateLifecycleExecutionRecoveryHandoff(
	handoff LifecycleExecutionRecoveryHandoff,
	observation LifecycleExecutionRecoveryObservation,
) error {
	if err := validateLifecycleExecutionRecoveryHandoff(handoff); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleExecutionRecoveryHandoff(observation)
	if err != nil {
		return err
	}
	if rebuilt != handoff {
		return fmt.Errorf(
			"%w: immutable recovery evidence changed",
			ErrStaleLifecycleExecutionRecovery,
		)
	}
	return nil
}

func classifyLifecycleExecutionRecoveryError(err error) error {
	switch {
	case errors.Is(err, ErrStaleLifecycleExecutionAdmission),
		errors.Is(err, ErrStaleLifecycleHandoff):
		return fmt.Errorf("%w: %v", ErrStaleLifecycleExecutionRecovery, err)
	default:
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionRecovery, err)
	}
}

func validateLifecycleExecutionRecoveryHandoff(handoff LifecycleExecutionRecoveryHandoff) error {
	if handoff.SchemaVersion != LifecycleExecutionRecoveryHandoffSchemaV1 ||
		handoff.RecoveryHandoffID == "" || handoff.AdmissionID == "" ||
		handoff.LifecycleHandoffID == "" || handoff.LifecyclePlanID == "" || handoff.NodeID == "" ||
		handoff.ClusterID == "" || handoff.ClusterGeneration == 0 || handoff.MembershipSnapshotID == "" ||
		handoff.RevisionEvidenceID == "" || handoff.ExpectedLifecycleGeneration == 0 ||
		handoff.PlannedLifecycleGeneration == 0 || handoff.ExpectedLifecycleResourceVersion == "" ||
		handoff.ObservedLifecycleGeneration == 0 || handoff.ObservedLifecycleResourceVersion == "" ||
		!handoff.FromState.Valid() || !handoff.TargetState.Valid() || !handoff.ObservedLifecycleState.Valid() {
		return fmt.Errorf(
			"%w: recovery identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if !handoff.RecoveryAssessmentRequired || handoff.RecoveryExecutionAuthorized ||
		handoff.FurtherLifecycleAuthorized || handoff.MembershipMutationAuthorized || handoff.FailoverAuthorized ||
		handoff.GenericCommandAuthorized || handoff.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: recovery safety flags are invalid",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if handoff.ObservedLifecycleState != handoff.FromState && handoff.ObservedLifecycleState != handoff.TargetState {
		return fmt.Errorf(
			"%w: observed lifecycle state escaped admitted boundary",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if handoff.ObservedLifecycleGeneration != handoff.ExpectedLifecycleGeneration &&
		handoff.ObservedLifecycleGeneration != handoff.PlannedLifecycleGeneration {
		return fmt.Errorf(
			"%w: observed lifecycle generation escaped admitted boundary",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	exactOriginal := handoff.ObservedLifecycleState == handoff.FromState &&
		handoff.ObservedLifecycleGeneration == handoff.ExpectedLifecycleGeneration &&
		handoff.ObservedLifecycleResourceVersion == handoff.ExpectedLifecycleResourceVersion
	if handoff.RollbackPlanningRequired == exactOriginal {
		return fmt.Errorf(
			"%w: rollback planning requirement does not match observed lifecycle movement",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	exactCompleted := handoff.ObservedLifecycleState == handoff.TargetState &&
		handoff.ObservedLifecycleGeneration == handoff.PlannedLifecycleGeneration &&
		handoff.ObservedLifecycleResourceVersion != handoff.ExpectedLifecycleResourceVersion
	if exactCompleted {
		return fmt.Errorf(
			"%w: exact completion cannot be represented as recovery evidence",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	if err := validateTransitionRevision(handoff.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleExecutionRecovery, err)
	}
	for field, value := range map[string]string{
		"recovery_handoff_id":                 handoff.RecoveryHandoffID,
		"admission_id":                        handoff.AdmissionID,
		"lifecycle_handoff_id":                handoff.LifecycleHandoffID,
		"lifecycle_plan_id":                   handoff.LifecyclePlanID,
		"node_id":                             handoff.NodeID,
		"cluster_id":                          handoff.ClusterID,
		"membership_snapshot_id":              handoff.MembershipSnapshotID,
		"revision_evidence_id":                handoff.RevisionEvidenceID,
		"expected_lifecycle_resource_version": handoff.ExpectedLifecycleResourceVersion,
		"observed_lifecycle_resource_version": handoff.ObservedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionRecovery, err)
		}
	}
	expectedID, err := lifecycleExecutionRecoveryHandoffID(handoff)
	if err != nil {
		return err
	}
	if expectedID != handoff.RecoveryHandoffID {
		return fmt.Errorf(
			"%w: recovery handoff fingerprint mismatch",
			ErrInvalidLifecycleExecutionRecovery,
		)
	}
	return nil
}

func lifecycleExecutionRecoveryHandoffID(handoff LifecycleExecutionRecoveryHandoff) (string, error) {
	input := struct {
		SchemaVersion                    string                       `json:"schema_version"`
		AdmissionID                      string                       `json:"admission_id"`
		LifecycleHandoffID               string                       `json:"lifecycle_handoff_id"`
		LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
		NodeID                           string                       `json:"node_id"`
		FromState                        nodelifecycle.State          `json:"from_state"`
		TargetState                      nodelifecycle.State          `json:"target_state"`
		TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
		ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
		PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
		ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
		ObservedLifecycleState           nodelifecycle.State          `json:"observed_lifecycle_state"`
		ObservedLifecycleGeneration      uint64                       `json:"observed_lifecycle_generation"`
		ObservedLifecycleResourceVersion string                       `json:"observed_lifecycle_resource_version"`
		ClusterID                        string                       `json:"cluster_id"`
		ClusterGeneration                uint64                       `json:"cluster_generation"`
		MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
		RevisionEvidenceID               string                       `json:"revision_evidence_id"`
		Revision                         TransitionRevision           `json:"revision"`
		StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
		RecoveryAssessmentRequired       bool                         `json:"recovery_assessment_required"`
		RollbackPlanningRequired         bool                         `json:"rollback_planning_required"`
		RecoveryExecutionAuthorized      bool                         `json:"recovery_execution_authorized"`
		FurtherLifecycleAuthorized       bool                         `json:"further_lifecycle_authorized"`
		MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
		FailoverAuthorized               bool                         `json:"failover_authorized"`
		GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
		HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
	}{
		SchemaVersion:                    handoff.SchemaVersion,
		AdmissionID:                      handoff.AdmissionID,
		LifecycleHandoffID:               handoff.LifecycleHandoffID,
		LifecyclePlanID:                  handoff.LifecyclePlanID,
		NodeID:                           handoff.NodeID,
		FromState:                        handoff.FromState,
		TargetState:                      handoff.TargetState,
		TransitionType:                   handoff.TransitionType,
		ExpectedLifecycleGeneration:      handoff.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       handoff.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: handoff.ExpectedLifecycleResourceVersion,
		ObservedLifecycleState:           handoff.ObservedLifecycleState,
		ObservedLifecycleGeneration:      handoff.ObservedLifecycleGeneration,
		ObservedLifecycleResourceVersion: handoff.ObservedLifecycleResourceVersion,
		ClusterID:                        handoff.ClusterID,
		ClusterGeneration:                handoff.ClusterGeneration,
		MembershipSnapshotID:             handoff.MembershipSnapshotID,
		RevisionEvidenceID:               handoff.RevisionEvidenceID,
		Revision:                         handoff.Revision,
		StandaloneDowntimeBound:          handoff.StandaloneDowntimeBound,
		RecoveryAssessmentRequired:       handoff.RecoveryAssessmentRequired,
		RollbackPlanningRequired:         handoff.RollbackPlanningRequired,
		RecoveryExecutionAuthorized:      handoff.RecoveryExecutionAuthorized,
		FurtherLifecycleAuthorized:       handoff.FurtherLifecycleAuthorized,
		MembershipMutationAuthorized:     handoff.MembershipMutationAuthorized,
		FailoverAuthorized:               handoff.FailoverAuthorized,
		GenericCommandAuthorized:         handoff.GenericCommandAuthorized,
		HostMutationAuthorized:           handoff.HostMutationAuthorized,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: recovery handoff fingerprint: %v",
			ErrInvalidLifecycleExecutionRecovery,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chr-" + hex.EncodeToString(digest[:])[:24], nil
}
