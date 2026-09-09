package clusterha

import (
	"errors"
	"testing"

	"control-center/internal/nodelifecycle"
)

func transitionTokenFixture(t *testing.T) (LifecycleTransitionToken, LifecyclePreflightRequest, TransitionRevision) {
	t.Helper()
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	revision := TransitionRevision{
		LeaderEpoch:     7,
		JournalSequence: 42,
		ResourceVersion: "rv-42",
	}
	token, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		Revision:         revision,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken() error = %v", err)
	}
	return token, request, revision
}

func TestBuildLifecycleTransitionTokenIsDeterministicAndMutationFree(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	second, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		Revision:         revision,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken(second) error = %v", err)
	}
	if token.TokenID != second.TokenID {
		t.Fatalf("token ID is not deterministic: %s != %s", token.TokenID, second.TokenID)
	}
	if token.MembershipSnapshotID == "" || !token.PlanOnly || token.ExecutionAuthorized || token.HostMutation || token.StateMutation {
		t.Fatalf("unexpected token safety contract: %+v", token)
	}
}

func TestRevalidateLifecycleTransitionAllowsExactObservation(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	result, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		Revision:         revision,
	})
	if err != nil {
		t.Fatalf("RevalidateLifecycleTransition() error = %v", err)
	}
	if !result.Revalidated || !result.PlanOnly || result.ExecutionAuthorized || result.HostMutation || result.StateMutation {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	if result.TokenID != token.TokenID || result.MembershipSnapshotID != token.MembershipSnapshotID {
		t.Fatalf("validation lost token binding: %+v", result)
	}
}

func TestRevalidateLifecycleTransitionRejectsHARevisionDrift(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	tests := []struct {
		name   string
		mutate func(*TransitionRevision)
	}{
		{name: "leader epoch", mutate: func(value *TransitionRevision) { value.LeaderEpoch++ }},
		{name: "journal sequence", mutate: func(value *TransitionRevision) { value.JournalSequence++ }},
		{name: "resource version", mutate: func(value *TransitionRevision) { value.ResourceVersion = "rv-43" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observed := revision
			test.mutate(&observed)
			_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
				PreflightRequest: request,
				Revision:         observed,
			})
			if !errors.Is(err, ErrStaleLifecycleTransitionToken) {
				t.Fatalf("error = %v, want ErrStaleLifecycleTransitionToken", err)
			}
		})
	}
}

func TestRevalidateLifecycleTransitionRejectsMembershipDrift(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	request.Membership.LeaderID = "controller-c"
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		Revision:         revision,
	})
	if !errors.Is(err, ErrStaleLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrStaleLifecycleTransitionToken", err)
	}
}

func TestRevalidateLifecycleTransitionRejectsUnsafeHealthDrift(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	request.Membership.Members[2].Healthy = false
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		Revision:         revision,
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
	request.Membership.LeaderID = "controller-c"
	_, err = BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		Revision: TransitionRevision{
			LeaderEpoch:     7,
			JournalSequence: 42,
			ResourceVersion: "rv-42",
		},
	})
	if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleTransitionToken", err)
	}
}

func TestBuildLifecycleTransitionTokenRejectsInvalidRevision(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	preflight, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	for _, revision := range []TransitionRevision{
		{LeaderEpoch: 0, JournalSequence: 42, ResourceVersion: "rv-42"},
		{LeaderEpoch: 7, JournalSequence: 0, ResourceVersion: "rv-42"},
		{LeaderEpoch: 7, JournalSequence: 42, ResourceVersion: "bad version"},
	} {
		_, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
			Preflight:        preflight,
			PreflightRequest: request,
			Revision:         revision,
		})
		if !errors.Is(err, ErrInvalidLifecycleTransitionToken) {
			t.Fatalf("revision %+v error = %v, want ErrInvalidLifecycleTransitionToken", revision, err)
		}
	}
}

func TestRevalidateLifecycleTransitionRejectsTamperedSafetyFlags(t *testing.T) {
	token, request, revision := transitionTokenFixture(t)
	token.ExecutionAuthorized = true
	_, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{
		PreflightRequest: request,
		Revision:         revision,
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
	revision := TransitionRevision{LeaderEpoch: 3, JournalSequence: 9, ResourceVersion: "rv-standalone-9"}
	token, err := BuildLifecycleTransitionToken(LifecycleTransitionTokenRequest{
		Preflight:        preflight,
		PreflightRequest: request,
		Revision:         revision,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleTransitionToken() error = %v", err)
	}
	if _, err := RevalidateLifecycleTransition(token, LifecycleTransitionObservation{PreflightRequest: request, Revision: revision}); err != nil {
		t.Fatalf("RevalidateLifecycleTransition() error = %v", err)
	}
}
