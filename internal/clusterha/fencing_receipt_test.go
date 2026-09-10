package clusterha

import (
	"errors"
	"testing"

	"control-center/internal/nodelifecycle"
)

func fencingReceiptFixture(t *testing.T) (FencingReceipt, Snapshot, TransitionRevisionEvidence, LifecyclePreflightRequest) {
	t.Helper()
	request := preflightRequest("controller-c", nodelifecycle.StateDraining)
	request.Membership.Members[2].Healthy = false
	request.Membership.Members[2].CaughtUp = false
	request.FencingConfirmed = true
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-fence-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	observation := ReconcilerFencingObservation{
		Membership:              request.Membership,
		ExpectedResourceVersion: beforeEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-fence-101",
		TargetNodeID:            "controller-c",
		FenceObservationID:      "fence-event-001",
		Fenced:                  true,
	}
	receipt, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		AfterObservation: observation,
	})
	if err != nil {
		t.Fatalf("BuildFencingReceipt() error = %v", err)
	}
	afterEvidence, err := AdvanceTransitionRevisionEvidence(beforeEvidence, ReconcilerRevisionObservation{
		Membership:              observation.Membership,
		ExpectedResourceVersion: observation.ExpectedResourceVersion,
		ResourceVersion:         observation.ResourceVersion,
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	return receipt, observation.Membership, afterEvidence, request
}

func TestBuildFencingReceiptIsDeterministicAndMutationFree(t *testing.T) {
	receipt, _, _, request := fencingReceiptFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-fence-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	second, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		BeforeMembership: request.Membership,
		BeforeEvidence:   beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership:              request.Membership,
			ExpectedResourceVersion: "rv-fence-100",
			ResourceVersion:         "rv-fence-101",
			TargetNodeID:            "controller-c",
			FenceObservationID:      "fence-event-001",
			Fenced:                  true,
		},
	})
	if err != nil {
		t.Fatalf("BuildFencingReceipt(second) error = %v", err)
	}
	if receipt.ReceiptID != second.ReceiptID {
		t.Fatalf("receipt ID is not deterministic: %s != %s", receipt.ReceiptID, second.ReceiptID)
	}
	if !receipt.ProofOnly || receipt.ExecutionAuthorized || receipt.HostMutation || receipt.StateMutation {
		t.Fatalf("unsafe receipt flags: %+v", receipt)
	}
}

func TestRevalidateFencingReceiptAllowsExactCurrentEvidence(t *testing.T) {
	receipt, membership, evidence, _ := fencingReceiptFixture(t)
	result, err := RevalidateFencingReceipt(receipt, membership, evidence)
	if err != nil {
		t.Fatalf("RevalidateFencingReceipt() error = %v", err)
	}
	if !result.Revalidated || !result.ProofOnly || result.ExecutionAuthorized || result.HostMutation || result.StateMutation {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	if result.ReceiptID != receipt.ReceiptID || result.FenceObservationID != receipt.FenceObservationID {
		t.Fatalf("validation lost receipt binding: %+v", result)
	}
}

func TestBuildFencingReceiptRejectsWrongTarget(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-200",
	})
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: request.Membership, ExpectedResourceVersion: "rv-fence-200", ResourceVersion: "rv-fence-201",
			TargetNodeID: "controller-b", FenceObservationID: "fence-event-002", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsUnconfirmedFence(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-300",
	})
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: request.Membership, ExpectedResourceVersion: "rv-fence-300", ResourceVersion: "rv-fence-301",
			TargetNodeID: "controller-c", FenceObservationID: "fence-event-003", Fenced: false,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsMembershipDrift(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-400",
	})
	after := request.Membership
	after.Members = append([]Member(nil), request.Membership.Members...)
	after.Members[1].CaughtUp = false
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: after, ExpectedResourceVersion: "rv-fence-400", ResourceVersion: "rv-fence-401",
			TargetNodeID: "controller-c", FenceObservationID: "fence-event-004", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsLeaderTransition(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-500",
	})
	after := request.Membership
	after.LeaderID = "controller-b"
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: after, ExpectedResourceVersion: "rv-fence-500", ResourceVersion: "rv-fence-501",
			TargetNodeID: "controller-c", FenceObservationID: "fence-event-005", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsStaleCAS(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-600",
	})
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: request.Membership, ExpectedResourceVersion: "rv-fence-stale", ResourceVersion: "rv-fence-601",
			TargetNodeID: "controller-c", FenceObservationID: "fence-event-006", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsUnadvancedResourceVersion(t *testing.T) {
	_, _, _, request := fencingReceiptFixture(t)
	preflight, _ := BuildLifecyclePreflight(request)
	beforeEvidence, _ := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-fence-700",
	})
	_, err := BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: request.Membership, ExpectedResourceVersion: "rv-fence-700", ResourceVersion: "rv-fence-700",
			TargetNodeID: "controller-c", FenceObservationID: "fence-event-007", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestRevalidateFencingReceiptRejectsLaterJournalAdvance(t *testing.T) {
	receipt, membership, evidence, _ := fencingReceiptFixture(t)
	later, err := AdvanceTransitionRevisionEvidence(evidence, ReconcilerRevisionObservation{
		Membership: membership, ExpectedResourceVersion: evidence.Revision().ResourceVersion, ResourceVersion: "rv-fence-102",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateFencingReceipt(receipt, membership, later)
	if !errors.Is(err, ErrStaleFencingReceipt) {
		t.Fatalf("error = %v, want ErrStaleFencingReceipt", err)
	}
}

func TestRevalidateFencingReceiptRejectsSubsequentFailover(t *testing.T) {
	receipt, membership, evidence, _ := fencingReceiptFixture(t)
	failedOver := membership
	failedOver.LeaderID = "controller-b"
	later, err := AdvanceTransitionRevisionEvidence(evidence, ReconcilerRevisionObservation{
		Membership: failedOver, ExpectedResourceVersion: evidence.Revision().ResourceVersion, ResourceVersion: "rv-fence-102",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateFencingReceipt(receipt, failedOver, later)
	if !errors.Is(err, ErrStaleFencingReceipt) {
		t.Fatalf("error = %v, want ErrStaleFencingReceipt", err)
	}
}

func TestRevalidateFencingReceiptRejectsTampering(t *testing.T) {
	receipt, membership, evidence, _ := fencingReceiptFixture(t)
	receipt.FenceObservationID = "fence-event-tampered"
	_, err := RevalidateFencingReceipt(receipt, membership, evidence)
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}

func TestBuildFencingReceiptRejectsStandalonePreflight(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	request.StandaloneDowntimeAcknowledged = true
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	beforeEvidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership: request.Membership, ResourceVersion: "rv-standalone-fence-1",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	_, err = BuildFencingReceipt(FencingReceiptRequest{
		Preflight: preflight, PreflightRequest: request, BeforeMembership: request.Membership, BeforeEvidence: beforeEvidence,
		AfterObservation: ReconcilerFencingObservation{
			Membership: request.Membership, ExpectedResourceVersion: "rv-standalone-fence-1", ResourceVersion: "rv-standalone-fence-2",
			TargetNodeID: "controller-a", FenceObservationID: "fence-event-standalone", Fenced: true,
		},
	})
	if !errors.Is(err, ErrInvalidFencingReceipt) {
		t.Fatalf("error = %v, want ErrInvalidFencingReceipt", err)
	}
}
