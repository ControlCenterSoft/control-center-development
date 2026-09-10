package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrInvalidTransitionRevisionEvidence = errors.New("invalid HA transition revision evidence")
	ErrStaleTransitionRevisionEvidence   = errors.New("stale HA transition revision evidence")
)

// ReconcilerRevisionObservation is the narrow adapter input expected from the
// reconciler/state-store boundary. Leader epoch and journal sequence are never
// caller supplied: this package derives them from an exact, CAS-linked chain of
// validated cluster observations.
type ReconcilerRevisionObservation struct {
	Membership              Snapshot `json:"membership"`
	ExpectedResourceVersion string   `json:"expected_resource_version,omitempty"`
	ResourceVersion         string   `json:"resource_version"`
}

// TransitionRevisionEvidence is sealed HA revision evidence. Its fields are
// intentionally private so callers cannot fabricate leader epochs, journal
// sequence numbers, topology identity, or state-store lineage directly.
type TransitionRevisionEvidence struct {
	evidenceID           string
	clusterID            string
	generation           uint64
	leaderID             string
	topologyID           string
	membershipSnapshotID string
	revision             TransitionRevision
}

// BootstrapTransitionRevisionEvidence starts a revision chain from one exact
// reconciler observation. Bootstrap establishes epoch/sequence 1 and does not
// authorize any lifecycle or role mutation.
func BootstrapTransitionRevisionEvidence(observed ReconcilerRevisionObservation) (TransitionRevisionEvidence, error) {
	if observed.ExpectedResourceVersion != "" {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: bootstrap must not declare an expected resource version", ErrInvalidTransitionRevisionEvidence)
	}
	if err := validateResourceVersion(observed.ResourceVersion); err != nil {
		return TransitionRevisionEvidence{}, err
	}
	if _, _, err := validateSnapshot(observed.Membership); err != nil {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: membership snapshot: %v", ErrInvalidTransitionRevisionEvidence, err)
	}

	return newTransitionRevisionEvidence(observed.Membership, TransitionRevision{
		LeaderEpoch:     1,
		JournalSequence: 1,
		ResourceVersion: observed.ResourceVersion,
	})
}

// AdvanceTransitionRevisionEvidence admits the next reconciler observation
// only when it is CAS-linked to the exact previous state-store version. An
// identical re-read is idempotent. Any accepted new resource version advances
// the HA journal exactly once; leader changes also advance the leader epoch.
func AdvanceTransitionRevisionEvidence(current TransitionRevisionEvidence, observed ReconcilerRevisionObservation) (TransitionRevisionEvidence, error) {
	if err := validateTransitionRevisionEvidence(current); err != nil {
		return TransitionRevisionEvidence{}, err
	}
	if err := validateResourceVersion(observed.ResourceVersion); err != nil {
		return TransitionRevisionEvidence{}, err
	}
	if _, _, err := validateSnapshot(observed.Membership); err != nil {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: membership snapshot: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	if observed.Membership.ClusterID != current.clusterID {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: cluster identity changed", ErrStaleTransitionRevisionEvidence)
	}
	if observed.ExpectedResourceVersion != current.revision.ResourceVersion {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: expected resource version %q does not match current %q", ErrStaleTransitionRevisionEvidence, observed.ExpectedResourceVersion, current.revision.ResourceVersion)
	}

	snapshotID, err := membershipSnapshotID(observed.Membership)
	if err != nil {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: membership snapshot: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	topologyID, err := membershipTopologyID(observed.Membership)
	if err != nil {
		return TransitionRevisionEvidence{}, err
	}

	if observed.ResourceVersion == current.revision.ResourceVersion {
		if snapshotID == current.membershipSnapshotID && topologyID == current.topologyID && observed.Membership.Generation == current.generation && observed.Membership.LeaderID == current.leaderID {
			return current, nil
		}
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: state changed without a new resource version", ErrStaleTransitionRevisionEvidence)
	}

	topologyChanged := topologyID != current.topologyID
	if topologyChanged && current.generation == math.MaxUint64 {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: membership generation cannot advance", ErrInvalidTransitionRevisionEvidence)
	}
	switch {
	case topologyChanged && observed.Membership.Generation != current.generation+1:
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: topology change must advance generation exactly once", ErrStaleTransitionRevisionEvidence)
	case !topologyChanged && observed.Membership.Generation != current.generation:
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: generation changed without a topology change", ErrStaleTransitionRevisionEvidence)
	}

	if current.revision.JournalSequence == math.MaxUint64 {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: journal sequence cannot advance", ErrInvalidTransitionRevisionEvidence)
	}
	revision := current.revision
	revision.JournalSequence++
	revision.ResourceVersion = observed.ResourceVersion
	if observed.Membership.LeaderID != current.leaderID {
		if current.revision.LeaderEpoch == math.MaxUint64 {
			return TransitionRevisionEvidence{}, fmt.Errorf("%w: leader epoch cannot advance", ErrInvalidTransitionRevisionEvidence)
		}
		revision.LeaderEpoch++
	}

	return newTransitionRevisionEvidence(observed.Membership, revision)
}

func (evidence TransitionRevisionEvidence) EvidenceID() string { return evidence.evidenceID }

func (evidence TransitionRevisionEvidence) Revision() TransitionRevision { return evidence.revision }

func (evidence TransitionRevisionEvidence) ClusterID() string { return evidence.clusterID }

func (evidence TransitionRevisionEvidence) Generation() uint64 { return evidence.generation }

func (evidence TransitionRevisionEvidence) LeaderID() string { return evidence.leaderID }

func (evidence TransitionRevisionEvidence) MembershipSnapshotID() string {
	return evidence.membershipSnapshotID
}

func newTransitionRevisionEvidence(snapshot Snapshot, revision TransitionRevision) (TransitionRevisionEvidence, error) {
	if err := validateTransitionRevision(revision); err != nil {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	snapshotID, err := membershipSnapshotID(snapshot)
	if err != nil {
		return TransitionRevisionEvidence{}, fmt.Errorf("%w: membership snapshot: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	topologyID, err := membershipTopologyID(snapshot)
	if err != nil {
		return TransitionRevisionEvidence{}, err
	}
	evidence := TransitionRevisionEvidence{
		clusterID:            snapshot.ClusterID,
		generation:           snapshot.Generation,
		leaderID:             snapshot.LeaderID,
		topologyID:           topologyID,
		membershipSnapshotID: snapshotID,
		revision:             revision,
	}
	evidenceID, err := transitionRevisionEvidenceID(evidence)
	if err != nil {
		return TransitionRevisionEvidence{}, err
	}
	evidence.evidenceID = evidenceID
	return evidence, nil
}

func validateTransitionRevisionEvidence(evidence TransitionRevisionEvidence) error {
	if evidence.evidenceID == "" || evidence.clusterID == "" || evidence.generation == 0 || evidence.leaderID == "" || evidence.topologyID == "" || evidence.membershipSnapshotID == "" {
		return fmt.Errorf("%w: evidence identity is incomplete", ErrInvalidTransitionRevisionEvidence)
	}
	for field, value := range map[string]string{
		"cluster_id":             evidence.clusterID,
		"leader_id":              evidence.leaderID,
		"topology_id":            evidence.topologyID,
		"membership_snapshot_id": evidence.membershipSnapshotID,
		"evidence_id":            evidence.evidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidTransitionRevisionEvidence, err)
		}
	}
	if err := validateTransitionRevision(evidence.revision); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	expectedID, err := transitionRevisionEvidenceID(evidence)
	if err != nil {
		return err
	}
	if expectedID != evidence.evidenceID {
		return fmt.Errorf("%w: evidence fingerprint mismatch", ErrInvalidTransitionRevisionEvidence)
	}
	return nil
}

func validateResourceVersion(resourceVersion string) error {
	if err := validateIdentifier("resource_version", resourceVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	return nil
}

func membershipTopologyID(snapshot Snapshot) (string, error) {
	if _, _, err := validateSnapshot(snapshot); err != nil {
		return "", fmt.Errorf("%w: membership topology: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	type topologyMember struct {
		ID   string     `json:"id"`
		Kind MemberKind `json:"kind"`
	}
	members := make([]topologyMember, 0, len(snapshot.Members))
	for _, member := range snapshot.Members {
		members = append(members, topologyMember{ID: member.ID, Kind: member.Kind})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	input := struct {
		ClusterID string           `json:"cluster_id"`
		Members   []topologyMember `json:"members"`
	}{
		ClusterID: snapshot.ClusterID,
		Members:   members,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: topology fingerprint: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	digest := sha256.Sum256(encoded)
	return "chtop-" + hex.EncodeToString(digest[:])[:24], nil
}

func transitionRevisionEvidenceID(evidence TransitionRevisionEvidence) (string, error) {
	input := struct {
		ClusterID            string             `json:"cluster_id"`
		Generation           uint64             `json:"generation"`
		LeaderID             string             `json:"leader_id"`
		TopologyID           string             `json:"topology_id"`
		MembershipSnapshotID string             `json:"membership_snapshot_id"`
		Revision             TransitionRevision `json:"revision"`
	}{
		ClusterID:            evidence.clusterID,
		Generation:           evidence.generation,
		LeaderID:             evidence.leaderID,
		TopologyID:           evidence.topologyID,
		MembershipSnapshotID: evidence.membershipSnapshotID,
		Revision:             evidence.revision,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: evidence fingerprint: %v", ErrInvalidTransitionRevisionEvidence, err)
	}
	digest := sha256.Sum256(encoded)
	return "chre-" + hex.EncodeToString(digest[:])[:24], nil
}
