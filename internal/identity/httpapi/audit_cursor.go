package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"control-center/internal/identity/audit"
)

const (
	auditCursorVersionV1        = "v1:"
	auditCursorVersionV2        = "v2:"
	maxAuditCursorEncodedLength = 128
)

type auditCursorScope struct {
	Limit         int    `json:"limit"`
	Action        string `json:"action"`
	Outcome       string `json:"outcome"`
	ActorID       string `json:"actor_id"`
	SubjectID     string `json:"subject_id"`
	EventID       string `json:"event_id"`
	CorrelationID string `json:"correlation_id"`
	From          string `json:"from"`
	To            string `json:"to"`
}

func encodeAuditCursor(sequenceID int64, query audit.Query, sessionToken string) (string, error) {
	if sequenceID <= 0 {
		return "", fmt.Errorf("invalid audit cursor sequence")
	}
	if sessionToken == "" {
		return "", fmt.Errorf("audit cursor session binding is unavailable")
	}
	mac, err := auditCursorMAC(sequenceID, query, sessionToken)
	if err != nil {
		return "", err
	}
	payload := auditCursorVersionV2 + strconv.FormatInt(sequenceID, 10) + ":" + hex.EncodeToString(mac)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)), nil
}

func decodeAuditCursor(cursor string, query audit.Query, sessionToken string) (int64, error) {
	if cursor == "" || len(cursor) > maxAuditCursorEncodedLength {
		return 0, fmt.Errorf("invalid audit cursor length")
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("decode audit cursor: %w", err)
	}
	text := string(payload)
	if strings.HasPrefix(text, auditCursorVersionV2) {
		if sessionToken == "" {
			return 0, fmt.Errorf("audit cursor session binding is unavailable")
		}
		rest := strings.TrimPrefix(text, auditCursorVersionV2)
		sequenceText, macText, ok := strings.Cut(rest, ":")
		if !ok || strings.Contains(macText, ":") {
			return 0, fmt.Errorf("invalid audit cursor payload")
		}
		sequenceID, err := parseAuditCursorSequence(sequenceText)
		if err != nil {
			return 0, err
		}
		actualMAC, err := hex.DecodeString(macText)
		if err != nil || len(actualMAC) != sha256.Size {
			return 0, fmt.Errorf("invalid audit cursor authentication code")
		}
		expectedMAC, err := auditCursorMAC(sequenceID, query, sessionToken)
		if err != nil {
			return 0, err
		}
		if !hmac.Equal(actualMAC, expectedMAC) {
			return 0, fmt.Errorf("audit cursor does not match this session and query")
		}
		return sequenceID, nil
	}
	if strings.HasPrefix(text, auditCursorVersionV1) {
		if !legacyAuditCursorAllowed(query) {
			return 0, fmt.Errorf("legacy audit cursor cannot be used with filtered queries")
		}
		return parseAuditCursorSequence(strings.TrimPrefix(text, auditCursorVersionV1))
	}
	return 0, fmt.Errorf("unsupported audit cursor version")
}

func parseAuditCursorSequence(value string) (int64, error) {
	sequenceID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequenceID <= 0 {
		return 0, fmt.Errorf("invalid audit cursor sequence")
	}
	return sequenceID, nil
}

func auditCursorMAC(sequenceID int64, query audit.Query, sessionToken string) ([]byte, error) {
	scope, err := json.Marshal(auditCursorScope{
		Limit:         query.Limit,
		Action:        query.Action,
		Outcome:       query.Outcome,
		ActorID:       query.ActorID,
		SubjectID:     query.SubjectID,
		EventID:       query.EventID,
		CorrelationID: query.CorrelationID,
		From:          auditCursorTimeBound(query.From),
		To:            auditCursorTimeBound(query.To),
	})
	if err != nil {
		return nil, fmt.Errorf("encode audit cursor scope: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionToken))
	_, _ = mac.Write([]byte(auditCursorVersionV2))
	_, _ = mac.Write([]byte(strconv.FormatInt(sequenceID, 10)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(scope)
	return mac.Sum(nil), nil
}

func legacyAuditCursorAllowed(query audit.Query) bool {
	return query.Action == "" && query.Outcome == "" && query.ActorID == "" && query.SubjectID == "" &&
		query.EventID == "" && query.CorrelationID == "" && query.From.IsZero() && query.To.IsZero()
}

func auditCursorTimeBound(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
