package clusterha

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/nodelifecycle"
)

func executableLifecyclePlan(plan nodelifecycle.TransitionPlan) nodelifecycle.TransitionPlan {
	plan.RequiresAuditedChangeJob = true
	plan.From = nodelifecycle.StateReady
	plan.Type = nodelifecycle.TransitionDesired
	plan.CurrentGeneration = 7
	plan.PlannedGeneration = 8
	plan.BasedOnResourceVersion = "rv-lifecycle-7"
	plan.EvaluatedAt = time.Date(2026, 9, 10, 5, 0, 0, 0, time.UTC)
	return plan
}

func lifecycleExecutionAdmissionFixture(
	t *testing.T,
	standaloneMode bool,
) (LifecycleExecutionAdmission, LifecycleExecutionObservation) {
	t.Helper()
	nodeID := "controller-b"
	if standaloneMode {
		nodeID = "controller-a"
	}
	request := preflightRequest(nodeID, nodelifecycle.StateDraining)
	request.LifecyclePlan = executableLifecyclePlan(request.LifecyclePlan)
	if standaloneMode {
		request.Membership = standalone()
		request.StandaloneDowntimeAcknowledged = true
	}
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-ha-admission-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	handoff, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: request.Membership,
		CurrentEvidence:   evidence,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleHandoff() error = %v", err)
	}
	cas := LifecycleCASObservation{
		NodeID:          request.LifecyclePlan.NodeID,
		State:           request.LifecyclePlan.From,
		Generation:      request.LifecyclePlan.CurrentGeneration,
		ResourceVersion: request.LifecyclePlan.BasedOnResourceVersion,
	}
	admission, err := BuildLifecycleExecutionAdmission(LifecycleExecutionAdmissionRequest{
		Handoff:           handoff,
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: request.Membership,
		CurrentEvidence:   evidence,
		LifecycleCAS:      cas,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionAdmission() error = %v", err)
	}
	return admission, LifecycleExecutionObservation{
		Handoff:           handoff,
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: request.Membership,
		CurrentEvidence:   evidence,
		LifecycleCAS:      cas,
	}
}

func TestBuildLifecycleExecutionAdmissionIsDeterministicAndBounded(t *testing.T) {
	first, observed := lifecycleExecutionAdmissionFixture(t, false)
	second, err := BuildLifecycleExecutionAdmission(LifecycleExecutionAdmissionRequest{
		Handoff:           observed.Handoff,
		Preflight:         observed.Preflight,
		PreflightRequest:  observed.PreflightRequest,
		CurrentMembership: observed.CurrentMembership,
		CurrentEvidence:   observed.CurrentEvidence,
		LifecycleCAS:      observed.LifecycleCAS,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionAdmission(second) error = %v", err)
	}
	if first.AdmissionID != second.AdmissionID {
		t.Fatalf("admission ID is not deterministic: %s != %s", first.AdmissionID, second.AdmissionID)
	}
	if first.TargetState != nodelifecycle.StateDraining || first.FromState != nodelifecycle.StateReady {
		t.Fatalf("typed lifecycle boundary changed: %+v", first)
	}
	if !first.CASBound || !first.SingleUseByCAS || !first.AuditedChangeJobRequired || !first.ExecutionAuthorized ||
		!first.LifecycleStateMutationAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("unsafe admission flags: %+v", first)
	}
}

func TestRevalidateLifecycleExecutionAdmissionAllowsExactCurrentEvidence(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	validation, err := RevalidateLifecycleExecutionAdmission(admission, observed)
	if err != nil {
		t.Fatalf("RevalidateLifecycleExecutionAdmission() error = %v", err)
	}
	if !validation.Revalidated || !validation.CASBound || !validation.SingleUseByCAS ||
		!validation.ExecutionAuthorized || !validation.LifecycleStateMutationAuthorized ||
		validation.GenericCommandAuthorized || validation.HostMutationAuthorized {
		t.Fatalf("unexpected validation: %+v", validation)
	}
	if validation.AdmissionID != admission.AdmissionID || validation.HandoffID != admission.HandoffID {
		t.Fatalf("validation lost admission lineage: %+v", validation)
	}
}

func TestBuildLifecycleExecutionAdmissionRejectsLifecycleCASDrift(t *testing.T) {
	_, observed := lifecycleExecutionAdmissionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "state", mutate: func(cas *LifecycleCASObservation) { cas.State = nodelifecycle.StateDegraded }},
		{name: "generation", mutate: func(cas *LifecycleCASObservation) { cas.Generation++ }},
		{name: "resource-version", mutate: func(cas *LifecycleCASObservation) { cas.ResourceVersion = "rv-lifecycle-8" }},
		{name: "node", mutate: func(cas *LifecycleCASObservation) { cas.NodeID = "controller-c" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cas := observed.LifecycleCAS
			test.mutate(&cas)
			_, err := BuildLifecycleExecutionAdmission(LifecycleExecutionAdmissionRequest{
				Handoff:           observed.Handoff,
				Preflight:         observed.Preflight,
				PreflightRequest:  observed.PreflightRequest,
				CurrentMembership: observed.CurrentMembership,
				CurrentEvidence:   observed.CurrentEvidence,
				LifecycleCAS:      cas,
			})
			if !errors.Is(err, ErrStaleLifecycleExecutionAdmission) {
				t.Fatalf("error = %v, want ErrStaleLifecycleExecutionAdmission", err)
			}
		})
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsConsumedCAS(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	observed.LifecycleCAS.State = admission.TargetState
	observed.LifecycleCAS.Generation = admission.PlannedLifecycleGeneration
	observed.LifecycleCAS.ResourceVersion = "rv-lifecycle-8"
	_, err := RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrStaleLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionAdmission", err)
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsLaterHAJournalAdvance(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observed.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observed.CurrentMembership,
		ExpectedResourceVersion: observed.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-admission-101",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observed.CurrentEvidence = advanced
	_, err = RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrStaleLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionAdmission", err)
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsMembershipDrift(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	changed := observed.CurrentMembership
	changed.Members = append([]Member(nil), changed.Members...)
	changed.Members[2].Healthy = false
	changed.Members[2].CaughtUp = false
	advanced, err := AdvanceTransitionRevisionEvidence(observed.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              changed,
		ExpectedResourceVersion: observed.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-admission-102",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observed.CurrentMembership = changed
	observed.CurrentEvidence = advanced
	_, err = RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrStaleLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionAdmission", err)
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsSourcePlanDrift(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	observed.PreflightRequest.LifecyclePlan.CurrentGeneration++
	observed.PreflightRequest.LifecyclePlan.PlannedGeneration++
	_, err := RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrStaleLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionAdmission", err)
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsTargetTampering(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	observed.PreflightRequest.LifecyclePlan.To = nodelifecycle.StateMaintenance
	_, err := RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrInvalidLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleExecutionAdmission", err)
	}
}

func TestRevalidateLifecycleExecutionAdmissionRejectsAdmissionTampering(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, false)
	admission.GenericCommandAuthorized = true
	_, err := RevalidateLifecycleExecutionAdmission(admission, observed)
	if !errors.Is(err, ErrInvalidLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleExecutionAdmission", err)
	}
}

func TestBuildLifecycleExecutionAdmissionPreservesStandaloneDowntimeBoundary(t *testing.T) {
	admission, observed := lifecycleExecutionAdmissionFixture(t, true)
	if !admission.StandaloneDowntimeBound || admission.HostMutationAuthorized {
		t.Fatalf("standalone downtime boundary was not preserved: %+v", admission)
	}
	if _, err := RevalidateLifecycleExecutionAdmission(admission, observed); err != nil {
		t.Fatalf("RevalidateLifecycleExecutionAdmission() error = %v", err)
	}
}

func TestBuildLifecycleExecutionAdmissionRejectsNonAuditedLifecyclePlan(t *testing.T) {
	_, observed := lifecycleExecutionAdmissionFixture(t, false)
	observed.PreflightRequest.LifecyclePlan.RequiresAuditedChangeJob = false
	_, err := BuildLifecycleExecutionAdmission(LifecycleExecutionAdmissionRequest{
		Handoff:           observed.Handoff,
		Preflight:         observed.Preflight,
		PreflightRequest:  observed.PreflightRequest,
		CurrentMembership: observed.CurrentMembership,
		CurrentEvidence:   observed.CurrentEvidence,
		LifecycleCAS:      observed.LifecycleCAS,
	})
	if !errors.Is(err, ErrInvalidLifecycleExecutionAdmission) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleExecutionAdmission", err)
	}
}

func TestBuildLifecycleExecutionAdmissionAcceptsRevalidatedLeaderTransferHandoff(t *testing.T) {
	receipt, currentMembership, currentEvidence, request := leaderTransferReceiptFixture(t)
	request.LifecyclePlan = executableLifecyclePlan(request.LifecyclePlan)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	handoff, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:             preflight,
		PreflightRequest:      request,
		CurrentMembership:     currentMembership,
		CurrentEvidence:       currentEvidence,
		LeaderTransferReceipt: &receipt,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleHandoff() error = %v", err)
	}
	admission, err := BuildLifecycleExecutionAdmission(LifecycleExecutionAdmissionRequest{
		Handoff:           handoff,
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: currentMembership,
		CurrentEvidence:   currentEvidence,
		LifecycleCAS: LifecycleCASObservation{
			NodeID:          request.LifecyclePlan.NodeID,
			State:           request.LifecyclePlan.From,
			Generation:      request.LifecyclePlan.CurrentGeneration,
			ResourceVersion: request.LifecyclePlan.BasedOnResourceVersion,
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionAdmission() error = %v", err)
	}
	if admission.HandoffID != handoff.HandoffID || admission.NodeID != request.LifecyclePlan.NodeID {
		t.Fatalf("admission lost leader-transfer handoff lineage: %+v", admission)
	}
}
