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

// TransitionRevision identifies the exact observed HA state used by a lifecycle
// preflight. LeaderEpoch invalidates decisions across elections even when the
// elected node does not change. JournalSequence invalidates decisions after any
// recorded HA transition. ResourceVersion is the reconciler/state-store CAS
// version and prevents executing against a superseded observation.
type TransitionRevision struct {
	LeaderEpoch     uint64 `json:"leader_epoch"`
	JournalSequence uint64 `json:"journal_sequence"`
	ResourceVersion string `json:"resource_version"`
}

// LifecycleTransitionToken binds an accepted, mutation-free lifecycle preflight
// to an exact membership snapshot and HA revision. The token is evidence only;
// it never grants execution authority or performs host/state mutation.
type LifecycleTransitionToken struct {
	TokenID              string             `json:"token_id"`
	PreflightPlanID      string             `json:"preflight_plan_id"`
	ClusterID            string             `json:"cluster_id"`
	ClusterGeneration    uint64             `json:"cluster_generation"`
	MembershipSnapshotID string             `json:"membership_snapshot_id"`
	Revision             TransitionRevision `json:"revision"`
	PlanOnly             bool               `json:"plan_only"`
	ExecutionAuthorized  bool               `json:"execution_authorized"`
	HostMutation         bool               `json:"host_mutation"`
	StateMutation        bool               `json:"state_mutation"`
}

type LifecycleTransitionTokenRequest struct {
	Preflight        LifecyclePreflightPlan    `json:"preflight"`
	PreflightRequest LifecyclePreflightRequest `json:"preflight_request"`
	Revision         TransitionRevision        `json:"revision"`
}

// LifecycleTransitionObservation is the trusted, current reconciler-owned view
// presented immediately before an executor handoff. Revalidation is fail-closed
// if any membership, leader epoch, journal sequence or resource version changed.
type LifecycleTransitionObservation struct {
	PreflightRequest LifecyclePreflightRequest `json:"preflight_request"`
	Revision         TransitionRevision        `json:"revision"`
}

type LifecycleTransitionValidation struct {
	TokenID              string             `json:"token_id"`
	PreflightPlanID      string             `json:"preflight_plan_id"`
	ClusterID            string             `json:"cluster_id"`
	ClusterGeneration    uint64             `json:"cluster_generation"`
	MembershipSnapshotID string             `json:"membership_snapshot_id"`
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
	if err := validateTransitionRevision(request.Revision); err != nil {
		return LifecycleTransitionToken{}, err
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
	tokenID, err := lifecycleTransitionTokenID(request.Preflight, snapshotID, request.Revision)
	if err != nil {
		return LifecycleTransitionToken{}, err
	}

	return LifecycleTransitionToken{
		TokenID:              tokenID,
		PreflightPlanID:      request.Preflight.PlanID,
		ClusterID:            request.Preflight.ClusterID,
		ClusterGeneration:    request.Preflight.ClusterGeneration,
		MembershipSnapshotID: snapshotID,
		Revision:             request.Revision,
		PlanOnly:             true,
		ExecutionAuthorized:  false,
		HostMutation:         false,
		StateMutation:        false,
	}, nil
}

// RevalidateLifecycleTransition proves that the exact preflight decision is
// still current. It deliberately returns no command, callback or privileged
// operation. A caller must use a separate audited executor after qualification.
func RevalidateLifecycleTransition(token LifecycleTransitionToken, observed LifecycleTransitionObservation) (LifecycleTransitionValidation, error) {
	if err := validateLifecycleTransitionToken(token); err != nil {
		return LifecycleTransitionValidation{}, err
	}
	if err := validateTransitionRevision(observed.Revision); err != nil {
		return LifecycleTransitionValidation{}, err
	}
	if observed.Revision != token.Revision {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: HA revision changed", ErrStaleLifecycleTransitionToken)
	}

	rebuilt, err := BuildLifecyclePreflight(observed.PreflightRequest)
	if err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current lifecycle preflight is no longer safe: %v", ErrStaleLifecycleTransitionToken, err)
	}
	if rebuilt.PlanID != token.PreflightPlanID || rebuilt.ClusterID != token.ClusterID || rebuilt.ClusterGeneration != token.ClusterGeneration {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: lifecycle preflight identity changed", ErrStaleLifecycleTransitionToken)
	}

	snapshotID, err := membershipSnapshotID(observed.PreflightRequest.Membership)
	if err != nil {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: current membership snapshot is invalid: %v", ErrStaleLifecycleTransitionToken, err)
	}
	if snapshotID != token.MembershipSnapshotID {
		return LifecycleTransitionValidation{}, fmt.Errorf("%w: membership snapshot changed", ErrStaleLifecycleTransitionToken)
	}

	return LifecycleTransitionValidation{
		TokenID:              token.TokenID,
		PreflightPlanID:      token.PreflightPlanID,
		ClusterID:            token.ClusterID,
		ClusterGeneration:    token.ClusterGeneration,
		MembershipSnapshotID: token.MembershipSnapshotID,
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
	if token.TokenID == "" || token.PreflightPlanID == "" || token.ClusterID == "" || token.ClusterGeneration == 0 || token.MembershipSnapshotID == "" {
		return fmt.Errorf("%w: token identity is incomplete", ErrInvalidLifecycleTransitionToken)
	}
	if !token.PlanOnly || token.ExecutionAuthorized || token.HostMutation || token.StateMutation {
		return fmt.Errorf("%w: token safety flags are invalid", ErrInvalidLifecycleTransitionToken)
	}
	if err := validateTransitionRevision(token.Revision); err != nil {
		return err
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

func lifecycleTransitionTokenID(preflight LifecyclePreflightPlan, snapshotID string, revision TransitionRevision) (string, error) {
	input := struct {
		PreflightPlanID      string             `json:"preflight_plan_id"`
		ClusterID            string             `json:"cluster_id"`
		ClusterGeneration    uint64             `json:"cluster_generation"`
		MembershipSnapshotID string             `json:"membership_snapshot_id"`
		Revision             TransitionRevision `json:"revision"`
	}{
		PreflightPlanID:      preflight.PlanID,
		ClusterID:            preflight.ClusterID,
		ClusterGeneration:    preflight.ClusterGeneration,
		MembershipSnapshotID: snapshotID,
		Revision:             revision,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: token fingerprint: %v", ErrInvalidLifecycleTransitionToken, err)
	}
	digest := sha256.Sum256(encoded)
	return "cht-" + hex.EncodeToString(digest[:])[:24], nil
}
