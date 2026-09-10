package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestBootSessionNewDeploymentIntentDeterministicAndDisjoint(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)

	firstAdmission, firstReceipt, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.issuedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondAdmission, secondReceipt, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.issuedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstAdmission, secondAdmission) || !reflect.DeepEqual(firstReceipt, secondReceipt) {
		t.Fatalf("identical terminal/current evidence produced non-deterministic new intent")
	}
	if firstAdmission.AdmissionID == fixture.previous.AdmissionID ||
		firstAdmission.AdmissionID == fixture.terminalAdmission.AdmissionID ||
		firstAdmission.ReplayKey == fixture.previous.ReplayKey ||
		firstAdmission.ReplayKey == fixture.terminalAdmission.ReplayKey {
		t.Fatalf("new deployment intent reused exhausted admission/replay lineage: %#v", firstAdmission)
	}
	if firstAdmission.MachineID != fixture.previous.MachineID || !firstAdmission.BootAuthorized ||
		firstAdmission.ProvisioningAuthorized || firstAdmission.SecretInjectionAuthorized ||
		firstAdmission.HostMutation || firstAdmission.NetworkMutation {
		t.Fatalf("new boot admission widened authority: %#v", firstAdmission)
	}
	if !firstReceipt.OperatorApproved || !firstReceipt.FreshServingEvidenceVerified ||
		!firstReceipt.NewAdmissionIssued || firstReceipt.PriorAdmissionsReusable ||
		firstReceipt.PriorReplayAuthorized || firstReceipt.FurtherReadmissionAuthorized ||
		firstReceipt.AutomaticRetryAuthorized || firstReceipt.BootHandoffAuthorized ||
		firstReceipt.ProvisioningAuthorized || firstReceipt.SecretInjectionAuthorized ||
		firstReceipt.HostMutation || firstReceipt.NetworkMutation || firstReceipt.ProductionMutation {
		t.Fatalf("new deployment intent receipt widened recovery authority: %#v", firstReceipt)
	}
	if firstReceipt.TerminalReconciliationID != fixture.terminalReconciliation.ReconciliationID ||
		firstReceipt.TerminalReadmissionConsumptionReceiptID != fixture.terminalConsumption.ReceiptID ||
		firstReceipt.NewAdmissionID != firstAdmission.AdmissionID ||
		firstReceipt.NewReplayKey != firstAdmission.ReplayKey ||
		firstReceipt.RecoveryAction != "consume-new-deployment-admission-once-or-reconcile" {
		t.Fatalf("new deployment intent receipt lost exact lineage: %#v", firstReceipt)
	}
	if err := VerifyBootSessionNewDeploymentIntentReceipt(
		firstReceipt,
		firstAdmission,
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
	); err != nil {
		t.Fatalf("valid new deployment intent rejected: %v", err)
	}
}

func TestBootSessionNewDeploymentIntentWaitsForTerminalAdmissionExpiry(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)

	_, _, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.terminalAdmission.ExpiresAtUnix-1,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntent) {
		t.Fatalf("overlapping terminal/new admission was accepted: %v", err)
	}
}

func TestBootSessionNewDeploymentIntentRequiresExplicitApprovalAndFreshIdentity(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	cases := []BootSessionNewDeploymentIntentRequest{
		fixture.intent,
		fixture.intent,
		fixture.intent,
		fixture.intent,
	}
	cases[0].OperatorApproved = false
	cases[1].BootRequest.RequestID = fixture.terminalAdmission.RequestID
	cases[2].BootRequest.MachineID = "machine-other"
	cases[3].IntentID = fixture.intent.BootRequest.RequestID

	for _, candidate := range cases {
		_, _, err := BuildBootSessionNewDeploymentIntent(
			candidate,
			fixture.terminalReconciliation,
			fixture.terminalConsumption,
			fixture.readmission,
			fixture.previous,
			fixture.previousCandidate,
			fixture.previousReconciliation,
			fixture.terminalAdmission,
			fixture.terminalCandidate,
			fixture.servingReceipt,
			fixture.plan,
			fixture.payload,
			nil,
			nil,
			fixture.issuedAt,
		)
		if !errors.Is(err, ErrBootSessionNewDeploymentIntent) {
			t.Fatalf("unsafe operator/new-lineage request was accepted: %#v err=%v", candidate, err)
		}
	}
}

func TestBootSessionNewDeploymentIntentRequiresDefinitelyAbsentTerminalEvidence(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(fixture.terminalCandidate); err != nil {
		t.Fatal(err)
	}
	consumed, err := ReconcileBootSessionReadmissionConsumption(
		store,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.terminalReconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.State != BootSessionConsumptionReconciledConsumed {
		t.Fatalf("fixture did not produce consumed terminal state: %#v", consumed)
	}

	_, _, err = BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		consumed,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.issuedAt,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntent) {
		t.Fatalf("consumed terminal lineage permitted a new intent: %v", err)
	}
}

func TestBootSessionNewDeploymentIntentRevalidatesCurrentMedia(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	_, _, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		[]byte("tampered-current-media"),
		nil,
		nil,
		fixture.issuedAt,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntent) {
		t.Fatalf("media checksum/size drift was accepted: %v", err)
	}
}

func TestVerifyBootSessionNewDeploymentIntentRejectsAuthorityAndLineageTampering(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, receipt, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.issuedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	tampered := []BootSessionNewDeploymentIntentReceipt{receipt, receipt, receipt, receipt, receipt}
	tampered[0].PriorReplayAuthorized = true
	tampered[1].AutomaticRetryAuthorized = true
	tampered[2].BootHandoffAuthorized = true
	tampered[3].ProductionMutation = true
	tampered[4].TerminalReplayKey = admission.ReplayKey
	for _, candidate := range tampered {
		err := VerifyBootSessionNewDeploymentIntentReceipt(
			candidate,
			admission,
			fixture.intent,
			fixture.terminalReconciliation,
			fixture.terminalConsumption,
			fixture.readmission,
			fixture.previous,
			fixture.previousCandidate,
			fixture.previousReconciliation,
			fixture.terminalAdmission,
			fixture.terminalCandidate,
			fixture.servingReceipt,
			fixture.plan,
			fixture.payload,
			nil,
			nil,
		)
		if !errors.Is(err, ErrBootSessionNewDeploymentIntent) {
			t.Fatalf("tampered new deployment intent receipt accepted: %#v err=%v", candidate, err)
		}
	}
}

type bootSessionNewDeploymentIntentTestFixture struct {
	previous               BootSessionAdmission
	previousCandidate      BootSessionConsumptionReceipt
	previousReconciliation BootSessionConsumptionReconciliation
	terminalAdmission      BootSessionAdmission
	readmission            BootSessionReadmissionReceipt
	terminalCandidate      BootSessionConsumptionReceipt
	terminalConsumption    BootSessionReadmissionConsumptionReceipt
	terminalReconciliation BootSessionReadmissionConsumptionReconciliation
	intent                 BootSessionNewDeploymentIntentRequest
	servingReceipt         InstallMediaServingReceipt
	plan                   AdmittedDeploymentPlan
	payload                []byte
	issuedAt               int64
}

func bootSessionNewDeploymentIntentFixture(t *testing.T) bootSessionNewDeploymentIntentTestFixture {
	t.Helper()
	previous, previousCandidate, previousReconciliation, terminalAdmission, readmission, terminalCandidate, terminalConsumption :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	terminalReconciliation, err := ReconcileBootSessionReadmissionConsumption(
		store,
		terminalConsumption,
		readmission,
		previous,
		previousCandidate,
		previousReconciliation,
		terminalAdmission,
		terminalCandidate,
		terminalCandidate.ConsumedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if terminalReconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent {
		t.Fatalf("fixture did not produce definitely-absent terminal state: %#v", terminalReconciliation)
	}
	_, servingReceipt, plan, payload := bootSessionFixture(t)
	issuedAt := terminalAdmission.ExpiresAtUnix
	return bootSessionNewDeploymentIntentTestFixture{
		previous:               previous,
		previousCandidate:      previousCandidate,
		previousReconciliation: previousReconciliation,
		terminalAdmission:      terminalAdmission,
		readmission:            readmission,
		terminalCandidate:      terminalCandidate,
		terminalConsumption:    terminalConsumption,
		terminalReconciliation: terminalReconciliation,
		intent: BootSessionNewDeploymentIntentRequest{
			IntentID:         "intent-deployment-after-terminal-recovery-0001",
			OperatorApproved: true,
			BootRequest: BootSessionRequest{
				RequestID:  "req-new-deployment-intent-0003",
				MachineID:  previous.MachineID,
				TTLSeconds: 120,
			},
		},
		servingReceipt: servingReceipt,
		plan:           plan,
		payload:        payload,
		issuedAt:       issuedAt,
	}
}
