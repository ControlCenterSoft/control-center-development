package audit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID            string         `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Action        string         `json:"action"`
	Outcome       string         `json:"outcome"`
	ActorID       string         `json:"actor_id,omitempty"`
	SubjectID     string         `json:"subject_id,omitempty"`
	SourceIP      string         `json:"source_ip,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
	PreviousHash  string         `json:"previous_hash,omitempty"`
	Hash          string         `json:"hash"`
}

type Logger interface {
	Append(context.Context, Event) error
}
type MemoryLog struct {
	mu      sync.RWMutex
	records []Event
}

func NewMemoryLog() *MemoryLog { return &MemoryLog{} }
func (l *MemoryLog) Append(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	previousHash := ""
	if len(l.records) > 0 {
		previousHash = l.records[len(l.records)-1].Hash
	}
	event, err := Prepare(event, previousHash)
	if err != nil {
		return err
	}
	l.records = append(l.records, event)
	return nil
}
func Prepare(event Event, previousHash string) (Event, error) {
	if strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.Outcome) == "" {
		return Event{}, fmt.Errorf("audit action and outcome are required")
	}
	if event.ID == "" {
		event.ID = randomID()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC().Truncate(time.Microsecond)
	} else {
		event.OccurredAt = event.OccurredAt.UTC().Truncate(time.Microsecond)
	}
	redacted, err := redactDetails(event.Details)
	if err != nil {
		return Event{}, fmt.Errorf("audit details are not safely serializable: %w", err)
	}
	event.Details = redacted
	event.PreviousHash = previousHash
	event.Hash, err = hashEvent(event)
	if err != nil {
		return Event{}, err
	}
	return event, nil
}
func Verify(event Event, expectedPreviousHash string) error {
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("audit timestamp is required")
	}
	canonicalTimestamp := event.OccurredAt.UTC().Truncate(time.Microsecond)
	if event.OccurredAt.Location() != time.UTC || event.OccurredAt.Nanosecond()%1_000 != 0 {
		return fmt.Errorf("audit timestamp is not canonical PostgreSQL microsecond UTC")
	}
	if !event.OccurredAt.Equal(canonicalTimestamp) {
		return fmt.Errorf("audit timestamp is not canonical")
	}
	if event.PreviousHash != expectedPreviousHash {
		return fmt.Errorf("audit previous hash mismatch")
	}
	computedHash, err := hashEvent(event)
	if err != nil {
		return err
	}
	if event.Hash == "" || computedHash != event.Hash {
		return fmt.Errorf("audit event hash mismatch")
	}
	return nil
}
func (l *MemoryLog) Records() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]Event, len(l.records))
	copy(result, l.records)
	return result
}

var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|authorization|cookie|api[-_]?key|private[-_]?key|credential)`)
var bearerValue = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)

// Redact returns a JSON-compatible copy of audit details with sensitive values
// removed. Unsupported values are replaced with a safe marker; Prepare applies
// the stricter fail-closed variant and rejects such events instead of persisting
// potentially unredacted data.
func Redact(input map[string]any) map[string]any {
	result, err := redactDetails(input)
	if err != nil {
		return map[string]any{"redaction_error": "unsupported_detail_value"}
	}
	return result
}

func redactDetails(input map[string]any) (map[string]any, error) {
	if input == nil {
		return nil, nil
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var normalized map[string]any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return redactMap(normalized), nil
}

func redactMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey.MatchString(key) {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = redactValue(value)
	}
	return result
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactValue(typed[i])
		}
		return out
	case string:
		return bearerValue.ReplaceAllString(typed, "Bearer [REDACTED]")
	default:
		return typed
	}
}
func hashEvent(event Event) (string, error) {
	event.Hash = ""
	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("audit event is not serializable: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("operating system random source unavailable")
	}
	return hex.EncodeToString(b)
}
