package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/audit"
	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
	"control-center/internal/identity/security"
)

type failLogoutAudit struct {
	base *audit.MemoryLog
}

func (l *failLogoutAudit) Append(ctx context.Context, event audit.Event) error {
	if event.Action == "auth.logout" {
		return errors.New("synthetic logout audit failure")
	}
	return l.base.Append(ctx, event)
}

func TestLogoutSurfacesAuditFailureAfterRevokingSession(t *testing.T) {
	store := auth.NewMemoryStore()
	log := &failLogoutAudit{base: audit.NewMemoryLog()}
	hasher := security.NewPasswordHasher()
	hash, err := hasher.Hash("a secure test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(context.Background(), auth.User{
		ID: "logout-audit-user", Username: "viewer", DisplayName: "Viewer",
		PasswordHash: hash, Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(store, store, log, hasher, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(service, rbac.NewAuthorizer(), log, Config{})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		path       string
		wantJSON   bool
		wantStatus int
	}{
		{name: "api", path: "/api/v1/auth/logout", wantJSON: true, wantStatus: http.StatusServiceUnavailable},
		{name: "browser", path: "/web/logout", wantJSON: false, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			issued, err := service.Login(context.Background(), auth.LoginInput{
				Username: "viewer", Password: "a secure test password", SourceIP: "192.0.2.10",
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.AddCookie(&http.Cookie{Name: DefaultSessionCookie, Value: issued.Token})
			result := httptest.NewRecorder()
			server.ServeHTTP(result, request)

			if result.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
			}
			if test.wantJSON {
				if !strings.Contains(result.Body.String(), `"code":"logout_unavailable"`) {
					t.Fatalf("API error body=%s", result.Body.String())
				}
			} else if strings.Contains(result.Body.String(), `"error"`) || strings.Contains(result.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("browser logout exposed JSON API envelope: content-type=%q body=%s", result.Header().Get("Content-Type"), result.Body.String())
			}

			cleared := false
			for _, cookie := range result.Result().Cookies() {
				if cookie.Name == DefaultSessionCookie && cookie.Value == "" && cookie.MaxAge < 0 {
					cleared = true
					break
				}
			}
			if !cleared {
				t.Fatal("logout audit failure did not clear the session cookie")
			}
			if _, err := service.Authenticate(context.Background(), issued.Token); !errors.Is(err, auth.ErrUnauthenticated) {
				t.Fatalf("revoked session authenticate err=%v, want ErrUnauthenticated", err)
			}
		})
	}
}
