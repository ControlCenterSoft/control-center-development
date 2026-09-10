package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestBootSessionReadmissionDeterministicAndBounded(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)

	firstAdmission, firstReceipt, err := BuildBootSessionReadmission(
		previous, candidate, reconciliation, request, servingReceipt, plan, payload, nil, nil, reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondAdmission, secondReceipt, err := BuildBootSessionReadmission(
		previous, candidate, reconciliation, request, servingReceipt, plan, payload, nil, nil, reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstAdmission, secondAdmission) || !reflect.DeepEqual(firstReceipt, secondReceipt) {
		t.Fatalf("readmission is not deterministic: admission=%#v/%#v receipt=%#v/%#v", firstAdmission, secondAdmission, firstReceipt, secondReceipt)
	}
	if firstAdmission.AdmissionID == previous.AdmissionID || firstAdmission.ReplayKey == previous.ReplayKey ||
		firstAdmission.MachineID != previous.MachineID || firstAdmission.PlanID != previous.PlanID ||
		firstAdmission.MediaSHA256 != previous.MediaSHA256 || firstAdmission.MediaSize != previous.MediaSize ||
		firstAdmission.Target != previous.Target || !firstAdmission.BootAuthorized ||
		firstAdmission.ProvisioningAuthorized || firstAdmission.SecretInjectionAuthorized ||
		firstAdmission.HostMutation || firstAdmission.NetworkMutation {
		t.Fatalf("fresh admission widened or changed deployment intent: %#v", firstAdmission)
	}
	if !firstReceipt.FreshServingEvidenceVerified || !firstReceipt.FreshAdmissionIssued ||
		firstReceipt.PreviousAdmissionReusable || firstReceipt.PreviousReplayAuthorized ||
		firstReceipt.BootHandoffAuthorized || firstReceipt.ProvisioningAuthorized ||
		firstReceipt.SecretInjectionAuthorized || firstReceipt.HostMutation || firstReceipt.NetworkMutation ||
		firstReceipt.ProductionMutation || firstReceipt.ReconciliationID != reconciliation.ReconciliationID ||
		firstReceipt.CandidateReceiptID != candidate.ReceiptID || firstReceipt.PreviousAdmissionID != previous.AdmissionID || firstReceipt.FreshAdmissionID != firstAdmission.AdmissionID {
		t.Fatalf("readmission receipt weakened recovery boundary: %#v", firstReceipt)
	}
	if err := VerifyBootSessionAdmission(firstAdmission, request, servingReceipt, plan, payload, nil, nil, firstAdmission.IssuedAtUnix); err != nil {
		t.Fatalf("fresh boot admission rejected: %v", err)
	}
	if err := VerifyBootSessionReadmissionReceipt(firstReceipt, previous, candidate, reconciliation, firstAdmission); err != nil {
		t.Fatalf("valid readmission receipt rejected: %v", err)
	}
}

func TestBootSessionReadmissionRejectsNonRecoverableReconciliation(t *testing.T) {
	previous, candidate, _, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)

	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unexpired, err := ReconcileBootSessionConsumption(store, candidate, previous, candidate.ConsumedAtUnix+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildBootSessionReadmission(
		previous, candidate, unexpired, request, servingReceipt, plan, payload, nil, nil, previous.ExpiresAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("unexpired reconciliation permitted readmission: %v", err)
	}

	consumedStore, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := consumedStore.AtomicConsumeBootSession(candidate); err != nil {
		t.Fatal(err)
	}
	consumed, err := ReconcileBootSessionConsumption(consumedStore, candidate, previous, previous.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildBootSessionReadmission(
		previous, candidate, consumed, request, servingReceipt, plan, payload, nil, nil, previous.ExpiresAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("consumed reconciliation permitted readmission: %v", err)
	}

	ambiguousStore := bootSessionConsumptionObservationStoreStub{
		observation: BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
		err:         errors.New("durability unknown"),
	}
	ambiguous, reconcileErr := ReconcileBootSessionConsumption(ambiguousStore, candidate, previous, previous.ExpiresAtUnix)
	if reconcileErr == nil {
		t.Fatal("ambiguous reconciliation unexpectedly returned nil error")
	}
	if _, _, err := BuildBootSessionReadmission(
		previous, candidate, ambiguous, request, servingReceipt, plan, payload, nil, nil, previous.ExpiresAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("ambiguous reconciliation permitted readmission: %v", err)
	}
}

func TestBootSessionReadmissionRejectsIdentityTimeAndLifetimeDrift(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)

	cases := []struct {
		name     string
		request  BootSessionRequest
		issuedAt int64
	}{
		{name: "same request id", request: BootSessionRequest{RequestID: previous.RequestID, MachineID: request.MachineID, TTLSeconds: request.TTLSeconds}, issuedAt: reconciliation.ObservedAtUnix},
		{name: "machine drift", request: BootSessionRequest{RequestID: request.RequestID, MachineID: "machine-other", TTLSeconds: request.TTLSeconds}, issuedAt: reconciliation.ObservedAtUnix},
		{name: "before reconciliation", request: request, issuedAt: reconciliation.ObservedAtUnix - 1},
		{name: "ttl widening", request: BootSessionRequest{RequestID: request.RequestID, MachineID: request.MachineID, TTLSeconds: previous.ExpiresAtUnix - previous.IssuedAtUnix + 1}, issuedAt: reconciliation.ObservedAtUnix},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := BuildBootSessionReadmission(
				previous, candidate, reconciliation, tc.request, servingReceipt, plan, payload, nil, nil, tc.issuedAt,
			); !errors.Is(err, ErrBootSessionReadmission) {
				t.Fatalf("unsafe readmission accepted: %v", err)
			}
		})
	}
}

func TestBootSessionReadmissionRejectsPlanMediaAndPreviousAdmissionDrift(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)

	tamperedPlan := plan
	tamperedPlan.Architecture = "arm64"
	if _, _, err := BuildBootSessionReadmission(
		previous, candidate, reconciliation, request, servingReceipt, tamperedPlan, payload, nil, nil, reconciliation.ObservedAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("plan drift permitted readmission: %v", err)
	}
	if _, _, err := BuildBootSessionReadmission(
		previous, candidate, reconciliation, request, servingReceipt, plan, []byte("changed-media"), nil, nil, reconciliation.ObservedAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("media drift permitted readmission: %v", err)
	}

	forgedPrevious := previous
	forgedPrevious.MachineID = "machine-forged"
	if _, _, err := BuildBootSessionReadmission(
		forgedPrevious, candidate, reconciliation, request, servingReceipt, plan, payload, nil, nil, reconciliation.ObservedAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("previous admission digest drift was accepted: %v", err)
	}

	forgedCandidate := candidate
	forgedCandidate.ConsumerID = "consumer-forged"
	if _, _, err := BuildBootSessionReadmission(
		previous, forgedCandidate, reconciliation, request, servingReceipt, plan, payload, nil, nil, reconciliation.ObservedAtUnix,
	); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("candidate receipt drift was accepted: %v", err)
	}
}

func TestVerifyBootSessionReadmissionReceiptRejectsAuthorityAndLineageDrift(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, receipt, err := BuildBootSessionReadmission(
		previous, candidate, reconciliation, request, servingReceipt, plan, payload, nil, nil, reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}

	unsafe := []BootSessionReadmissionReceipt{receipt, receipt, receipt, receipt}
	unsafe[0].PreviousAdmissionReusable = true
	unsafe[1].BootHandoffAuthorized = true
	unsafe[2].ProductionMutation = true
	unsafe[3].FreshReplayKey = previous.ReplayKey
	for _, tampered := range unsafe {
		if err := VerifyBootSessionReadmissionReceipt(tampered, previous, candidate, reconciliation, fresh); !errors.Is(err, ErrBootSessionReadmission) {
			t.Fatalf("unsafe readmission receipt accepted: %#v err=%v", tampered, err)
		}
	}

	drift := receipt
	drift.RecoveryAction = "caller-selected-recovery"
	if err := VerifyBootSessionReadmissionReceipt(drift, previous, candidate, reconciliation, fresh); !errors.Is(err, ErrBootSessionReadmission) {
		t.Fatalf("receipt digest/semantic drift accepted: %v", err)
	}
}

func bootSessionReadmissionFixture(t *testing.T) (
	BootSessionAdmission,
	BootSessionConsumptionReceipt,
	BootSessionConsumptionReconciliation,
	BootSessionRequest,
	InstallMediaServingReceipt,
	AdmittedDeploymentPlan,
	[]byte,
) {
	t.Helper()
	oldRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	previous, err := BuildBootSessionAdmission(oldRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := BuildBootSessionConsumptionReceipt(BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission",
		ConsumerID:     "edge-readmission-0001",
		ConsumedAtUnix: issuedAt + 1,
	}, previous)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := ReconcileBootSessionConsumption(store, candidate, previous, previous.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionRequest{
		RequestID:  "req-readmission-0002",
		MachineID:  oldRequest.MachineID,
		TTLSeconds: oldRequest.TTLSeconds,
	}
	return previous, candidate, reconciliation, request, servingReceipt, plan, payload
}
