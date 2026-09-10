package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestAuditCursorV2RoundTripAndQueryBinding(t *testing.T) {
	from := time.Date(2026, 9, 1, 12, 0, 0, 123000, time.UTC)
	query := audit.Query{
		Limit: 25, Action: "auth.login", Outcome: "success", ActorID: "user-1", SubjectID: "subject-1",
		EventID: "event-1", CorrelationID: "change-42", From: from, To: from.Add(2 * time.Hour),
	}
	cursor, err := encodeAuditCursor(42, query, "session-token-a")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(payload), auditCursorVersionV2) {
		t.Fatalf("cursor payload=%q", payload)
	}
	sequenceID, err := decodeAuditCursor(cursor, query, "session-token-a")
	if err != nil || sequenceID != 42 {
		t.Fatalf("sequence=%d err=%v", sequenceID, err)
	}

	mutations := map[string]func(audit.Query) audit.Query{
		"limit":       func(q audit.Query) audit.Query { q.Limit++; return q },
		"action":      func(q audit.Query) audit.Query { q.Action = "auth.logout"; return q },
		"outcome":     func(q audit.Query) audit.Query { q.Outcome = "failed"; return q },
		"actor":       func(q audit.Query) audit.Query { q.ActorID = "user-2"; return q },
		"subject":     func(q audit.Query) audit.Query { q.SubjectID = "subject-2"; return q },
		"event":       func(q audit.Query) audit.Query { q.EventID = "event-2"; return q },
		"correlation": func(q audit.Query) audit.Query { q.CorrelationID = "change-43"; return q },
		"from":        func(q audit.Query) audit.Query { q.From = q.From.Add(time.Microsecond); return q },
		"to":          func(q audit.Query) audit.Query { q.To = q.To.Add(time.Microsecond); return q },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAuditCursor(cursor, mutate(query), "session-token-a"); err == nil {
				t.Fatal("cursor accepted a different query")
			}
		})
	}
	if _, err := decodeAuditCursor(cursor, query, "session-token-b"); err == nil {
		t.Fatal("cursor accepted a different authenticated session")
	}
}

func TestAuditCursorIgnoresPreviousCursorPositionInScope(t *testing.T) {
	query := audit.Query{Limit: 20, Action: "security.test", BeforeSequenceID: 99}
	cursor, err := encodeAuditCursor(50, query, "session-token")
	if err != nil {
		t.Fatal(err)
	}
	query.BeforeSequenceID = 7
	sequenceID, err := decodeAuditCursor(cursor, query, "session-token")
	if err != nil || sequenceID != 50 {
		t.Fatalf("sequence=%d err=%v", sequenceID, err)
	}
}

func TestLegacyAuditCursorCompatibilityIsLimitedToUnfilteredReads(t *testing.T) {
	legacy := base64.RawURLEncoding.EncodeToString([]byte(auditCursorVersionV1 + "17"))
	sequenceID, err := decodeAuditCursor(legacy, audit.Query{Limit: 25}, "session-token")
	if err != nil || sequenceID != 17 {
		t.Fatalf("legacy unfiltered sequence=%d err=%v", sequenceID, err)
	}
	if _, err := decodeAuditCursor(legacy, audit.Query{Limit: 25, Action: "auth.login"}, "session-token"); err == nil {
		t.Fatal("legacy cursor accepted a filtered query")
	}
}

func TestAuditCursorRejectsMalformedAndUnboundValues(t *testing.T) {
	query := audit.Query{Limit: 50}
	for _, cursor := range []string{
		"not-base64!",
		base64.RawURLEncoding.EncodeToString([]byte("v9:1")),
		base64.RawURLEncoding.EncodeToString([]byte("v2:0:" + strings.Repeat("0", 64))),
		base64.RawURLEncoding.EncodeToString([]byte("v2:1:abcd")),
		strings.Repeat("a", maxAuditCursorEncodedLength+1),
	} {
		if _, err := decodeAuditCursor(cursor, query, "session-token"); err == nil {
			t.Fatalf("accepted malformed cursor %q", cursor)
		}
	}
	if _, err := encodeAuditCursor(1, query, ""); err == nil {
		t.Fatal("encoded cursor without a session binding")
	}
}
