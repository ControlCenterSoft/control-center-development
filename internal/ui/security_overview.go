package ui

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

const SecurityOverviewContractVersion = "ui.security-overview/v1"

const (
	maxSecurityOverviewSessions  = 128
	maxSecurityOverviewGrants    = 64
	maxSecurityOverviewUserAgent = 512
)

var ErrInvalidSecurityOverview = errors.New("invalid security overview")

type FirstLoginState string

const (
	FirstLoginChangeRequired        FirstLoginState = "password_change_required"
	FirstLoginCompleteOrNotRequired FirstLoginState = "complete_or_not_required"
)

type SecurityOverviewInput struct {
	Identity               auth.Identity
	PasswordChangeRequired bool
	CurrentSessionID       string
	Sessions               []auth.SessionSecurityView
	SessionPolicy          auth.SessionSecurityPolicyView
	Grants                 []rbac.EffectiveGrant
	Now                    time.Time
}

type SecurityOverviewView struct {
	ContractVersion        string                         `json:"contract_version"`
	GeneratedAt            time.Time                      `json:"generated_at"`
	Identity               auth.Identity                  `json:"identity"`
	SelfOnly               bool                           `json:"self_only"`
	MutationAuthorized     bool                           `json:"mutation_authorized"`
	FirstLogin             FirstLoginState                `json:"first_login"`
	PasswordChangeRequired bool                           `json:"password_change_required"`
	SessionPolicy          auth.SessionSecurityPolicyView `json:"session_policy"`
	SessionCount           int                            `json:"session_count"`
	CurrentSessionID       string                         `json:"current_session_id"`
	Sessions               []SecuritySessionView          `json:"sessions"`
	GrantCount             int                            `json:"grant_count"`
	PermissionCount        int                            `json:"permission_count"`
	HasWildcardPermission  bool                           `json:"has_wildcard_permission"`
	Grants                 []rbac.EffectiveGrant          `json:"grants"`
	Attention              []string                       `json:"attention"`
}

type SecuritySessionView struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	IdleExpiresAt  time.Time `json:"idle_expires_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	SourceIP       string    `json:"source_ip,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	Current        bool      `json:"current"`
}

// BuildSecurityOverview projects only the authenticated subject's existing
// identity, session and RBAC evidence into a deterministic read model for the
// 0.33 Security/Identity UI. It never grants mutation or authorization.
func BuildSecurityOverview(input SecurityOverviewInput) (SecurityOverviewView, error) {
	if input.Now.IsZero() {
		return SecurityOverviewView{}, invalidSecurityOverview("current time is required")
	}
	now := input.Now.UTC()
	identity, err := validateSecurityOverviewIdentity(input.Identity, now)
	if err != nil {
		return SecurityOverviewView{}, err
	}
	currentSessionID := strings.TrimSpace(input.CurrentSessionID)
	if currentSessionID == "" || currentSessionID != input.CurrentSessionID {
		return SecurityOverviewView{}, invalidSecurityOverview("current session id must be canonical")
	}
	if err := validateSecurityOverviewPolicy(input.SessionPolicy); err != nil {
		return SecurityOverviewView{}, err
	}
	if len(input.Sessions) == 0 || len(input.Sessions) > maxSecurityOverviewSessions {
		return SecurityOverviewView{}, invalidSecurityOverview("active session evidence must contain between 1 and %d sessions", maxSecurityOverviewSessions)
	}
	if len(input.Grants) > maxSecurityOverviewGrants {
		return SecurityOverviewView{}, invalidSecurityOverview("effective grant evidence exceeds %d entries", maxSecurityOverviewGrants)
	}

	sessions, err := normalizeSecurityOverviewSessions(input.Sessions, currentSessionID, now)
	if err != nil {
		return SecurityOverviewView{}, err
	}
	grants, permissionCount, wildcard, err := normalizeSecurityOverviewGrants(input.Grants)
	if err != nil {
		return SecurityOverviewView{}, err
	}

	firstLogin := FirstLoginCompleteOrNotRequired
	attention := make([]string, 0, 2)
	if input.PasswordChangeRequired {
		firstLogin = FirstLoginChangeRequired
		attention = append(attention, "password_change_required")
	}
	if len(grants) == 0 {
		attention = append(attention, "no_effective_grants")
	}

	return SecurityOverviewView{
		ContractVersion:        SecurityOverviewContractVersion,
		GeneratedAt:            now,
		Identity:               identity,
		SelfOnly:               true,
		MutationAuthorized:     false,
		FirstLogin:             firstLogin,
		PasswordChangeRequired: input.PasswordChangeRequired,
		SessionPolicy:          input.SessionPolicy,
		SessionCount:           len(sessions),
		CurrentSessionID:       currentSessionID,
		Sessions:               sessions,
		GrantCount:             len(grants),
		PermissionCount:        permissionCount,
		HasWildcardPermission:  wildcard,
		Grants:                 grants,
		Attention:              attention,
	}, nil
}

func validateSecurityOverviewIdentity(identity auth.Identity, now time.Time) (auth.Identity, error) {
	if strings.TrimSpace(identity.ID) == "" || identity.ID != strings.TrimSpace(identity.ID) {
		return auth.Identity{}, invalidSecurityOverview("identity id must be canonical")
	}
	if strings.TrimSpace(identity.Username) == "" || identity.Username != strings.TrimSpace(identity.Username) {
		return auth.Identity{}, invalidSecurityOverview("username must be canonical")
	}
	if identity.DisplayName != "" && identity.DisplayName != strings.TrimSpace(identity.DisplayName) {
		return auth.Identity{}, invalidSecurityOverview("display name must be canonical when present")
	}
	if identity.CreatedAt.IsZero() || identity.CreatedAt.After(now) {
		return auth.Identity{}, invalidSecurityOverview("identity creation timestamp is invalid")
	}
	identity.CreatedAt = identity.CreatedAt.UTC()
	return identity, nil
}

func validateSecurityOverviewPolicy(policy auth.SessionSecurityPolicyView) error {
	if policy.AbsoluteTTLSeconds < 60 {
		return invalidSecurityOverview("absolute session ttl must be at least 60 seconds")
	}
	if policy.IdleTimeoutSeconds < 60 || policy.IdleTimeoutSeconds > policy.AbsoluteTTLSeconds {
		return invalidSecurityOverview("idle timeout must be between 60 seconds and the absolute ttl")
	}
	if policy.ActivityExtendsAbsoluteExpiry {
		return invalidSecurityOverview("session activity must not extend absolute expiry")
	}
	return nil
}

func normalizeSecurityOverviewSessions(sources []auth.SessionSecurityView, currentSessionID string, now time.Time) ([]SecuritySessionView, error) {
	out := make([]SecuritySessionView, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	currentMatches := 0
	for _, source := range sources {
		id := strings.TrimSpace(source.ID)
		if id == "" || id != source.ID {
			return nil, invalidSecurityOverview("session id must be canonical")
		}
		if _, exists := seen[id]; exists {
			return nil, invalidSecurityOverview("duplicate session %q", id)
		}
		seen[id] = struct{}{}
		if source.CreatedAt.IsZero() || source.LastActivityAt.IsZero() || source.IdleExpiresAt.IsZero() || source.ExpiresAt.IsZero() {
			return nil, invalidSecurityOverview("session %q has incomplete timestamps", id)
		}
		createdAt := source.CreatedAt.UTC()
		lastActivityAt := source.LastActivityAt.UTC()
		idleExpiresAt := source.IdleExpiresAt.UTC()
		expiresAt := source.ExpiresAt.UTC()
		if lastActivityAt.Before(createdAt) || !idleExpiresAt.After(lastActivityAt) || !expiresAt.After(createdAt) {
			return nil, invalidSecurityOverview("session %q has inconsistent timestamps", id)
		}
		if !now.Before(idleExpiresAt) || !now.Before(expiresAt) {
			return nil, invalidSecurityOverview("session %q is not active at projection time", id)
		}
		if source.Current != (id == currentSessionID) {
			return nil, invalidSecurityOverview("session %q current-session marker is inconsistent", id)
		}
		if source.Current {
			currentMatches++
		}
		sourceIP := strings.TrimSpace(source.SourceIP)
		if sourceIP != "" && net.ParseIP(sourceIP) == nil {
			return nil, invalidSecurityOverview("session %q source ip is invalid", id)
		}
		userAgent := strings.TrimSpace(source.UserAgent)
		if len(userAgent) > maxSecurityOverviewUserAgent {
			return nil, invalidSecurityOverview("session %q user agent exceeds %d bytes", id, maxSecurityOverviewUserAgent)
		}
		out = append(out, SecuritySessionView{
			ID:             id,
			CreatedAt:      createdAt,
			LastActivityAt: lastActivityAt,
			IdleExpiresAt:  idleExpiresAt,
			ExpiresAt:      expiresAt,
			SourceIP:       sourceIP,
			UserAgent:      userAgent,
			Current:        source.Current,
		})
	}
	if currentMatches != 1 {
		return nil, invalidSecurityOverview("exactly one active session must match the current session")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		if !out[i].LastActivityAt.Equal(out[j].LastActivityAt) {
			return out[i].LastActivityAt.After(out[j].LastActivityAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func normalizeSecurityOverviewGrants(sources []rbac.EffectiveGrant) ([]rbac.EffectiveGrant, int, bool, error) {
	out := make([]rbac.EffectiveGrant, 0, len(sources))
	seenGrants := make(map[string]struct{}, len(sources))
	effectivePermissions := make(map[rbac.Permission]struct{})
	hasWildcard := false
	for _, source := range sources {
		roleName := strings.TrimSpace(source.RoleName)
		if roleName == "" || roleName != source.RoleName || !source.Scope.Valid() {
			return nil, 0, false, invalidSecurityOverview("effective grant has invalid role or scope")
		}
		grantKey := roleName + "\x00" + string(source.Scope.Kind) + "\x00" + source.Scope.ID
		if _, exists := seenGrants[grantKey]; exists {
			return nil, 0, false, invalidSecurityOverview("duplicate effective grant for role %q", roleName)
		}
		seenGrants[grantKey] = struct{}{}
		if len(source.Permissions) == 0 {
			return nil, 0, false, invalidSecurityOverview("effective grant %q has no permissions", roleName)
		}
		permissions := make([]rbac.Permission, 0, len(source.Permissions))
		seenPermissions := make(map[rbac.Permission]struct{}, len(source.Permissions))
		for _, permission := range source.Permissions {
			if strings.TrimSpace(string(permission)) == "" || string(permission) != strings.TrimSpace(string(permission)) {
				return nil, 0, false, invalidSecurityOverview("effective grant %q contains an invalid permission", roleName)
			}
			if _, exists := seenPermissions[permission]; exists {
				return nil, 0, false, invalidSecurityOverview("effective grant %q contains duplicate permission %q", roleName, permission)
			}
			seenPermissions[permission] = struct{}{}
			effectivePermissions[permission] = struct{}{}
			if permission == rbac.PermissionAll {
				hasWildcard = true
			}
			permissions = append(permissions, permission)
		}
		sort.Slice(permissions, func(i, j int) bool { return permissions[i] < permissions[j] })
		out = append(out, rbac.EffectiveGrant{RoleName: roleName, Scope: source.Scope, Permissions: permissions})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope.Kind != out[j].Scope.Kind {
			return out[i].Scope.Kind < out[j].Scope.Kind
		}
		if out[i].Scope.ID != out[j].Scope.ID {
			return out[i].Scope.ID < out[j].Scope.ID
		}
		return out[i].RoleName < out[j].RoleName
	})
	return out, len(effectivePermissions), hasWildcard, nil
}

func invalidSecurityOverview(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSecurityOverview, fmt.Sprintf(format, values...))
}
