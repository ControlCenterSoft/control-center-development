package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

var (
	ErrInvalidMembership  = errors.New("invalid cluster membership")
	ErrUnsafeMembership   = errors.New("unsafe cluster membership change")
	ErrNoMembershipChange = errors.New("no cluster membership change")
)

type MemberKind string

const (
	MemberController MemberKind = "controller"
	MemberWitness    MemberKind = "witness"
)

type Profile string

const (
	ProfileStandalone       Profile = "1-controller"
	ProfileTwoPlusWitness   Profile = "2-controller-plus-witness"
	ProfileThreeControllers Profile = "3-controller"
	ProfileFiveControllers  Profile = "5-controller"
)

type Check string

const (
	CheckCurrentQuorum   Check = "current-quorum"
	CheckDesiredQuorum   Check = "desired-quorum"
	CheckNewMembersReady Check = "new-members-ready"
	CheckLeaderTransfer  Check = "leader-transfer"
	CheckFencing         Check = "fencing"
)

type Member struct {
	ID       string     `json:"id"`
	Kind     MemberKind `json:"kind"`
	Healthy  bool       `json:"healthy"`
	CaughtUp bool       `json:"caught_up"`
}

type Snapshot struct {
	ClusterID  string   `json:"cluster_id"`
	Generation uint64   `json:"generation"`
	LeaderID   string   `json:"leader_id"`
	Members    []Member `json:"members"`
}

type ChangeRequest struct {
	Current                 Snapshot `json:"current"`
	DesiredGeneration       uint64   `json:"desired_generation"`
	DesiredMembers          []Member `json:"desired_members"`
	NextLeaderID            string   `json:"next_leader_id,omitempty"`
	LeaderTransferConfirmed bool     `json:"leader_transfer_confirmed"`
	FencingConfirmed        bool     `json:"fencing_confirmed"`
}

type Plan struct {
	PlanID                 string   `json:"plan_id"`
	Accepted               bool     `json:"accepted"`
	PlanOnly               bool     `json:"plan_only"`
	HostMutation           bool     `json:"host_mutation"`
	StateMutation          bool     `json:"state_mutation"`
	ClusterID              string   `json:"cluster_id"`
	CurrentGeneration      uint64   `json:"current_generation"`
	DesiredGeneration      uint64   `json:"desired_generation"`
	CurrentProfile         Profile  `json:"current_profile"`
	DesiredProfile         Profile  `json:"desired_profile"`
	CurrentQuorum          int      `json:"current_quorum"`
	DesiredQuorum          int      `json:"desired_quorum"`
	CurrentHealthyVotes    int      `json:"current_healthy_votes"`
	DesiredReadyVotes      int      `json:"desired_ready_votes"`
	AddMembers             []string `json:"add_members,omitempty"`
	RemoveMembers          []string `json:"remove_members,omitempty"`
	RequiresLeaderTransfer bool     `json:"requires_leader_transfer"`
	RequiresFencing        bool     `json:"requires_fencing"`
	RequiredChecks         []Check  `json:"required_checks"`
}

func BuildMembershipPlan(request ChangeRequest) (Plan, error) {
	currentProfile, currentByID, err := validateSnapshot(request.Current)
	if err != nil {
		return Plan{}, err
	}
	if request.Current.Generation == math.MaxUint64 {
		return Plan{}, fmt.Errorf("%w: current generation cannot advance", ErrInvalidMembership)
	}
	if request.DesiredGeneration != request.Current.Generation+1 {
		return Plan{}, fmt.Errorf("%w: desired generation must advance exactly once", ErrInvalidMembership)
	}
	if len(request.DesiredMembers) == 0 {
		return Plan{}, fmt.Errorf("%w: desired_members are required", ErrInvalidMembership)
	}
	desiredProfile, desiredByID, err := validateMembers(request.DesiredMembers)
	if err != nil {
		return Plan{}, err
	}

	currentHealthy := healthyVotes(request.Current.Members)
	currentQuorum := quorum(len(request.Current.Members))
	if currentHealthy < currentQuorum {
		return Plan{}, fmt.Errorf("%w: current quorum is lost: healthy=%d quorum=%d", ErrUnsafeMembership, currentHealthy, currentQuorum)
	}

	addMembers, removeMembers := membershipDiff(currentByID, desiredByID)
	if len(addMembers) == 0 && len(removeMembers) == 0 {
		return Plan{}, ErrNoMembershipChange
	}

	desiredQuorum := quorum(len(request.DesiredMembers))
	desiredReady := readyVotes(request.DesiredMembers)
	if desiredReady < desiredQuorum {
		return Plan{}, fmt.Errorf("%w: desired quorum is not ready: ready=%d quorum=%d", ErrUnsafeMembership, desiredReady, desiredQuorum)
	}
	for _, memberID := range addMembers {
		member := desiredByID[memberID]
		if !memberReady(member) {
			return Plan{}, fmt.Errorf("%w: new member %q is not healthy and ready", ErrUnsafeMembership, memberID)
		}
	}

	_, leaderRemoved := desiredByID[request.Current.LeaderID]
	leaderRemoved = !leaderRemoved
	if leaderRemoved {
		if !request.LeaderTransferConfirmed {
			return Plan{}, fmt.Errorf("%w: removing leader requires a confirmed leader transfer", ErrUnsafeMembership)
		}
		if request.NextLeaderID == "" || request.NextLeaderID == request.Current.LeaderID {
			return Plan{}, fmt.Errorf("%w: next_leader_id must identify a different desired controller", ErrInvalidMembership)
		}
		nextLeader, exists := desiredByID[request.NextLeaderID]
		if !exists || nextLeader.Kind != MemberController || !memberReady(nextLeader) {
			return Plan{}, fmt.Errorf("%w: next leader %q is not a ready desired controller", ErrUnsafeMembership, request.NextLeaderID)
		}
	} else if request.NextLeaderID != "" || request.LeaderTransferConfirmed {
		return Plan{}, fmt.Errorf("%w: leader transfer metadata is only valid when removing the current leader", ErrInvalidMembership)
	}

	requiresFencing := false
	for _, memberID := range removeMembers {
		member := currentByID[memberID]
		if member.Kind == MemberController && !member.Healthy {
			requiresFencing = true
			break
		}
	}
	if requiresFencing && !request.FencingConfirmed {
		return Plan{}, fmt.Errorf("%w: removing an unhealthy controller requires confirmed fencing", ErrUnsafeMembership)
	}
	if request.FencingConfirmed && !requiresFencing {
		return Plan{}, fmt.Errorf("%w: fencing confirmation has no matching unhealthy controller removal", ErrInvalidMembership)
	}

	fingerprint, err := planFingerprint(request, addMembers, removeMembers)
	if err != nil {
		return Plan{}, err
	}
	checks := []Check{CheckCurrentQuorum, CheckDesiredQuorum}
	if len(addMembers) > 0 {
		checks = append(checks, CheckNewMembersReady)
	}
	if leaderRemoved {
		checks = append(checks, CheckLeaderTransfer)
	}
	if requiresFencing {
		checks = append(checks, CheckFencing)
	}

	return Plan{
		PlanID:                 "chp-" + fingerprint[:24],
		Accepted:               true,
		PlanOnly:               true,
		HostMutation:           false,
		StateMutation:          false,
		ClusterID:              request.Current.ClusterID,
		CurrentGeneration:      request.Current.Generation,
		DesiredGeneration:      request.DesiredGeneration,
		CurrentProfile:         currentProfile,
		DesiredProfile:         desiredProfile,
		CurrentQuorum:          currentQuorum,
		DesiredQuorum:          desiredQuorum,
		CurrentHealthyVotes:    currentHealthy,
		DesiredReadyVotes:      desiredReady,
		AddMembers:             addMembers,
		RemoveMembers:          removeMembers,
		RequiresLeaderTransfer: leaderRemoved,
		RequiresFencing:        requiresFencing,
		RequiredChecks:         checks,
	}, nil
}

func validateSnapshot(snapshot Snapshot) (Profile, map[string]Member, error) {
	if err := validateIdentifier("cluster_id", snapshot.ClusterID); err != nil {
		return "", nil, err
	}
	if snapshot.Generation == 0 {
		return "", nil, fmt.Errorf("%w: generation must be positive", ErrInvalidMembership)
	}
	profile, members, err := validateMembers(snapshot.Members)
	if err != nil {
		return "", nil, err
	}
	if err := validateIdentifier("leader_id", snapshot.LeaderID); err != nil {
		return "", nil, err
	}
	leader, exists := members[snapshot.LeaderID]
	if !exists || leader.Kind != MemberController {
		return "", nil, fmt.Errorf("%w: leader %q must be a current controller", ErrInvalidMembership, snapshot.LeaderID)
	}
	if !memberReady(leader) {
		return "", nil, fmt.Errorf("%w: leader %q is not healthy and caught up", ErrUnsafeMembership, snapshot.LeaderID)
	}
	return profile, members, nil
}

func validateMembers(members []Member) (Profile, map[string]Member, error) {
	if len(members) == 0 {
		return "", nil, fmt.Errorf("%w: members are required", ErrInvalidMembership)
	}
	byID := make(map[string]Member, len(members))
	controllers := 0
	witnesses := 0
	for _, member := range members {
		if err := validateIdentifier("member.id", member.ID); err != nil {
			return "", nil, err
		}
		if _, exists := byID[member.ID]; exists {
			return "", nil, fmt.Errorf("%w: duplicate member %q", ErrInvalidMembership, member.ID)
		}
		switch member.Kind {
		case MemberController:
			controllers++
		case MemberWitness:
			witnesses++
			if member.CaughtUp {
				return "", nil, fmt.Errorf("%w: witness %q cannot declare caught_up", ErrInvalidMembership, member.ID)
			}
		default:
			return "", nil, fmt.Errorf("%w: member %q has unsupported kind %q", ErrInvalidMembership, member.ID, member.Kind)
		}
		byID[member.ID] = member
	}

	switch {
	case controllers == 1 && witnesses == 0 && len(members) == 1:
		return ProfileStandalone, byID, nil
	case controllers == 2 && witnesses == 1 && len(members) == 3:
		return ProfileTwoPlusWitness, byID, nil
	case controllers == 3 && witnesses == 0 && len(members) == 3:
		return ProfileThreeControllers, byID, nil
	case controllers == 5 && witnesses == 0 && len(members) == 5:
		return ProfileFiveControllers, byID, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported voter profile controllers=%d witnesses=%d total=%d", ErrInvalidMembership, controllers, witnesses, len(members))
	}
}

func healthyVotes(members []Member) int {
	count := 0
	for _, member := range members {
		if member.Healthy {
			count++
		}
	}
	return count
}

func readyVotes(members []Member) int {
	count := 0
	for _, member := range members {
		if memberReady(member) {
			count++
		}
	}
	return count
}

func memberReady(member Member) bool {
	if !member.Healthy {
		return false
	}
	if member.Kind == MemberWitness {
		return true
	}
	return member.CaughtUp
}

func quorum(voters int) int { return voters/2 + 1 }

func membershipDiff(current, desired map[string]Member) ([]string, []string) {
	add := make([]string, 0)
	remove := make([]string, 0)
	for id := range desired {
		if _, exists := current[id]; !exists {
			add = append(add, id)
		}
	}
	for id := range current {
		if _, exists := desired[id]; !exists {
			remove = append(remove, id)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

func planFingerprint(request ChangeRequest, add, remove []string) (string, error) {
	currentMembers := append([]Member(nil), request.Current.Members...)
	desiredMembers := append([]Member(nil), request.DesiredMembers...)
	sort.Slice(currentMembers, func(i, j int) bool { return currentMembers[i].ID < currentMembers[j].ID })
	sort.Slice(desiredMembers, func(i, j int) bool { return desiredMembers[i].ID < desiredMembers[j].ID })
	input := struct {
		ClusterID               string   `json:"cluster_id"`
		CurrentGeneration       uint64   `json:"current_generation"`
		DesiredGeneration       uint64   `json:"desired_generation"`
		LeaderID                string   `json:"leader_id"`
		NextLeaderID            string   `json:"next_leader_id,omitempty"`
		LeaderTransferConfirmed bool     `json:"leader_transfer_confirmed"`
		FencingConfirmed        bool     `json:"fencing_confirmed"`
		CurrentMembers          []Member `json:"current_members"`
		DesiredMembers          []Member `json:"desired_members"`
		Add                     []string `json:"add"`
		Remove                  []string `json:"remove"`
	}{
		ClusterID:               request.Current.ClusterID,
		CurrentGeneration:       request.Current.Generation,
		DesiredGeneration:       request.DesiredGeneration,
		LeaderID:                request.Current.LeaderID,
		NextLeaderID:            request.NextLeaderID,
		LeaderTransferConfirmed: request.LeaderTransferConfirmed,
		FencingConfirmed:        request.FencingConfirmed,
		CurrentMembers:          currentMembers,
		DesiredMembers:          desiredMembers,
		Add:                     append([]string(nil), add...),
		Remove:                  append([]string(nil), remove...),
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: fingerprint: %v", ErrInvalidMembership, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateIdentifier(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: %s is required and must not have surrounding whitespace", ErrInvalidMembership, field)
	}
	if len(value) > 128 {
		return fmt.Errorf("%w: %s exceeds 128 bytes", ErrInvalidMembership, field)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("%w: %s contains whitespace or control characters", ErrInvalidMembership, field)
		}
	}
	return nil
}
