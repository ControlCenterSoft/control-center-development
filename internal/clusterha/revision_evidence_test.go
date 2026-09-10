package clusterha

import (
	"errors"
	"testing"
)

func revisionEvidenceCluster() Snapshot {
	return Snapshot{
		ClusterID:  "cluster-a",
		Generation: 7,
		LeaderID:   "controller-a",
		Members: []Member{
			{ID: "controller-a", Kind: MemberController, Healthy: true, CaughtUp: true},
			{ID: "controller-b", Kind: MemberController, Healthy: true, CaughtUp: true},
			{ID: "controller-c", Kind: MemberController, Healthy: true, CaughtUp: true},
		},
	}
}

func bootstrapRevisionEvidence(t *testing.T) TransitionRevisionEvidence {
	t.Helper()
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      revisionEvidenceCluster(),
		ResourceVersion: "rv-100",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	return evidence
}

func TestBootstrapTransitionRevisionEvidenceDerivesInitialRevision(t *testing.T) {
	evidence := bootstrapRevisionEvidence(t)
	if evidence.EvidenceID() == "" || evidence.MembershipSnapshotID() == "" {
		t.Fatalf("missing evidence identity: %+v", evidence)
	}
	if evidence.ClusterID() != "cluster-a" || evidence.Generation() != 7 || evidence.LeaderID() != "controller-a" {
		t.Fatalf("unexpected cluster binding: %+v", evidence)
	}
	want := (TransitionRevision{LeaderEpoch: 1, JournalSequence: 1, ResourceVersion: "rv-100"})
	if evidence.Revision() != want {
		t.Fatalf("revision = %+v, want %+v", evidence.Revision(), want)
	}
}

func TestBootstrapTransitionRevisionEvidenceIsDeterministic(t *testing.T) {
	first := bootstrapRevisionEvidence(t)
	snapshot := revisionEvidenceCluster()
	snapshot.Members[0], snapshot.Members[2] = snapshot.Members[2], snapshot.Members[0]
	second, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{Membership: snapshot, ResourceVersion: "rv-100"})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence(second) error = %v", err)
	}
	if first.EvidenceID() != second.EvidenceID() || first.MembershipSnapshotID() != second.MembershipSnapshotID() {
		t.Fatalf("evidence is not deterministic: first=%+v second=%+v", first, second)
	}
}

func TestBootstrapTransitionRevisionEvidenceSupportsStandalone(t *testing.T) {
	snapshot := Snapshot{
		ClusterID:  "cluster-standalone",
		Generation: 1,
		LeaderID:   "controller-a",
		Members: []Member{
			{ID: "controller-a", Kind: MemberController, Healthy: true, CaughtUp: true},
		},
	}
	evidence, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      snapshot,
		ResourceVersion: "rv-standalone-1",
	})
	if err != nil {
		t.Fatalf("BootstrapTransitionRevisionEvidence() error = %v", err)
	}
	if evidence.Generation() != 1 || evidence.LeaderID() != "controller-a" || evidence.Revision().LeaderEpoch != 1 {
		t.Fatalf("unexpected standalone evidence: %+v", evidence)
	}
}

func TestAdvanceTransitionRevisionEvidenceRecordsPartialFailureWithoutInventingFailover(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.Members[1].Healthy = false
	observed.Members[1].CaughtUp = false
	observed.Members[2].Healthy = false
	observed.Members[2].CaughtUp = false
	next, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	if next.LeaderID() != "controller-a" || next.Revision().LeaderEpoch != 1 || next.Revision().JournalSequence != 2 {
		t.Fatalf("partial failure invented a leader transition: %+v", next)
	}
}

func TestAdvanceTransitionRevisionEvidenceIsIdempotentForExactReread(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	next, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              revisionEvidenceCluster(),
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-100",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	if next.EvidenceID() != current.EvidenceID() || next.Revision() != current.Revision() {
		t.Fatalf("exact re-read changed evidence: current=%+v next=%+v", current, next)
	}
}

func TestAdvanceTransitionRevisionEvidenceRecordsHealthDrift(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.Members[2].Healthy = false
	observed.Members[2].CaughtUp = false
	next, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	if next.Revision().LeaderEpoch != 1 || next.Revision().JournalSequence != 2 || next.Revision().ResourceVersion != "rv-101" {
		t.Fatalf("unexpected revision after health drift: %+v", next.Revision())
	}
	if next.MembershipSnapshotID() == current.MembershipSnapshotID() || next.EvidenceID() == current.EvidenceID() {
		t.Fatalf("health drift did not invalidate evidence")
	}
}

func TestAdvanceTransitionRevisionEvidenceRecordsLeaderFailover(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.LeaderID = "controller-b"
	next, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	if next.Revision().LeaderEpoch != 2 || next.Revision().JournalSequence != 2 {
		t.Fatalf("failover did not advance epoch/journal: %+v", next.Revision())
	}
	if next.LeaderID() != "controller-b" {
		t.Fatalf("leader binding = %q, want controller-b", next.LeaderID())
	}
}

func TestAdvanceTransitionRevisionEvidenceRequiresCASLineage(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	_, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              revisionEvidenceCluster(),
		ExpectedResourceVersion: "rv-stale",
		ResourceVersion:         "rv-101",
	})
	if !errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		t.Fatalf("error = %v, want ErrStaleTransitionRevisionEvidence", err)
	}
}

func TestAdvanceTransitionRevisionEvidenceRejectsStateChangeWithoutNewResourceVersion(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.LeaderID = "controller-b"
	_, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-100",
	})
	if !errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		t.Fatalf("error = %v, want ErrStaleTransitionRevisionEvidence", err)
	}
}

func TestAdvanceTransitionRevisionEvidenceRequiresExactMembershipGenerationStep(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.Generation = 9
	observed.Members = []Member{
		{ID: "controller-a", Kind: MemberController, Healthy: true, CaughtUp: true},
	}
	_, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if !errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		t.Fatalf("error = %v, want ErrStaleTransitionRevisionEvidence", err)
	}
}

func TestAdvanceTransitionRevisionEvidenceRejectsGenerationDriftWithoutTopologyChange(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.Generation++
	_, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if !errors.Is(err, ErrStaleTransitionRevisionEvidence) {
		t.Fatalf("error = %v, want ErrStaleTransitionRevisionEvidence", err)
	}
}

func TestAdvanceTransitionRevisionEvidenceAcceptsSingleStepTopologyChange(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	observed := revisionEvidenceCluster()
	observed.Generation++
	observed.Members = []Member{{ID: "controller-a", Kind: MemberController, Healthy: true, CaughtUp: true}}
	next, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              observed,
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	if next.Generation() != current.Generation()+1 || next.Revision().JournalSequence != current.Revision().JournalSequence+1 {
		t.Fatalf("unexpected topology transition: current=%+v next=%+v", current, next)
	}
}

func TestAdvanceTransitionRevisionEvidenceRejectsTamperedCurrentEvidence(t *testing.T) {
	current := bootstrapRevisionEvidence(t)
	current.revision.JournalSequence++
	_, err := AdvanceTransitionRevisionEvidence(current, ReconcilerRevisionObservation{
		Membership:              revisionEvidenceCluster(),
		ExpectedResourceVersion: "rv-100",
		ResourceVersion:         "rv-101",
	})
	if !errors.Is(err, ErrInvalidTransitionRevisionEvidence) {
		t.Fatalf("error = %v, want ErrInvalidTransitionRevisionEvidence", err)
	}
}

func TestBootstrapTransitionRevisionEvidenceRejectsInvalidBoundaryInput(t *testing.T) {
	_, err := BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:              revisionEvidenceCluster(),
		ExpectedResourceVersion: "rv-old",
		ResourceVersion:         "rv-100",
	})
	if !errors.Is(err, ErrInvalidTransitionRevisionEvidence) {
		t.Fatalf("bootstrap expected-resource error = %v, want ErrInvalidTransitionRevisionEvidence", err)
	}
	_, err = BootstrapTransitionRevisionEvidence(ReconcilerRevisionObservation{
		Membership:      revisionEvidenceCluster(),
		ResourceVersion: "bad version",
	})
	if !errors.Is(err, ErrInvalidTransitionRevisionEvidence) {
		t.Fatalf("bootstrap resource-version error = %v, want ErrInvalidTransitionRevisionEvidence", err)
	}
}
