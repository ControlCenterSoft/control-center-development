package clusterha

import (
	"errors"
	"testing"

	"control-center/internal/nodelifecycle"
)

func transitionTokenFixture(t *testing.T) (LifecycleTransitionToken, LifecyclePreflightRequest, TransitionRevisionEvidence) {
	t.Helper()
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-42",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	token, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken() error = %v", err)
	}
	return token, request, evidence
}

func TestBuildLifecycleTransitionTokenIsDeterministicAndMutationFree(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	second, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken(second) error = %v", err)
	}
	if token.TokenID != second.TokenID {
		t.Fatalf("token ID is not deterministic: %s != %s", token.TokenID, second.TokenID)
	}
	if token.RevisionEvidenceID != evidence.EvidenceID() || token.Revision != evidence.Revision() {
		t.Fatalf("token lost sealed revision binding: token=%+v evidence=%+v", token, evidence)
	}
	if token.MembershipSnapshotID == "" || !token.PlanOnly || token.ExecutionAuthorized || token.HostMutation || token.StateMutation {
		t.Fatalf("unexpected token safety contract: %+v", token)
	}
}

func TestRevalidateLifecycleTransitionAllowsExactObservation(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	result, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if err != nil {
		t.Fatalf("RevalidateLifecycleTransition() error = %v", err)
	}
	if !result.Revalidated || !result.PlanOnly || result.ExecutionAuthorized || result.HostMutation || result.StateMutation {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	if result.TokenID != token.TokenID || result.MembershipSnapshotID != token.MembershipSnapshotID || result.RevisionEvidenceID != evidence.EvidenceID() {
		t.Fatalf("validation lost token binding: %+v", result)
	}
}

func TestBuildLifecycleTransitionTokenRejectsEvidenceFromDifferentMembership(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	other := request.Membership
	other.Members = append([]Member(nil), request.Membership.Members...)
	other.Members[2].Healthy = false
	other.Members[2].CaughtUp = false
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      other,
		ResourceVersion: "rv-other",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	_, err = BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsJournalAdvance(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	next, err := AdvanceTransitionRevisionEvidence(evidence, ReconcilerRevisionObservation{
		Membership:              request.Membership,
		ExpectedResourceVersion: evidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-43",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: next,
	})
	if !errors.Is(err, ErrStaleLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrStaleLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsLeaderFailoverEvidence(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	failedOver := request.Membership
	failedOver.LeaderID = "controller-c"
	next, err := AdvanceTransitionRevisionEvidence(evidence, ReconcilerRevisionObservation{
		Membership:              failedOver,
		ExpectedResourceVersion: evidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-43",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	request.Membership = failedOver
	_, err = RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: next,
	})
	if !errors.Is(err, ErrStaleLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrStaleLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsUnsafeHealthDrift(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	request.Membership.Members[2].Healthy = false
	request.Membership.Members[2].CaughtUp = false
	next, err := AdvanceTransitionRevisionEvidence(evidence, ReconcilerRevisionObservation{
		Membership:              request.Membership,
		ExpectedResourceVersion: evidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-43",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	_, err = RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: next,
	})
	if !errors.Is(err, ErrStaleLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrStaleLifecycleTransitionToken", err)
	}
}

func TestBuildLifecycleTransitionTokenRejectsMismatchedPreflightEvidence(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-42",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	request.Membership.LeaderID = "controller-c"
	_, err = BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestBuildLifecycleTransitionTokenRejectsTamperedSealedEvidence(t *testing.T) {
	_, request, evidence := transitionTokenFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence.revision.JournalSequence++
	_, err = BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsTamperedTokenEvidenceIdentity(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	token.RevisionEvidenceID = "chre-tampered"
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsTamperedTokenRevision(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	token.Revision.JournalSequence++
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsTamperedCurrentEvidence(t *testing.T) {
	token, request, evidence := transitionTokenFixture(t)
	evidence.revision.JournalSequence++
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestLifecycleTransitionTokenSupportsStandaloneWithExplicitDowntime(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	request.StandaloneDowntimeAcknowledged = true
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      request.Membership,
		ResourceVersion: "rv-standalone-9",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	token, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		RevisionEvidence: evidence,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken() error = %v", err)
	}
	if _, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{PreflightRequest: request, RevisionEvidence: evidence}); err != nil {
		t.Fatalf("RevalidateLifecycleTransition() error = %v", err)
	}
}
