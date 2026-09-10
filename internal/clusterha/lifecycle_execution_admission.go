package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecycleExecutionAdmission = errors.New("invalid cluster lifecycle execution admission")
	ErrStaleLifecycleExecutionAdmission   = errors.New("stale cluster lifecycle execution admission")
)

const LifecycleExecutionAdmissionSchemaV1 = "clusterha.lifecycle-execution-admission/v1"

// LifecycleCASObservation is the bounded lifecycle storage state that an
// audited executor must compare-and-swap atomically with the admitted state
// transition. It deliberately carries no command, callback or host operation.
type LifecycleCASObservation struct {
	NodeID          string              `json:"node_id"`
	State           nodelifecycle.State `json:"state"`
	Generation      uint64              `json:"generation"`
	ResourceVersion string              `json:"resource_version"`
}

// LifecycleExecutionAdmission is a single-use-by-CAS capability for exactly
// one typed lifecycle state transition. It is valid only while both the sealed
// HA handoff and the lifecycle storage precondition remain exact. The
// capability never authorizes generic commands or direct host mutation.
type LifecycleExecutionAdmission struct {
	SchemaVersion                    string                       `json:"schema_version"`
	AdmissionID                      string                       `json:"admission_id"`
	HandoffID                        string                       `json:"handoff_id"`
	PreflightPlanID                  string                       `json:"preflight_plan_id"`
	LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	CASBound                         bool                         `json:"cas_bound"`
	SingleUseByCAS                   bool                         `json:"single_use_by_cas"`
	AuditedChangeJobRequired         bool                         `json:"audited_change_job_required"`
	ExecutionAuthorized              bool                         `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                         `json:"lifecycle_state_mutation_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

type LifecycleExecutionAdmissionRequest struct {
	Handoff           LifecycleHandoff           `json:"handoff"`
	Preflight         LifecyclePreflightPlan     `json:"preflight"`
	PreflightRequest  LifecyclePreflightRequest  `json:"preflight_request"`
	CurrentMembership Snapshot                   `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence `json:"-"`
	LifecycleCAS      LifecycleCASObservation    `json:"lifecycle_cas"`
}

// LifecycleExecutionObservation contains the exact source evidence that must be
// re-read immediately before the audited lifecycle CAS. Any lifecycle, HA
// journal, leader, health, topology or generation movement invalidates the
// admission and requires a fresh preflight/handoff.
type LifecycleExecutionObservation struct {
	Handoff           LifecycleHandoff           `json:"handoff"`
	Preflight         LifecyclePreflightPlan     `json:"preflight"`
	PreflightRequest  LifecyclePreflightRequest  `json:"preflight_request"`
	CurrentMembership Snapshot                   `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence `json:"-"`
	LifecycleCAS      LifecycleCASObservation    `json:"lifecycle_cas"`
}

type LifecycleExecutionAdmissionValidation struct {
	AdmissionID                      string                       `json:"admission_id"`
	HandoffID                        string                       `json:"handoff_id"`
	PreflightPlanID                  string                       `json:"preflight_plan_id"`
	LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
	PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
	ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	Revalidated                      bool                         `json:"revalidated"`
	CASBound                         bool                         `json:"cas_bound"`
	SingleUseByCAS                   bool                         `json:"single_use_by_cas"`
	AuditedChangeJobRequired         bool                         `json:"audited_change_job_required"`
	ExecutionAuthorized              bool                         `json:"execution_authorized"`
	LifecycleStateMutationAuthorized bool                         `json:"lifecycle_state_mutation_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

func BuildLifecycleExecutionAdmission(request LifecycleExecutionAdmissionRequest) (LifecycleExecutionAdmission, error) {
	plan, err := validateLifecycleExecutionSource(request.Handoff, request.Preflight, request.PreflightRequest)
	if err != nil {
		return LifecycleExecutionAdmission{}, err
	}
	if _, err := RevalidateLifecycleHandoff(request.Handoff, request.CurrentMembership, request.CurrentEvidence); err != nil {
		return LifecycleExecutionAdmission{}, classifyLifecycleExecutionHandoffError(err)
	}
	if err := validateLifecycleCASObservation(request.LifecycleCAS); err != nil {
		return LifecycleExecutionAdmission{}, err
	}
	if !lifecycleCASMatchesPlan(request.LifecycleCAS, plan) {
		return LifecycleExecutionAdmission{}, fmt.Errorf("%w: lifecycle CAS no longer matches the sealed transition plan", ErrStaleLifecycleExecutionAdmission)
	}

	admission := LifecycleExecutionAdmission{
		SchemaVersion:                    LifecycleExecutionAdmissionSchemaV1,
		HandoffID:                        request.Handoff.HandoffID,
		PreflightPlanID:                  request.Preflight.PlanID,
		LifecyclePlanID:                  plan.PlanID,
		NodeID:                           plan.NodeID,
		FromState:                        plan.From,
		TargetState:                      plan.To,
		TransitionType:                   plan.Type,
		ExpectedLifecycleGeneration:      plan.CurrentGeneration,
		PlannedLifecycleGeneration:       plan.PlannedGeneration,
		ExpectedLifecycleResourceVersion: plan.BasedOnResourceVersion,
		ClusterID:                        request.Handoff.ClusterID,
		ClusterGeneration:                request.Handoff.ClusterGeneration,
		MembershipSnapshotID:             request.Handoff.MembershipSnapshotID,
		RevisionEvidenceID:               request.Handoff.RevisionEvidenceID,
		Revision:                         request.Handoff.Revision,
		StandaloneDowntimeBound:          request.Handoff.StandaloneDowntimeBound,
		CASBound:                         true,
		SingleUseByCAS:                   true,
		AuditedChangeJobRequired:         true,
		ExecutionAuthorized:              true,
		LifecycleStateMutationAuthorized: true,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	admissionID, err := lifecycleExecutionAdmissionID(admission)
	if err != nil {
		return LifecycleExecutionAdmission{}, err
	}
	admission.AdmissionID = admissionID
	return admission, nil
}

// RevalidateLifecycleExecutionAdmission proves that the admission still refers
// to the exact trusted preflight/handoff and exact lifecycle CAS precondition.
// A successful result is permission only for the typed lifecycle state CAS;
// host-side actions remain outside this capability.
func RevalidateLifecycleExecutionAdmission(
	admission LifecycleExecutionAdmission,
	observed LifecycleExecutionObservation,
) (LifecycleExecutionAdmissionValidation, error) {
	if err := validateLifecycleExecutionAdmission(admission); err != nil {
		return LifecycleExecutionAdmissionValidation{}, err
	}
	plan, err := validateLifecycleExecutionSource(observed.Handoff, observed.Preflight, observed.PreflightRequest)
	if err != nil {
		return LifecycleExecutionAdmissionValidation{}, err
	}
	if !lifecycleExecutionAdmissionMatchesSource(admission, observed.Handoff, observed.Preflight, plan) {
		return LifecycleExecutionAdmissionValidation{}, fmt.Errorf("%w: immutable lifecycle source evidence changed", ErrStaleLifecycleExecutionAdmission)
	}
	if _, err := RevalidateLifecycleHandoff(observed.Handoff, observed.CurrentMembership, observed.CurrentEvidence); err != nil {
		return LifecycleExecutionAdmissionValidation{}, classifyLifecycleExecutionHandoffError(err)
	}
	if err := validateLifecycleCASObservation(observed.LifecycleCAS); err != nil {
		return LifecycleExecutionAdmissionValidation{}, err
	}
	if !lifecycleCASMatchesAdmission(observed.LifecycleCAS, admission) {
		return LifecycleExecutionAdmissionValidation{}, fmt.Errorf("%w: lifecycle CAS moved or admission was already consumed", ErrStaleLifecycleExecutionAdmission)
	}

	return LifecycleExecutionAdmissionValidation{
		AdmissionID:                      admission.AdmissionID,
		HandoffID:                        admission.HandoffID,
		PreflightPlanID:                  admission.PreflightPlanID,
		LifecyclePlanID:                  admission.LifecyclePlanID,
		NodeID:                           admission.NodeID,
		FromState:                        admission.FromState,
		TargetState:                      admission.TargetState,
		TransitionType:                   admission.TransitionType,
		ExpectedLifecycleGeneration:      admission.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       admission.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: admission.ExpectedLifecycleResourceVersion,
		ClusterID:                        admission.ClusterID,
		ClusterGeneration:                admission.ClusterGeneration,
		MembershipSnapshotID:             admission.MembershipSnapshotID,
		RevisionEvidenceID:               admission.RevisionEvidenceID,
		Revision:                         admission.Revision,
		Revalidated:                      true,
		CASBound:                         true,
		SingleUseByCAS:                   true,
		AuditedChangeJobRequired:         true,
		ExecutionAuthorized:              true,
		LifecycleStateMutationAuthorized: true,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}, nil
}

func validateLifecycleExecutionSource(
	handoff LifecycleHandoff,
	preflight LifecyclePreflightPlan,
	preflightRequest LifecyclePreflightRequest,
) (nodelifecycle.TransitionPlan, error) {
	if err := validateLifecycleHandoff(handoff); err != nil {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: handoff: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	if err := validatePreflightEvidence(preflight); err != nil {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: preflight: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	rebuilt, err := BuildLifecyclePreflight(preflightRequest)
	if err != nil {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: preflight request no longer validates: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	if !sameLifecycleHandoffPreflight(preflight, rebuilt) {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: preflight evidence does not match supplied request", ErrInvalidLifecycleExecutionAdmission)
	}
	if !preflight.ControllerBound {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: execution admission is only valid for controller-bound lifecycle", ErrInvalidLifecycleExecutionAdmission)
	}
	if handoff.PreflightPlanID != preflight.PlanID || handoff.LifecyclePlanID != preflight.LifecyclePlanID ||
		handoff.NodeID != preflight.NodeID || handoff.ClusterID != preflight.ClusterID ||
		handoff.ClusterGeneration != preflight.ClusterGeneration ||
		handoff.StandaloneDowntimeBound != preflight.RequiresStandaloneDowntime {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: handoff is bound to different lifecycle preflight evidence", ErrInvalidLifecycleExecutionAdmission)
	}

	plan := preflightRequest.LifecyclePlan
	if err := validateLifecyclePlanForExecution(plan); err != nil {
		return nodelifecycle.TransitionPlan{}, err
	}
	if plan.PlanID != preflight.LifecyclePlanID || plan.NodeID != preflight.NodeID || plan.To != preflight.LifecycleTarget {
		return nodelifecycle.TransitionPlan{}, fmt.Errorf("%w: lifecycle plan is bound to different preflight evidence", ErrInvalidLifecycleExecutionAdmission)
	}
	return plan, nil
}

func validateLifecyclePlanForExecution(plan nodelifecycle.TransitionPlan) error {
	if plan.PlanID == "" || plan.NodeID == "" || plan.BasedOnResourceVersion == "" {
		return fmt.Errorf("%w: lifecycle plan identity and CAS resource version are required", ErrInvalidLifecycleExecutionAdmission)
	}
	if !plan.Accepted || !plan.PlanOnly || plan.HostMutation || plan.StateMutation || !plan.RequiresAuditedChangeJob {
		return fmt.Errorf("%w: lifecycle plan must be accepted, mutation-free and require an audited change job", ErrInvalidLifecycleExecutionAdmission)
	}
	if !plan.From.Valid() || !plan.To.Valid() || !disruptiveLifecycleTarget(plan.To) {
		return fmt.Errorf("%w: lifecycle from/target states are invalid for HA execution admission", ErrInvalidLifecycleExecutionAdmission)
	}
	if plan.CurrentGeneration == 0 || plan.PlannedGeneration == 0 || plan.EvaluatedAt.IsZero() {
		return fmt.Errorf("%w: lifecycle generation and evaluated_at evidence are required", ErrInvalidLifecycleExecutionAdmission)
	}
	if err := validateIdentifier("lifecycle_plan_id", plan.PlanID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	if err := validateIdentifier("lifecycle_node_id", plan.NodeID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	if err := validateIdentifier("lifecycle_resource_version", plan.BasedOnResourceVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	switch plan.Type {
	case nodelifecycle.TransitionDesired:
		if plan.CurrentGeneration == math.MaxUint64 || plan.PlannedGeneration != plan.CurrentGeneration+1 {
			return fmt.Errorf("%w: desired lifecycle generation does not advance exactly once", ErrInvalidLifecycleExecutionAdmission)
		}
	case nodelifecycle.TransitionObservation:
		if plan.PlannedGeneration != plan.CurrentGeneration {
			return fmt.Errorf("%w: observed lifecycle generation must remain unchanged", ErrInvalidLifecycleExecutionAdmission)
		}
	default:
		return fmt.Errorf("%w: unsupported lifecycle transition type %q", ErrInvalidLifecycleExecutionAdmission, plan.Type)
	}
	return nil
}

func validateLifecycleCASObservation(observation LifecycleCASObservation) error {
	if observation.NodeID == "" || observation.ResourceVersion == "" || observation.Generation == 0 || !observation.State.Valid() {
		return fmt.Errorf("%w: lifecycle CAS observation is incomplete or invalid", ErrInvalidLifecycleExecutionAdmission)
	}
	if err := validateIdentifier("lifecycle_cas_node_id", observation.NodeID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	if err := validateIdentifier("lifecycle_cas_resource_version", observation.ResourceVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	return nil
}

func lifecycleCASMatchesPlan(observation LifecycleCASObservation, plan nodelifecycle.TransitionPlan) bool {
	return observation.NodeID == plan.NodeID && observation.State == plan.From &&
		observation.Generation == plan.CurrentGeneration && observation.ResourceVersion == plan.BasedOnResourceVersion
}

func lifecycleCASMatchesAdmission(observation LifecycleCASObservation, admission LifecycleExecutionAdmission) bool {
	return observation.NodeID == admission.NodeID && observation.State == admission.FromState &&
		observation.Generation == admission.ExpectedLifecycleGeneration &&
		observation.ResourceVersion == admission.ExpectedLifecycleResourceVersion
}

func lifecycleExecutionAdmissionMatchesSource(
	admission LifecycleExecutionAdmission,
	handoff LifecycleHandoff,
	preflight LifecyclePreflightPlan,
	plan nodelifecycle.TransitionPlan,
) bool {
	return admission.HandoffID == handoff.HandoffID &&
		admission.PreflightPlanID == preflight.PlanID && admission.LifecyclePlanID == plan.PlanID &&
		admission.NodeID == plan.NodeID && admission.FromState == plan.From && admission.TargetState == plan.To &&
		admission.TransitionType == plan.Type && admission.ExpectedLifecycleGeneration == plan.CurrentGeneration &&
		admission.PlannedLifecycleGeneration == plan.PlannedGeneration &&
		admission.ExpectedLifecycleResourceVersion == plan.BasedOnResourceVersion &&
		admission.ClusterID == handoff.ClusterID && admission.ClusterGeneration == handoff.ClusterGeneration &&
		admission.MembershipSnapshotID == handoff.MembershipSnapshotID &&
		admission.RevisionEvidenceID == handoff.RevisionEvidenceID && admission.Revision == handoff.Revision &&
		admission.StandaloneDowntimeBound == handoff.StandaloneDowntimeBound
}

func classifyLifecycleExecutionHandoffError(err error) error {
	if errors.Is(err, ErrStaleLifecycleHandoff) {
		return fmt.Errorf("%w: handoff: %v", ErrStaleLifecycleExecutionAdmission, err)
	}
	return fmt.Errorf("%w: handoff: %v", ErrInvalidLifecycleExecutionAdmission, err)
}

func validateLifecycleExecutionAdmission(admission LifecycleExecutionAdmission) error {
	if admission.SchemaVersion != LifecycleExecutionAdmissionSchemaV1 || admission.AdmissionID == "" ||
		admission.HandoffID == "" || admission.PreflightPlanID == "" || admission.LifecyclePlanID == "" ||
		admission.NodeID == "" || admission.ClusterID == "" || admission.ClusterGeneration == 0 ||
		admission.MembershipSnapshotID == "" || admission.RevisionEvidenceID == "" ||
		admission.ExpectedLifecycleResourceVersion == "" || admission.ExpectedLifecycleGeneration == 0 ||
		admission.PlannedLifecycleGeneration == 0 || !admission.FromState.Valid() || !admission.TargetState.Valid() {
		return fmt.Errorf("%w: admission identity or typed lifecycle boundary is incomplete", ErrInvalidLifecycleExecutionAdmission)
	}
	if !admission.CASBound || !admission.SingleUseByCAS || !admission.AuditedChangeJobRequired ||
		!admission.ExecutionAuthorized || !admission.LifecycleStateMutationAuthorized ||
		admission.GenericCommandAuthorized || admission.HostMutationAuthorized {
		return fmt.Errorf("%w: admission safety flags are invalid", ErrInvalidLifecycleExecutionAdmission)
	}
	if !disruptiveLifecycleTarget(admission.TargetState) {
		return fmt.Errorf("%w: target state is not an HA-gated lifecycle target", ErrInvalidLifecycleExecutionAdmission)
	}
	if err := validateTransitionRevision(admission.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	for field, value := range map[string]string{
		"admission_id":                        admission.AdmissionID,
		"handoff_id":                          admission.HandoffID,
		"preflight_plan_id":                   admission.PreflightPlanID,
		"lifecycle_plan_id":                   admission.LifecyclePlanID,
		"node_id":                             admission.NodeID,
		"cluster_id":                          admission.ClusterID,
		"membership_snapshot_id":              admission.MembershipSnapshotID,
		"revision_evidence_id":                admission.RevisionEvidenceID,
		"expected_lifecycle_resource_version": admission.ExpectedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleExecutionAdmission, err)
		}
	}
	if admission.TransitionType != nodelifecycle.TransitionDesired && admission.TransitionType != nodelifecycle.TransitionObservation {
		return fmt.Errorf("%w: unsupported lifecycle transition type %q", ErrInvalidLifecycleExecutionAdmission, admission.TransitionType)
	}
	if admission.TransitionType == nodelifecycle.TransitionDesired {
		if admission.ExpectedLifecycleGeneration == math.MaxUint64 ||
			admission.PlannedLifecycleGeneration != admission.ExpectedLifecycleGeneration+1 {
			return fmt.Errorf("%w: desired lifecycle generation does not advance exactly once", ErrInvalidLifecycleExecutionAdmission)
		}
	} else if admission.PlannedLifecycleGeneration != admission.ExpectedLifecycleGeneration {
		return fmt.Errorf("%w: observed lifecycle generation must remain unchanged", ErrInvalidLifecycleExecutionAdmission)
	}
	expectedID, err := lifecycleExecutionAdmissionID(admission)
	if err != nil {
		return err
	}
	if expectedID != admission.AdmissionID {
		return fmt.Errorf("%w: admission fingerprint mismatch", ErrInvalidLifecycleExecutionAdmission)
	}
	return nil
}

func lifecycleExecutionAdmissionID(admission LifecycleExecutionAdmission) (string, error) {
	input := struct {
		SchemaVersion                    string                       `json:"schema_version"`
		HandoffID                        string                       `json:"handoff_id"`
		PreflightPlanID                  string                       `json:"preflight_plan_id"`
		LifecyclePlanID                  string                       `json:"lifecycle_plan_id"`
		NodeID                           string                       `json:"node_id"`
		FromState                        nodelifecycle.State          `json:"from_state"`
		TargetState                      nodelifecycle.State          `json:"target_state"`
		TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
		ExpectedLifecycleGeneration      uint64                       `json:"expected_lifecycle_generation"`
		PlannedLifecycleGeneration       uint64                       `json:"planned_lifecycle_generation"`
		ExpectedLifecycleResourceVersion string                       `json:"expected_lifecycle_resource_version"`
		ClusterID                        string                       `json:"cluster_id"`
		ClusterGeneration                uint64                       `json:"cluster_generation"`
		MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
		RevisionEvidenceID               string                       `json:"revision_evidence_id"`
		Revision                         TransitionRevision           `json:"revision"`
		StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
		CASBound                         bool                         `json:"cas_bound"`
		SingleUseByCAS                   bool                         `json:"single_use_by_cas"`
		AuditedChangeJobRequired         bool                         `json:"audited_change_job_required"`
		ExecutionAuthorized              bool                         `json:"execution_authorized"`
		LifecycleStateMutationAuthorized bool                         `json:"lifecycle_state_mutation_authorized"`
		GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
		HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
	}{
		SchemaVersion:                    admission.SchemaVersion,
		HandoffID:                        admission.HandoffID,
		PreflightPlanID:                  admission.PreflightPlanID,
		LifecyclePlanID:                  admission.LifecyclePlanID,
		NodeID:                           admission.NodeID,
		FromState:                        admission.FromState,
		TargetState:                      admission.TargetState,
		TransitionType:                   admission.TransitionType,
		ExpectedLifecycleGeneration:      admission.ExpectedLifecycleGeneration,
		PlannedLifecycleGeneration:       admission.PlannedLifecycleGeneration,
		ExpectedLifecycleResourceVersion: admission.ExpectedLifecycleResourceVersion,
		ClusterID:                        admission.ClusterID,
		ClusterGeneration:                admission.ClusterGeneration,
		MembershipSnapshotID:             admission.MembershipSnapshotID,
		RevisionEvidenceID:               admission.RevisionEvidenceID,
		Revision:                         admission.Revision,
		StandaloneDowntimeBound:          admission.StandaloneDowntimeBound,
		CASBound:                         admission.CASBound,
		SingleUseByCAS:                   admission.SingleUseByCAS,
		AuditedChangeJobRequired:         admission.AuditedChangeJobRequired,
		ExecutionAuthorized:              admission.ExecutionAuthorized,
		LifecycleStateMutationAuthorized: admission.LifecycleStateMutationAuthorized,
		GenericCommandAuthorized:         admission.GenericCommandAuthorized,
		HostMutationAuthorized:           admission.HostMutationAuthorized,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: admission fingerprint: %v", ErrInvalidLifecycleExecutionAdmission, err)
	}
	digest := sha256.Sum256(encoded)
	return "cha-" + hex.EncodeToString(digest[:])[:24], nil
}
