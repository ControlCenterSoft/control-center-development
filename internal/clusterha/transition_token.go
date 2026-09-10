package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrInvalidLifecycleTransitionToken = errors.New("invalid cluster lifecycle transition token")
	ErrStaleLifecycleTransitionToken   = errors.New("stale cluster lifecycle transition token")
)

// LifecycleTransitionToken binds an accepted, mutation-free lifecycle preflight
// to exact sealed HA revision evidence. The token is evidence only; it never
// grants execution authority or performs host/state mutation.
type LifecycleTransitionToken struct {
	TokenID              string             `json:"token_id"`
	PreflightPlanID      string             `json:"preflight_plan_id"`
	ClusterID            string             `json:"cluster_id"`
	ClusterGeneration    uint64             `json:"cluster_generation"`
	MembershipSnapshotID string             `json:"membership_snapshot_id"`
	RevisionEvidenceID   string             `json:"revision_evidence_id"`
	Revision             TransitionRevision `json:"revision"`
	PlanOnly             bool               `json:"plan_only"`
	ExecutionAuthorized  bool               `json:"execution_authorized"`
	HostMutation         bool               `json:"host_mutation"`
	StateMutation        bool               `json:"state_mutation"`
}

type LifecycleTransitionTokenRequest struct {
	Preflight        LifecyclePreflightPlan     `json:"preflight"`
	PreflightRequest LifecyclePreflightRequest  `json:"preflight_request"`
	RevisionEvidence TransitionRevisionEvidence `json:"-"`
}

// LifecycleTransitionObservation is the trusted, current reconciler-owned view
// presented immediately before an executor handoff. Revalidation is fail-closed
// if the sealed evidence identity, membership, leader, journal or CAS revision
// changed.
type LifecycleTransitionObservation struct {
	PreflightRequest LifecyclePreflightRequest  `json:"preflight_request"`
	RevisionEvidence TransitionRevisionEvidence `json:"-"`
}

type LifecycleTransitionValidation struct {
	TokenID              string             `json:"token_id"`
	PreflightPlanID      string             `json:"preflight_plan_id"`
	ClusterID            string             `json:"cluster_id"`
	ClusterGeneration    uint64             `json:"cluster_generation"`
	MembershipSnapshotID string             `json:"membership_snapshot_id"`
	RevisionEvidenceID   string             `json:"revision_evidence_id"`
	Revision             TransitionRevision `json:"revision"`
	Revalidated          bool               `json:"revalidated"`
	PlanOnly             bool               `json:"plan_only"`
	ExecutionAuthorized  bool               `json:"execution_authorized"`
	HostMutation         bool               `json:"host_mutation"`
	StateMutation        bool               `json:"state_mutation"`
}

func BuildLifecycleTransitionToken(request LifecycleTransitionTokenRequest) (LifecycleTransitionToken, error) {
	if err := validatePreflightEvidence(request.Preflight); err != nil {
		return LifecycleTransitionToken{}, err
	}
	if err := validateTransitionRevisionEvidence(request.RevisionEvidence); err != nil {
		return LifecycleTransitionToken{}, fmt.Errorf("%w: revision evidence: %v", ErrInvalidLifecycleTransitionToken, err)
	}

	rebuilt, err := BuildLifecyclePreflight(request.PreflightRequest)
	if err != nil {
		return LifecycleTransitionToken{}, fmt.Errorf("%w: preflight request no longer validates: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	if !sameLifecyclePreflightEvidence(request.Preflight, rebuilt) {
		return LifecycleTransitionToken{}, fmt.Errorf("%w: preflight evidence does not match the supplied preflight request", ErrInvalidLifecycleTransitionToken)
	}

	snapshotID, err := membershipSnapshotID(request.PreflightRequest.Membership)
	if err != nil {
		return LifecycleTransitionToken{}, err
	}
	if err := validateRevisionEvidenceBinding(request.RevisionEvidence, request.PreflightRequest.Membership, snapshotID); err != nil {
		return LifecycleTransitionToken{}, fmt.Errorf("%w: %v", ErrInvalidLifecycleTransitionToken, err)
	}

	revision := request.RevisionEvidence.Revision()
	tokenID, err := lifecycleTransitionTokenID(
		request.Preflight.PlanID,
		request.Preflight.ClusterID,
		request.Preflight.ClusterGeneration,
		snapshotID,
		request.RevisionEvidence.EvidenceID(),
		revision,
	)
	if err != nil {
		return LifecycleTransitionToken{}, err
	}

	return LifecycleTransitionToken{
		TokenID:              tokenID,
		PreflightPlanID:      request.Preflight.PlanID,
		ClusterID:            request.Preflight.ClusterID,
		ClusterGeneration:    request.Preflight.ClusterGeneration,
		MembershipSnapshotID: snapshotID,
		RevisionEvidenceID:   request.RevisionEvidence.EvidenceID(),
		Revision:             revision,
		PlanOnly:             true,
		ExecutionAuthorized:  false,
		HostMutation:         false,
		StateMutation:        false,
	}, nil
}

// RevalidateLifecycleTransition proves that the exact preflight decision and
// exact sealed HA revision evidence are still current. It deliberately returns
// no command, callback or privileged operation.
func RevalidateLifecycleTransition(token LifecycleTransitionToken, observed LifecycleTransitionObservation) (LifecycleTransitionValidation, error) {
	if err := validateLifecycleTransitionToken(token); err != nil {
		return LifecycleTransitionValidation{}, err
	}
	if err := validateTransitionRevisionEvidence(observed.RevisionEvidence); err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current revision evidence is invalid: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	if observed.RevisionEvidence.EvidenceID() != token.RevisionEvidenceID || observed.RevisionEvidence.Revision() != token.Revision {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: HA revision evidence changed", ErrStaleLifecycleTransitionToken)
	}

	snapshotID, err := membershipSnapshotID(observed.PreflightRequest.Membership)
	if err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current membership snapshot is invalid: %v", ErrStaleLifecycleTransitionToken, err)
	}
	if err := validateRevisionEvidenceBinding(observed.RevisionEvidence, observed.PreflightRequest.Membership, snapshotID); err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current revision evidence no longer matches membership: %v", ErrStaleLifecycleTransitionToken, err)
	}
	if snapshotID != token.MembershipSnapshotID {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: membership snapshot changed", ErrStaleLifecycleTransitionToken)
	}

	rebuilt, err := BuildLifecyclePreflight(observed.PreflightRequest)
	if err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current lifecycle preflight is no longer safe: %v", ErrStaleLifecycleTransitionToken, err)
	}
	if rebuilt.PlanID != token.PreflightPlanID || rebuilt.ClusterID != token.ClusterID || rebuilt.ClusterGeneration != token.ClusterGeneration {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: lifecycle preflight identity changed", ErrStaleLifecycleTransitionToken)
	}

	return LifecycleTransitionValidation{
		TokenID:              token.TokenID,
		PreflightPlanID:      token.PreflightPlanID,
		ClusterID:            token.ClusterID,
		ClusterGeneration:    token.ClusterGeneration,
		MembershipSnapshotID: token.MembershipSnapshotID,
		RevisionEvidenceID:   token.RevisionEvidenceID,
		Revision:             token.Revision,
		Revalidated:          true,
		PlanOnly:             true,
		ExecutionAuthorized:  false,
		HostMutation:         false,
		StateMutation:        false,
	}, nil
}

func validatePreflightEvidence(preflight LifecyclePreflightPlan) error {
	if !preflight.Accepted || !preflight.PlanOnly || preflight.HostMutation || preflight.StateMutation {
		return fmt.Errorf("%w: preflight must be accepted, plan-only and mutation-free", ErrInvalidLifecycleTransitionToken)
	}
	if preflight.PlanID == "" || preflight.LifecyclePlanID == "" || preflight.NodeID == "" || preflight.ClusterID == "" || preflight.ClusterGeneration == 0 {
		return fmt.Errorf("%w: preflight identity is incomplete", ErrInvalidLifecycleTransitionToken)
	}
	return nil
}

func validateLifecycleTransitionToken(token LifecycleTransitionToken) error {
	if token.TokenID == "" || token.PreflightPlanID == "" || token.ClusterID == "" || token.ClusterGeneration == 0 || token.MembershipSnapshotID == "" || token.RevisionEvidenceID == "" {
		return fmt.Errorf("%w: token identity is incomplete", ErrInvalidLifecycleTransitionToken)
	}
	if !token.PlanOnly || token.ExecutionAuthorized || token.HostMutation || token.StateMutation {
		return fmt.Errorf("%w: token safety flags are invalid", ErrInvalidLifecycleTransitionToken)
	}
	if err := validateTransitionRevision(token.Revision); err != nil {
		return err
	}
	if err := validateIdentifier("revision_evidence_id", token.RevisionEvidenceID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	expectedID, err := lifecycleTransitionTokenID(
		token.PreflightPlanID,
		token.ClusterID,
		token.ClusterGeneration,
		token.MembershipSnapshotID,
		token.RevisionEvidenceID,
		token.Revision,
	)
	if err != nil {
		return err
	}
	if expectedID != token.TokenID {
		return fmt.Errorf("%w: token fingerprint mismatch", ErrInvalidLifecycleTransitionToken)
	}
	return nil
}

func validateTransitionRevision(revision TransitionRevision) error {
	if revision.LeaderEpoch == 0 {
		return fmt.Errorf("%w: leader_epoch must be positive", ErrInvalidLifecycleTransitionToken)
	}
	if revision.JournalSequence == 0 {
		return fmt.Errorf("%w: journal_sequence must be positive", ErrInvalidLifecycleTransitionToken)
	}
	if err := validateIdentifier("resource_version", revision.ResourceVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	return nil
}

func validateRevisionEvidenceBinding(evidence TransitionRevisionEvidence, snapshot Snapshot, snapshotID string) error {
	if evidence.ClusterID() != snapshot.ClusterID {
		return fmt.Errorf("revision evidence cluster identity changed")
	}
	if evidence.Generation() != snapshot.Generation {
		return fmt.Errorf("revision evidence generation changed")
	}
	if evidence.LeaderID() != snapshot.LeaderID {
		return fmt.Errorf("revision evidence leader changed")
	}
	if evidence.MembershipSnapshotID() != snapshotID {
		return fmt.Errorf("revision evidence membership snapshot changed")
	}
	return nil
}

func sameLifecyclePreflightEvidence(left, right LifecyclePreflightPlan) bool {
	return left.PlanID == right.PlanID &&
		left.Accepted == right.Accepted &&
		left.PlanOnly == right.PlanOnly &&
		left.HostMutation == right.HostMutation &&
		left.StateMutation == right.StateMutation &&
		left.NodeID == right.NodeID &&
		left.LifecyclePlanID == right.LifecyclePlanID &&
		left.LifecycleTarget == right.LifecycleTarget &&
		left.ClusterID == right.ClusterID &&
		left.ClusterGeneration == right.ClusterGeneration &&
		left.ControllerBound == right.ControllerBound
}

func membershipSnapshotID(snapshot Snapshot) (string, error) {
	if _, _, err := validateSnapshot(snapshot); err != nil {
		return "", fmt.Errorf("%w: membership snapshot: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	input := struct {
		ClusterID  string   `json:"cluster_id"`
		Generation uint64   `json:"generation"`
		LeaderID   string   `json:"leader_id"`
		Members    []Member `json:"members"`
	}{
		ClusterID:  snapshot.ClusterID,
		Generation: snapshot.Generation,
		LeaderID:   snapshot.LeaderID,
		Members:    canonicalMembers(snapshot.Members),
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: membership fingerprint: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	digest := sha256.Sum256(encoded)
	return "chs-" + hex.EncodeToString(digest[:])[:24], nil
}

func lifecycleTransitionTokenID(preflightPlanID, clusterID string, clusterGeneration uint64, snapshotID, revisionEvidenceID string, revision TransitionRevision) (string, error) {
	input := struct {
		PreflightPlanID      string             `json:"preflight_plan_id"`
		ClusterID            string             `json:"cluster_id"`
		ClusterGeneration    uint64             `json:"cluster_generation"`
		MembershipSnapshotID string             `json:"membership_snapshot_id"`
		RevisionEvidenceID   string             `json:"revision_evidence_id"`
		Revision             TransitionRevision `json:"revision"`
	}{
		PreflightPlanID:      preflightPlanID,
		ClusterID:            clusterID,
		ClusterGeneration:    clusterGeneration,
		MembershipSnapshotID: snapshotID,
		RevisionEvidenceID:   revisionEvidenceID,
		Revision:             revision,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: token fingerprint: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	digest := sha256.Sum256(encoded)
	return "cht-" + hex.EncodeToString(digest[:])[:24], nil
}
