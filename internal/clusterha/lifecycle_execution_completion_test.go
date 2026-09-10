package clusterha

import (
	"errors"
	"testing"
)

func lifecycleExecutionCompletionFixture(
	t *testing.T,
	standalone bool,
) (LifecycleExecutionCompletionReceipt, LifecycleExecutionCompletionObservation) {
	t.Helper()
	admission, source := lifecycleExecutionAdmissionFixture(t, standalone)
	post := source.LifecycleCAS
	post.State = admission.TargetState
	post.Generation = admission.PlannedLifecycleGeneration
	post.ResourceVersion = "rv-lifecycle-completed-8"
	observation := LifecycleExecutionCompletionObservation{
		Admission:         admission,
		Source:            source,
		PostCAS:           post,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
	}
	receipt, err := BuildLifecycleExecutionCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionCompletionReceipt() error = %v", err)
	}
	return receipt, observation
}

func TestBuildLifecycleExecutionCompletionReceiptIsDeterministicAndBounded(t *testing.T) {
	first, observation := lifecycleExecutionCompletionFixture(t, false)
	second, err := BuildLifecycleExecutionCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionCompletionReceipt(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("receipt is not deterministic: first=%+v second=%+v", first, second)
	}
	if !first.CompletionVerified || first.RecoveryRequired || first.FurtherLifecycleAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("unsafe completion flags: %+v", first)
	}
}

func TestBuildLifecycleExecutionCompletionReceiptPreservesStandaloneBoundary(t *testing.T) {
	receipt, observation := lifecycleExecutionCompletionFixture(t, true)
	if !receipt.StandaloneDowntimeBound {
		t.Fatalf("standalone downtime evidence was lost: %+v", receipt)
	}
	if err := RevalidateLifecycleExecutionCompletionReceipt(receipt, observation); err != nil {
		t.Fatalf("RevalidateLifecycleExecutionCompletionReceipt() error = %v", err)
	}
}

func TestBuildLifecycleExecutionCompletionReceiptRejectsIncompleteTransition(t *testing.T) {
	_, observation := lifecycleExecutionCompletionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "old-state", mutate: func(post *LifecycleCASObservation) { post.State = observation.Admission.FromState }},
		{name: "old-generation", mutate: func(post *LifecycleCASObservation) {
			post.Generation = observation.Admission.ExpectedLifecycleGeneration
		}},
		{name: "old-resource-version", mutate: func(post *LifecycleCASObservation) {
			post.ResourceVersion = observation.Admission.ExpectedLifecycleResourceVersion
		}},
		{name: "wrong-node", mutate: func(post *LifecycleCASObservation) { post.NodeID = "controller-z" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			changed.PostCAS = observation.PostCAS
			test.mutate(&changed.PostCAS)
			_, err := BuildLifecycleExecutionCompletionReceipt(changed)
			if !errors.Is(err, ErrLifecycleExecutionRecoveryRequired) {
				t.Fatalf("error = %v, want ErrLifecycleExecutionRecoveryRequired", err)
			}
		})
	}
}

func TestBuildLifecycleExecutionCompletionReceiptRejectsHAJournalMovement(t *testing.T) {
	_, observation := lifecycleExecutionCompletionFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-completion-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleExecutionCompletionReceipt(observation)
	if !errors.Is(err, ErrStaleLifecycleExecutionCompletion) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionCompletion", err)
	}
}

func TestRevalidateLifecycleExecutionCompletionReceiptRejectsTampering(t *testing.T) {
	receipt, observation := lifecycleExecutionCompletionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleExecutionCompletionReceipt)
	}{
		{name: "authorize-next", mutate: func(r *LifecycleExecutionCompletionReceipt) { r.FurtherLifecycleAuthorized = true }},
		{name: "authorize-failover", mutate: func(r *LifecycleExecutionCompletionReceipt) { r.FailoverAuthorized = true }},
		{name: "resource-version", mutate: func(r *LifecycleExecutionCompletionReceipt) { r.AppliedLifecycleResourceVersion = "rv-tampered" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			err := RevalidateLifecycleExecutionCompletionReceipt(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleExecutionCompletion) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleExecutionCompletion", err)
			}
		})
	}
}

func TestRevalidateLifecycleExecutionCompletionReceiptRejectsPostCASDrift(t *testing.T) {
	receipt, observation := lifecycleExecutionCompletionFixture(t, false)
	observation.PostCAS.ResourceVersion = "rv-lifecycle-completed-9"
	err := RevalidateLifecycleExecutionCompletionReceipt(receipt, observation)
	if !errors.Is(err, ErrStaleLifecycleExecutionCompletion) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionCompletion", err)
	}
}
