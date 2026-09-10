package clusterha

import (
	"errors"
	"testing"

	"control-center/internal/nodelifecycle"
)

func leaderTransferReceiptFixture(t *testing.T) (LeaderTransferReceipt, Snapshot, TransitionRevisionEvidence, LifecyclePreflightRequest) {
	t.Helper()
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.LeaderTransferConfirmed = true
	request.NextLeaderID = "controller-b"
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	after := request.Membership
	after.LeaderID = request.NextLeaderID
	observation := ReconcilerRevisionObservation{
		Membership:              after,
		ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-101",
	}
	receipt, err := BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: observation,
	})
	if err != nil {
		t.Fatalf("BuildLeaderTransferReceipt() error = %v", err)
	}
	afterEvidence, err := AdvanceTransitionRevisionEvidence(beforeEvidence, observation)
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	return receipt, after, afterEvidence, request
}

func TestBuildLeaderTransferReceiptIsDeterministicAndMutationFree(t *testing.T) {
	receipt, after, afterEvidence, request := leaderTransferReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	second, err := BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              after,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-101",
		},
	})
	if err != nil {
		t.Fatalf("BuildLeaderTransferReceipt(second) error = %v", err)
	}
	if receipt.ReceiptID != second.ReceiptID {
		t.Fatalf("receipt ID is not deterministic: %s != %s", receipt.ReceiptID, second.ReceiptID)
	}
	if receipt.AfterRevisionEvidenceID != afterEvidence.EvidenceID() || receipt.AfterRevision != afterEvidence.Revision() {
		t.Fatalf("receipt lost exact after revision binding: %+v", receipt)
	}
	if !receipt.ProofOnly || receipt.ExecutionAuthorized || receipt.HostMutation || receipt.StateMutation {
		t.Fatalf("unsafe receipt flags: %+v", receipt)
	}
}

func TestRevalidateLeaderTransferReceiptAllowsExactCurrentEvidence(t *testing.T) {
	receipt, after, afterEvidence, _ := leaderTransferReceiptFixture(t)
	result, err := RevalidateLeaderTransferReceipt(receipt, after, afterEvidence)
	if err != nil {
		t.Fatalf("RevalidateLeaderTransferReceipt() error = %v", err)
	}
	if !result.Revalidated || !result.ProofOnly || result.ExecutionAuthorized || result.HostMutation || result.StateMutation {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	if result.ReceiptID != receipt.ReceiptID || result.AfterRevisionEvidenceID != receipt.AfterRevisionEvidenceID {
		t.Fatalf("validation lost receipt binding: %+v", result)
	}
}

func TestBuildLeaderTransferReceiptRejectsMemberStateDrift(t *testing.T) {
	_, _, _, request := leaderTransferReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-200",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	after := request.Membership
	after.Members = append([]Member(nil), request.Membership.Members...)
	after.LeaderID = request.NextLeaderID
	after.Members[2].Healthy = false
	after.Members[2].CaughtUp = false
	_, err = BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              after,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-201",
		},
	})
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}

func TestBuildLeaderTransferReceiptRejectsGenerationChange(t *testing.T) {
	_, _, _, request := leaderTransferReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-300",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	after := request.Membership
	after.LeaderID = request.NextLeaderID
	after.Generation++
	_, err = BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              after,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-301",
		},
	})
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}

func TestBuildLeaderTransferReceiptRejectsUnadvancedResourceVersion(t *testing.T) {
	_, _, _, request := leaderTransferReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-400",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	after := request.Membership
	after.LeaderID = request.NextLeaderID
	_, err = BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              after,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         beforeEvidence.Revision().ResourceVersion,
		},
	})
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}

func TestBuildLeaderTransferReceiptRejectsMismatchedPreflightRequest(t *testing.T) {
	_, _, _, request := leaderTransferReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-500",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	after := request.Membership
	after.LeaderID = request.NextLeaderID
	tamperedRequest := request
	tamperedRequest.NextLeaderID = "controller-c"
	_, err = BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: tamperedRequest,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     request.NextLeaderID,
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              after,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-501",
		},
	})
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}

func TestRevalidateLeaderTransferReceiptRejectsLaterJournalAdvance(t *testing.T) {
	receipt, after, afterEvidence, _ := leaderTransferReceiptFixture(t)
	later, err := AdvanceTransitionRevisionEvidence(afterEvidence, ReconcilerRevisionObservation{
		Membership:              after,
		ExpectedResourceVersion: afterEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-102",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateLeaderTransferReceipt(receipt, after, later)
	if !errors.Is(err, ErrStaleLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrStaleLeaderTransferReceipt", err)
	}
}

func TestRevalidateLeaderTransferReceiptRejectsSubsequentFailover(t *testing.T) {
	receipt, after, afterEvidence, _ := leaderTransferReceiptFixture(t)
	failedOver := after
	failedOver.LeaderID = "controller-c"
	later, err := AdvanceTransitionRevisionEvidence(afterEvidence, ReconcilerRevisionObservation{
		Membership:              failedOver,
		ExpectedResourceVersion: afterEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-102",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateLeaderTransferReceipt(receipt, failedOver, later)
	if !errors.Is(err, ErrStaleLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrStaleLeaderTransferReceipt", err)
	}
}

func TestRevalidateLeaderTransferReceiptRejectsTamperedReceipt(t *testing.T) {
	receipt, after, afterEvidence, _ := leaderTransferReceiptFixture(t)
	receipt.NextLeaderID = "controller-c"
	_, err := RevalidateLeaderTransferReceipt(receipt, after, afterEvidence)
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}

func TestLeaderTransferReceiptDoesNotApplyToStandalone(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	request.StandaloneDowntimeAcknowledged = true
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-standalone-1",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	_, err = BuildLeaderTransferReceipt(LeaderTransferReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		NextLeaderID:     "controller-b",
		AfterObservation: ReconcilerRevisionObservation{
			Membership:              request.Membership,
			ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-standalone-2",
		},
	})
	if !errors.Is(err, ErrInvalidLeaderTransferReceipt) {
		t.Fatalf("error = %v, want ErrInvalidLeaderTransferReceipt", err)
	}
}
