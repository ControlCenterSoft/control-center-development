package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestSeparateDeploymentIntentDeterministicAndDisjoint(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	reconciliation := definitelyAbsentExpiredNewIntentReconciliation(t, source, evidence)
	request := BootSessionSeparateDeploymentIntentRequest{
		IntentID:         "intent-separate-deployment-recovery-0002",
		OperatorApproved: true,
		BootRequest: BootSessionRequest{
			RequestID:  "req-separate-deployment-recovery-0004",
			MachineID:  evidence.NewAdmission.MachineID,
			TTLSeconds: 120,
		},
	}
	issuedAt := reconciliation.ObservedAtUnix + 1

	firstAdmission, firstReceipt, err := BuildBootSessionSeparateDeploymentIntent(
		request, reconciliation, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil, issuedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		nextAdmission, nextReceipt, err := BuildBootSessionSeparateDeploymentIntent(
			request, reconciliation, source, evidence,
			fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil, issuedAt,
		)
		if err != nil || !reflect.DeepEqual(firstAdmission, nextAdmission) ||
			!reflect.DeepEqual(firstReceipt, nextReceipt) {
			t.Fatalf("non-deterministic separate intent: admission=%#v receipt=%#v err=%v",
				nextAdmission, nextReceipt, err)
		}
	}
	if firstAdmission.AdmissionID == evidence.NewAdmission.AdmissionID ||
		firstAdmission.ReplayKey == evidence.NewAdmission.ReplayKey ||
		firstAdmission.AdmissionID == evidence.TerminalAdmission.AdmissionID ||
		firstAdmission.ReplayKey == evidence.TerminalAdmission.ReplayKey {
		t.Fatalf("separate deployment intent reused exhausted lineage: %#v", firstAdmission)
	}
	if !firstAdmission.BootAuthorized || !firstAdmission.SingleUse ||
		!firstAdmission.AtomicConsumeRequired || firstAdmission.MaxBootAttempts != 1 ||
		firstAdmission.ProvisioningAuthorized || firstAdmission.SecretInjectionAuthorized ||
		firstAdmission.HostMutation || firstAdmission.NetworkMutation {
		t.Fatalf("separate deployment admission widened authority: %#v", firstAdmission)
	}
	assertSeparateDeploymentIntentReceiptNoAuthority(t, firstReceipt)
	if !firstReceipt.OperatorApproved || !firstReceipt.PreviousAdmissionExpired ||
		!firstReceipt.FreshServingEvidenceVerified || !firstReceipt.NewAdmissionIssued ||
		firstReceipt.PreviousReconciliationID != reconciliation.ReconciliationID ||
		firstReceipt.PreviousIntentReceiptID != evidence.IntentReceipt.ReceiptID ||
		firstReceipt.NewAdmissionID != firstAdmission.AdmissionID ||
		firstReceipt.NewReplayKey != firstAdmission.ReplayKey ||
		firstReceipt.RecoveryAction != "consume-separate-deployment-admission-once-or-reconcile" {
		t.Fatalf("separate deployment intent lost recovery boundary: %#v", firstReceipt)
	}
	if err := VerifyBootSessionSeparateDeploymentIntentReceipt(
		firstReceipt, firstAdmission, request, reconciliation, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
	); err != nil {
		t.Fatalf("valid separate deployment intent rejected: %v", err)
	}
}

func TestSeparateDeploymentIntentRequiresExpiredDefinitelyAbsentEvidence(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	observedAt := source.ConsumptionReceipt.ConsumedAtUnix + 1
	if observedAt >= evidence.NewAdmission.ExpiresAtUnix {
		t.Fatalf("fixture does not provide pre-expiry observation window")
	}
	reconciliation, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		store, source, evidence, observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		reconciliation.AdmissionExpired {
		t.Fatalf("fixture did not produce live definitely-absent evidence: %#v", reconciliation)
	}
	request := validSeparateDeploymentIntentRequest(evidence)
	_, _, err = BuildBootSessionSeparateDeploymentIntent(
		request, reconciliation, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
		evidence.NewAdmission.ExpiresAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
		t.Fatalf("live prior admission permitted separate intent: %v", err)
	}
}

func TestSeparateDeploymentIntentRejectsConsumedAndAmbiguousRecovery(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	request := validSeparateDeploymentIntentRequest(evidence)

	consumedStore, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result, err := consumedStore.AtomicConsumeBootSession(source.ConsumptionReceipt); err != nil ||
		result.State != BootSessionConsumptionCommitted {
		t.Fatalf("persist candidate: result=%#v err=%v", result, err)
	}
	consumed, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		consumedStore, source, evidence, evidence.NewAdmission.ExpiresAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = BuildBootSessionSeparateDeploymentIntent(
		request, consumed, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
		consumed.ObservedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
		t.Fatalf("consumed prior lineage permitted separate intent: %v", err)
	}

	ambiguous, reconcileErr := ReconcileBootSessionNewDeploymentIntentConsumption(
		newDeploymentIntentObservationErrorStore{}, source, evidence,
		evidence.NewAdmission.ExpiresAtUnix+1,
	)
	if !errors.Is(reconcileErr, ErrBootSessionNewDeploymentIntentConsumptionReconciliation) {
		t.Fatalf("fixture did not produce ambiguous reconciliation: %#v err=%v", ambiguous, reconcileErr)
	}
	_, _, err = BuildBootSessionSeparateDeploymentIntent(
		request, ambiguous, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
		ambiguous.ObservedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
		t.Fatalf("ambiguous prior lineage permitted separate intent: %v", err)
	}
}

func TestSeparateDeploymentIntentRequiresFreshMediaEvidence(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	reconciliation := definitelyAbsentExpiredNewIntentReconciliation(t, source, evidence)
	_, _, err := BuildBootSessionSeparateDeploymentIntent(
		validSeparateDeploymentIntentRequest(evidence),
		reconciliation,
		source,
		evidence,
		fixture.servingReceipt,
		fixture.plan,
		[]byte("tampered-current-media"),
		nil,
		nil,
		reconciliation.ObservedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
		t.Fatalf("media integrity drift was accepted: %v", err)
	}
}

func TestSeparateDeploymentIntentRejectsIdentityReuseAndImplicitApproval(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	reconciliation := definitelyAbsentExpiredNewIntentReconciliation(t, source, evidence)
	base := validSeparateDeploymentIntentRequest(evidence)
	cases := []BootSessionSeparateDeploymentIntentRequest{base, base, base, base}
	cases[0].OperatorApproved = false
	cases[1].IntentID = evidence.IntentReceipt.IntentID
	cases[2].BootRequest.RequestID = evidence.NewAdmission.RequestID
	cases[3].BootRequest.MachineID = "machine-other"
	for _, candidate := range cases {
		_, _, err := BuildBootSessionSeparateDeploymentIntent(
			candidate, reconciliation, source, evidence,
			fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
			reconciliation.ObservedAtUnix+1,
		)
		if !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
			t.Fatalf("unsafe separate intent accepted: %#v err=%v", candidate, err)
		}
	}
}

func TestVerifySeparateDeploymentIntentRejectsAuthorityTampering(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	reconciliation := definitelyAbsentExpiredNewIntentReconciliation(t, source, evidence)
	request := validSeparateDeploymentIntentRequest(evidence)
	admission, receipt, err := BuildBootSessionSeparateDeploymentIntent(
		request, reconciliation, source, evidence,
		fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
		reconciliation.ObservedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	cases := []BootSessionSeparateDeploymentIntentReceipt{receipt, receipt, receipt, receipt, receipt}
	cases[0].PreviousAdmissionReusable = true
	cases[1].PreviousReplayAuthorized = true
	cases[2].AutomaticRetryAuthorized = true
	cases[3].BootHandoffAuthorized = true
	cases[4].ProductionMutation = true
	for _, candidate := range cases {
		if err := VerifyBootSessionSeparateDeploymentIntentReceipt(
			candidate, admission, request, reconciliation, source, evidence,
			fixture.servingReceipt, fixture.plan, fixture.payload, nil, nil,
		); !errors.Is(err, ErrBootSessionSeparateDeploymentIntent) {
			t.Fatalf("authority tampering accepted: %#v err=%v", candidate, err)
		}
	}
}

func definitelyAbsentExpiredNewIntentReconciliation(
	t *testing.T,
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
) BootSessionNewDeploymentIntentConsumptionReconciliation {
	t.Helper()
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		store, source, evidence, evidence.NewAdmission.ExpiresAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		!decision.AdmissionExpired || !decision.RecoveryRequired {
		t.Fatalf("fixture did not produce expired definitely-absent evidence: %#v", decision)
	}
	return decision
}

func validSeparateDeploymentIntentRequest(
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
) BootSessionSeparateDeploymentIntentRequest {
	return BootSessionSeparateDeploymentIntentRequest{
		IntentID:         "intent-separate-deployment-recovery-0002",
		OperatorApproved: true,
		BootRequest: BootSessionRequest{
			RequestID:  "req-separate-deployment-recovery-0004",
			MachineID:  evidence.NewAdmission.MachineID,
			TTLSeconds: 120,
		},
	}
}

func assertSeparateDeploymentIntentReceiptNoAuthority(
	t *testing.T,
	receipt BootSessionSeparateDeploymentIntentReceipt,
) {
	t.Helper()
	if receipt.PreviousAdmissionReusable || receipt.PreviousReplayAuthorized ||
		receipt.AutomaticRetryAuthorized || receipt.BootHandoffAuthorized ||
		receipt.ProvisioningAuthorized || receipt.SecretInjectionAuthorized ||
		receipt.HostMutation || receipt.NetworkMutation || receipt.ProductionMutation {
		t.Fatalf("separate deployment receipt widened authority: %#v", receipt)
	}
}
