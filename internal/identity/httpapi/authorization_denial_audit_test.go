package httpapi

import (
	"net/http"
	"testing"
)

func TestAuthorizationDenialsEmitAuditEvidence(t *testing.T) {
	fixture := newAuditEventsFixture(t)

	viewer := fixture.login(t, "viewer-audit", "a secure test password")
	beforeViewerDenial := len(fixture.log.Records())
	viewerResult := fixture.get(t, viewer, "/api/v1/audit/events")
	if viewerResult.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", viewerResult.Code, viewerResult.Body.String())
	}

	viewerEvidence := false
	for _, event := range fixture.log.Records()[beforeViewerDenial:] {
		if event.Action != "authorization.check" || event.Outcome != "denied" || event.ActorID != "viewer-1" {
			continue
		}
		if _, ok := event.Details["permission"]; !ok {
			t.Fatalf("viewer denial missing permission metadata: %#v", event.Details)
		}
		if _, ok := event.Details["scope"]; !ok {
			t.Fatalf("viewer denial missing scope metadata: %#v", event.Details)
		}
		viewerEvidence = true
		break
	}
	if !viewerEvidence {
		t.Fatal("viewer permission denial did not create authorization audit evidence")
	}

	bootstrapAdmin := fixture.login(t, "admin-audit", "admin")
	beforePasswordDenial := len(fixture.log.Records())
	adminResult := fixture.get(t, bootstrapAdmin, "/api/v1/audit/events")
	if adminResult.Code != http.StatusForbidden {
		t.Fatalf("bootstrap admin status=%d body=%s", adminResult.Code, adminResult.Body.String())
	}

	passwordEvidence := false
	for _, event := range fixture.log.Records()[beforePasswordDenial:] {
		if event.Action != "authorization.check" || event.Outcome != "denied" || event.ActorID != "admin-1" {
			continue
		}
		if event.Details["reason"] != "password_change_required" {
			continue
		}
		passwordEvidence = true
		break
	}
	if !passwordEvidence {
		t.Fatal("first-login password-change denial did not create authorization audit evidence")
	}
}
