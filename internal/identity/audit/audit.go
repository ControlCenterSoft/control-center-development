package audit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxAuditDetailsBytes          = 16 * 1024
	maxAuditDetailDepth           = 8
	maxAuditDetailNodes           = 512
	maxAuditDetailCollectionItems = 128
	maxAuditDetailStringBytes     = 4096

	maxAuditEventIDBytes       = 64
	maxAuditActionBytes        = 192
	maxAuditOutcomeBytes       = 32
	maxAuditActorIDBytes       = 64
	maxAuditSubjectIDBytes     = 512
	maxAuditSourceIPBytes      = 64
	maxAuditCorrelationIDBytes = 512
	maxAuditHashBytes          = 64
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
	event = normalizeEventFields(event)
	if event.ID == "" {
		event.ID = randomID()
	}
	// Chain fields are derived by Prepare. Ignore any caller-supplied values so
	// existing callers that reuse Event values cannot influence chain linkage.
	event.PreviousHash = ""
	event.Hash = ""
	if err := validateEventFields(event); err != nil {
		return Event{}, err
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
	if len(event.PreviousHash) > maxAuditHashBytes {
		return Event{}, fmt.Errorf("audit previous_hash exceeds %d-byte limit", maxAuditHashBytes)
	}
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

func normalizeEventFields(event Event) Event {
	event.ID = strings.TrimSpace(event.ID)
	event.Action = strings.TrimSpace(event.Action)
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.ActorID = strings.TrimSpace(event.ActorID)
	event.SubjectID = strings.TrimSpace(event.SubjectID)
	event.SourceIP = strings.TrimSpace(event.SourceIP)
	event.CorrelationID = strings.TrimSpace(event.CorrelationID)
	return event
}

func validateEventFields(event Event) error {
	fields := []struct {
		name     string
		value    string
		maxBytes int
		required bool
	}{
		{name: "id", value: event.ID, maxBytes: maxAuditEventIDBytes, required: true},
		{name: "action", value: event.Action, maxBytes: maxAuditActionBytes, required: true},
		{name: "outcome", value: event.Outcome, maxBytes: maxAuditOutcomeBytes, required: true},
		{name: "actor_id", value: event.ActorID, maxBytes: maxAuditActorIDBytes},
		{name: "subject_id", value: event.SubjectID, maxBytes: maxAuditSubjectIDBytes},
		{name: "source_ip", value: event.SourceIP, maxBytes: maxAuditSourceIPBytes},
		{name: "correlation_id", value: event.CorrelationID, maxBytes: maxAuditCorrelationIDBytes},
		{name: "previous_hash", value: event.PreviousHash, maxBytes: maxAuditHashBytes},
		{name: "hash", value: event.Hash, maxBytes: maxAuditHashBytes},
	}
	for _, field := range fields {
		if field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("audit %s is not canonical", field.name)
		}
		if field.required && field.value == "" {
			return fmt.Errorf("audit %s is required", field.name)
		}
		if len(field.value) > field.maxBytes {
			return fmt.Errorf("audit %s exceeds %d-byte limit", field.name, field.maxBytes)
		}
	}
	return nil
}

var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|authorization|cookie|api[-_]?key|private[-_]?key|credential)`)
var bearerValue = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)

// Redact returns a JSON-compatible copy of audit details with sensitive values
// removed. Unsupported or over-budget values are replaced with a safe marker;
// Prepare applies the stricter fail-closed variant and rejects such events
// instead of persisting potentially unsafe or unbounded data.
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
	if err := validateDetailShape(reflect.ValueOf(input), 1, &detailBudget{}, make(map[visit]bool)); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxAuditDetailsBytes {
		return nil, fmt.Errorf("audit details exceed %d-byte limit", maxAuditDetailsBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var normalized map[string]any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	if err := validateNormalizedDetailValue(normalized, 1, &detailBudget{}); err != nil {
		return nil, err
	}
	return redactMap(normalized), nil
}

type detailBudget struct {
	nodes int
}

type visit struct {
	kind reflect.Kind
	ptr  uintptr
}

func (b *detailBudget) consume() error {
	b.nodes++
	if b.nodes > maxAuditDetailNodes {
		return fmt.Errorf("audit details exceed %d-node limit", maxAuditDetailNodes)
	}
	return nil
}

func validateDetailShape(value reflect.Value, depth int, budget *detailBudget, stack map[visit]bool) error {
	if !value.IsValid() {
		return budget.consume()
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return budget.consume()
		}
		value = value.Elem()
	}
	if depth > maxAuditDetailDepth {
		return fmt.Errorf("audit details exceed depth limit %d", maxAuditDetailDepth)
	}
	if err := budget.consume(); err != nil {
		return err
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		key := visit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[key] {
			return fmt.Errorf("audit details contain a cycle")
		}
		stack[key] = true
		defer delete(stack, key)
		return validateDetailShape(value.Elem(), depth+1, budget, stack)
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if value.Len() > maxAuditDetailCollectionItems {
			return fmt.Errorf("audit detail map exceeds %d-item limit", maxAuditDetailCollectionItems)
		}
		key := visit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[key] {
			return fmt.Errorf("audit details contain a cycle")
		}
		stack[key] = true
		defer delete(stack, key)
		iter := value.MapRange()
		for iter.Next() {
			mapKey := iter.Key()
			if mapKey.Kind() == reflect.String && len(mapKey.String()) > maxAuditDetailStringBytes {
				return fmt.Errorf("audit detail map key exceeds %d-byte string limit", maxAuditDetailStringBytes)
			}
			if err := validateDetailShape(iter.Value(), depth+1, budget, stack); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		if value.Len() > maxAuditDetailCollectionItems {
			return fmt.Errorf("audit detail array exceeds %d-item limit", maxAuditDetailCollectionItems)
		}
		key := visit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[key] {
			return fmt.Errorf("audit details contain a cycle")
		}
		stack[key] = true
		defer delete(stack, key)
		for i := 0; i < value.Len(); i++ {
			if err := validateDetailShape(value.Index(i), depth+1, budget, stack); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array:
		if value.Len() > maxAuditDetailCollectionItems {
			return fmt.Errorf("audit detail array exceeds %d-item limit", maxAuditDetailCollectionItems)
		}
		for i := 0; i < value.Len(); i++ {
			if err := validateDetailShape(value.Index(i), depth+1, budget, stack); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		typeOfValue := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := typeOfValue.Field(i)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			if err := validateDetailShape(value.Field(i), depth+1, budget, stack); err != nil {
				return err
			}
		}
		return nil
	case reflect.String:
		if value.Len() > maxAuditDetailStringBytes {
			return fmt.Errorf("audit detail string exceeds %d-byte limit", maxAuditDetailStringBytes)
		}
		return nil
	default:
		return nil
	}
}

func validateNormalizedDetailValue(value any, depth int, budget *detailBudget) error {
	if depth > maxAuditDetailDepth {
		return fmt.Errorf("audit details exceed depth limit %d", maxAuditDetailDepth)
	}
	if err := budget.consume(); err != nil {
		return err
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) > maxAuditDetailCollectionItems {
			return fmt.Errorf("audit detail map exceeds %d-item limit", maxAuditDetailCollectionItems)
		}
		for key, child := range typed {
			if len(key) > maxAuditDetailStringBytes {
				return fmt.Errorf("audit detail map key exceeds %d-byte string limit", maxAuditDetailStringBytes)
			}
			if err := validateNormalizedDetailValue(child, depth+1, budget); err != nil {
				return err
			}
		}
	case []any:
		if len(typed) > maxAuditDetailCollectionItems {
			return fmt.Errorf("audit detail array exceeds %d-item limit", maxAuditDetailCollectionItems)
		}
		for _, child := range typed {
			if err := validateNormalizedDetailValue(child, depth+1, budget); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > maxAuditDetailStringBytes {
			return fmt.Errorf("audit detail string exceeds %d-byte limit", maxAuditDetailStringBytes)
		}
	}
	return nil
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
	if err := validateEventFields(event); err != nil {
		return "", err
	}
	if event.Details != nil {
		if err := validateDetailShape(reflect.ValueOf(event.Details), 1, &detailBudget{}, make(map[visit]bool)); err != nil {
			return "", err
		}
		payload, err := json.Marshal(event.Details)
		if err != nil {
			return "", fmt.Errorf("audit event details are not serializable: %w", err)
		}
		if len(payload) > maxAuditDetailsBytes {
			return "", fmt.Errorf("audit event details exceed %d-byte limit", maxAuditDetailsBytes)
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.UseNumber()
		var normalized any
		if err := decoder.Decode(&normalized); err != nil {
			return "", fmt.Errorf("audit event details are not serializable: %w", err)
		}
		if err := validateNormalizedDetailValue(normalized, 1, &detailBudget{}); err != nil {
			return "", err
		}
	}
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
