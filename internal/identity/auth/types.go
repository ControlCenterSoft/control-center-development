package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCredentials     = errors.New("invalid username or password")
	ErrUnauthenticated        = errors.New("authentication required")
	ErrNotFound               = errors.New("not found")
	ErrConflict               = errors.New("already exists")
	ErrAuditUnavailable       = errors.New("security audit unavailable")
	ErrInvalidCurrentPassword = errors.New("invalid current password")
	ErrPasswordPolicy         = errors.New("password does not satisfy policy")
)

type User struct {
	ID                     string
	Username               string
	DisplayName            string
	PasswordHash           string
	Enabled                bool
	PasswordChangeRequired bool
	CreatedAt              time.Time
	PasswordChangedAt      time.Time
	LastLoginAt            *time.Time
}
type Identity struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func (u User) Identity() Identity {
	return Identity{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt}
}

type Session struct {
	ID                string
	UserID            string
	TokenDigest       string
	CredentialVersion time.Time
	CreatedAt         time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	SourceIP          string
	UserAgent         string
}
type SessionView struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s Session) View() SessionView {
	return SessionView{ID: s.ID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
}

type SessionSecurityView struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	SourceIP  string    `json:"source_ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Current   bool      `json:"current"`
}

func (s Session) SecurityView(currentSessionID string) SessionSecurityView {
	return SessionSecurityView{
		ID: s.ID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt,
		SourceIP: s.SourceIP, UserAgent: s.UserAgent, Current: s.ID == currentSessionID,
	}
}

type UserStore interface {
	FindUserByUsername(context.Context, string) (User, error)
	FindUserByID(context.Context, string) (User, error)
	SetLastLogin(context.Context, string, time.Time) error
	ChangePasswordAndRevokeSessions(context.Context, string, string, string, time.Time) error
}
type SessionStore interface {
	CreateSession(context.Context, Session, string) error
	FindSessionByDigest(context.Context, string) (Session, error)
	FindSessionForUserByID(context.Context, string, string) (Session, error)
	ListActiveSessionsForUser(context.Context, string, time.Time) ([]Session, error)
	RevokeSessionByDigest(context.Context, string, time.Time) error
	RevokeSessionForUserByID(context.Context, string, string, time.Time) error
	RevokeSessionsForUser(context.Context, string, time.Time) (int, error)
}
type LoginInput struct {
	Username  string
	Password  string
	SourceIP  string
	UserAgent string
}
type IssuedSession struct {
	Token                  string
	Session                SessionView
	Identity               Identity
	PasswordChangeRequired bool
}
type AuthenticatedSession struct {
	Session                SessionView
	Identity               Identity
	PasswordChangeRequired bool
}

type ChangePasswordInput struct {
	UserID          string
	CurrentPassword string
	NewPassword     string
	SourceIP        string
}

type ListSessionsInput struct {
	UserID           string
	CurrentSessionID string
	SourceIP         string
}

type RevokeSessionInput struct {
	UserID           string
	SessionID        string
	CurrentSessionID string
	SourceIP         string
}

type RevokeSessionResult struct {
	SessionID             string
	CurrentSessionRevoked bool
}

type RevokeAllSessionsInput struct {
	UserID   string
	SourceIP string
}
