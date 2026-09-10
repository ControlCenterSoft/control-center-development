package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"control-center/internal/identity/audit"
)

type auditEventsResponse struct {
	Events     []audit.Event `json:"events"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	principal, _ := PrincipalFromContext(r.Context())
	if s.auditReader == nil {
		writeError(w, r, http.StatusServiceUnavailable, "audit_read_unavailable", "Audit events are temporarily unavailable")
		return
	}
	query, rawCursor, err := parseAuditQuery(r.URL.Query())
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_audit_query", "Invalid audit query")
		return
	}
	if rawCursor != "" {
		sequenceID, err := decodeAuditCursor(rawCursor, query, principal.Token)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_audit_query", "Invalid audit query")
			return
		}
		query.BeforeSequenceID = sequenceID
	}
	page, err := s.auditReader.Read(r.Context(), query)
	if err != nil {
		_ = s.audit.Append(r.Context(), audit.Event{
			Action: "audit.events_list", Outcome: "failed", ActorID: principal.Identity.ID,
			SourceIP: remoteIP(r), Details: map[string]any{"reason": "read_failed"},
		})
		writeError(w, r, http.StatusServiceUnavailable, "audit_read_unavailable", "Audit events are temporarily unavailable")
		return
	}

	events := make([]audit.Event, len(page.Entries))
	for index, entry := range page.Entries {
		events[index] = entry.Event
	}
	nextCursor := ""
	if page.HasMore && len(page.Entries) > 0 {
		nextCursor, err = encodeAuditCursor(page.Entries[len(page.Entries)-1].SequenceID, query, principal.Token)
		if err != nil {
			_ = s.audit.Append(r.Context(), audit.Event{
				Action: "audit.events_list", Outcome: "failed", ActorID: principal.Identity.ID,
				SourceIP: remoteIP(r), Details: map[string]any{"reason": "cursor_encode_failed"},
			})
			writeError(w, r, http.StatusServiceUnavailable, "audit_read_unavailable", "Audit events are temporarily unavailable")
			return
		}
	}
	if err := s.audit.Append(r.Context(), audit.Event{
		Action: "audit.events_list", Outcome: "success", ActorID: principal.Identity.ID,
		SourceIP: remoteIP(r), Details: map[string]any{
			"limit": query.Limit, "returned": len(events), "action": query.Action,
			"outcome": query.Outcome, "actor_id": query.ActorID, "subject_id": query.SubjectID,
			"event_id": query.EventID, "correlation_id": query.CorrelationID,
			"from": auditTimeBound(query.From), "to": auditTimeBound(query.To),
		},
	}); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "audit_evidence_unavailable", "Audit read evidence could not be recorded")
		return
	}
	writeJSON(w, http.StatusOK, auditEventsResponse{Events: events, NextCursor: nextCursor})
}

func parseAuditQuery(values url.Values) (audit.Query, string, error) {
	allowed := map[string]struct{}{
		"limit": {}, "cursor": {}, "action": {}, "outcome": {}, "actor_id": {}, "subject_id": {},
		"event_id": {}, "correlation_id": {}, "from": {}, "to": {},
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return audit.Query{}, "", fmt.Errorf("unsupported or repeated audit query parameter %q", key)
		}
	}

	query := audit.Query{
		Action:        values.Get("action"),
		Outcome:       values.Get("outcome"),
		ActorID:       values.Get("actor_id"),
		SubjectID:     values.Get("subject_id"),
		EventID:       values.Get("event_id"),
		CorrelationID: values.Get("correlation_id"),
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return audit.Query{}, "", fmt.Errorf("invalid audit limit: %w", err)
		}
		query.Limit = limit
	}

	rawFrom := strings.TrimSpace(values.Get("from"))
	rawTo := strings.TrimSpace(values.Get("to"))
	if rawFrom != "" || rawTo != "" {
		if rawFrom == "" || rawTo == "" {
			return audit.Query{}, "", fmt.Errorf("audit time window requires both from and to")
		}
		if len(rawFrom) > 64 || len(rawTo) > 64 {
			return audit.Query{}, "", fmt.Errorf("audit time bound is too long")
		}
		from, err := time.Parse(time.RFC3339Nano, rawFrom)
		if err != nil {
			return audit.Query{}, "", fmt.Errorf("invalid audit from timestamp: %w", err)
		}
		to, err := time.Parse(time.RFC3339Nano, rawTo)
		if err != nil {
			return audit.Query{}, "", fmt.Errorf("invalid audit to timestamp: %w", err)
		}
		query.From = from
		query.To = to
	}
	normalized, err := audit.NormalizeQuery(query)
	if err != nil {
		return audit.Query{}, "", err
	}
	rawCursor := strings.TrimSpace(values.Get("cursor"))
	if len(rawCursor) > maxAuditCursorEncodedLength {
		return audit.Query{}, "", fmt.Errorf("audit cursor is too long")
	}
	return normalized, rawCursor, nil
}

func auditTimeBound(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
