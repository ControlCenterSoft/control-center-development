package clusterha

import (
	"errors"
	"testing"

	"control-center/internal/nodelifecycle"
)

func TestBuildLifecycleHandoffBindsLeaderTransferReceipt(t *testing.T) {
	receipt, currentMembership, currentEvidence, request := leaderTransferReceiptFixture(t)
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
	if handoff.LeaderTransferReceiptID != receipt.ReceiptID || handoff.FencingReceiptID != "" {
		t.Fatalf("handoff lost exact leader-transfer binding: %+v", handoff)
	}
	if !handoff.ReadyForBoundedExecutor || handoff.ExecutionAuthorized || handoff.HostMutation || handoff.StateMutation {
		t.Fatalf("unsafe handoff flags: %+v", handoff)
	}

	second, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:             preflight,
		PreflightRequest:      request,
		CurrentMembership:     currentMembership,
		CurrentEvidence:       currentEvidence,
		LeaderTransferReceipt: &receipt,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleHandoff(second) error = %v", err)
	}
	if handoff.HandoffID != second.HandoffID {
		t.Fatalf("handoff ID is not deterministic: %s != %s", handoff.HandoffID, second.HandoffID)
	}
}

func TestBuildLifecycleHandoffBindsFencingReceipt(t *testing.T) {
	receipt, currentMembership, currentEvidence, request := fencingReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}

	handoff, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: currentMembership,
		CurrentEvidence:   currentEvidence,
		FencingReceipt:    &receipt,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleHandoff() error = %v", err)
	}
	if handoff.FencingReceiptID != receipt.ReceiptID || handoff.LeaderTransferReceiptID != "" {
		t.Fatalf("handoff lost exact fencing binding: %+v", handoff)
	}
}

func TestBuildLifecycleHandoffAllowsReceiptFreeStandbyDrain(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-handoff-100",
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
	if handoff.LeaderTransferReceiptID != "" || handoff.FencingReceiptID != "" || handoff.StandaloneDowntimeBound {
		t.Fatalf("unexpected receipt-free handoff: %+v", handoff)
	}
}

func TestBuildLifecycleHandoffPreservesStandaloneDowntimeContract(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	request.StandaloneDowntimeAcknowledged = true
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-handoff-standalone-100",
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
	if !handoff.StandaloneDowntimeBound || handoff.LeaderTransferReceiptID != "" || handoff.FencingReceiptID != "" {
		t.Fatalf("standalone contract was not preserved: %+v", handoff)
	}
}

func TestBuildLifecycleHandoffRejectsMissingRequiredReceipt(t *testing.T) {
	_, currentMembership, currentEvidence, request := leaderTransferReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	_, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: currentMembership,
		CurrentEvidence:   currentEvidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleHandoff) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleHandoff", err)
	}
}

func TestBuildLifecycleHandoffRejectsUnrequiredReceipt(t *testing.T) {
	receipt, currentMembership, currentEvidence, _ := leaderTransferReceiptFixture(t)
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, _ := BuildLifecyclePreflight(request)
	_, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:             preflight,
		PreflightRequest:      request,
		CurrentMembership:     currentMembership,
		CurrentEvidence:       currentEvidence,
		LeaderTransferReceipt: &receipt,
	})
	if !errors.Is(err, ErrInvalidLifecycleHandoff) && !errors.Is(err, ErrStaleLifecycleHandoff) {
		t.Fatalf("error = %v, want fail-closed handoff rejection", err)
	}
}

func TestBuildLifecycleHandoffRejectsTamperedPreflightRequirement(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, _ := BuildLifecyclePreflight(request)
	preflight.RequiresFencing = true
	evidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-handoff-tamper-100",
	})
	_, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: request.Membership,
		CurrentEvidence:   evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleHandoff) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleHandoff", err)
	}
}

func TestRevalidateLifecycleHandoffRejectsLaterJournalAdvance(t *testing.T) {
	receipt, currentMembership, currentEvidence, request := leaderTransferReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
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

	validation, err := RevalidateLifecycleHandoff(handoff, currentMembership, currentEvidence)
	if err != nil {
		t.Fatalf("RevalidateLifecycleHandoff() error = %v", err)
	}
	if !validation.Revalidated || !validation.ReadyForBoundedExecutor || validation.ExecutionAuthorized {
		t.Fatalf("unexpected validation: %+v", validation)
	}

	advanced, err := AdvanceTransitionRevisionEvidence(currentEvidence, ReconcilerRevisionObservation{
		Membership:              currentMembership,
		ExpectedResourceVersion: currentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-handoff-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateLifecycleHandoff(handoff, currentMembership, advanced)
	if !errors.Is(err, ErrStaleLifecycleHandoff) {
		t.Fatalf("error = %v, want ErrStaleLifecycleHandoff", err)
	}
}

func TestRevalidateLifecycleHandoffRejectsTampering(t *testing.T) {
	receipt, currentMembership, currentEvidence, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	handoff, err := BuildLifecycleHandoff(LifecycleHandoffRequest{
		Preflight:         preflight,
		PreflightRequest:  request,
		CurrentMembership: currentMembership,
		CurrentEvidence:   currentEvidence,
		FencingReceipt:    &receipt,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleHandoff() error = %v", err)
	}
	handoff.NodeID = "controller-b"
	_, err = RevalidateLifecycleHandoff(handoff, currentMembership, currentEvidence)
	if !errors.Is(err, ErrInvalidLifecycleHandoff) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleHandoff", err)
	}
}
