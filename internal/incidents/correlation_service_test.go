package incidents

import (
	"context"
	"errors"
	"testing"
	"time"
)

type correlationReaderStub struct {
	page  ListPage
	err   error
	query ListQuery
	calls int
}

func (s *correlationReaderStub) Get(context.Context, string) (Incident, error) {
	return Incident{}, ErrNotFound
}

func (s *correlationReaderStub) List(_ context.Context, query ListQuery) (ListPage, error) {
	s.calls++
	s.query = query
	return s.page, s.err
}

func TestCorrelationServiceUsesBoundedActiveScopeQuery(t *testing.T) {
	candidate := validIncident(StatusOpen)
	candidate.AffectedResources = []ResourceRef{
		{Kind: "service", ID: "dns", ScopeID: "site:primary"},
		{Kind: "node", ID: "node-1", ScopeID: "site:primary"},
	}
	observation := correlationObservation(candidate)
	observation.Signal.ID = "signal-new"
	observation.Signal.ObservedAt = candidate.LastObservedAt.Add(time.Minute)
	reader := &correlationReaderStub{page: ListPage{Items: []Incident{candidate}}}

	decision, err := NewCorrelationService(reader).Evaluate(context.Background(), observation)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if decision.State != CorrelationMatch {
		t.Fatalf("state = %q, want %q", decision.State, CorrelationMatch)
	}
	if reader.calls != 1 {
		t.Fatalf("List() calls = %d, want 1", reader.calls)
	}
	if reader.query.Limit != MaxListLimit || reader.query.ScopeID != observation.ScopeID {
		t.Fatalf("unexpected bounded query: %#v", reader.query)
	}
	if len(reader.query.Statuses) != 2 || reader.query.Statuses[0] != StatusOpen || reader.query.Statuses[1] != StatusAcknowledged {
		t.Fatalf("unexpected active status query: %#v", reader.query.Statuses)
	}
	// node/node-1 sorts before service/dns and becomes the deterministic anchor.
	if reader.query.ResourceKind != "node" || reader.query.ResourceID != "node-1" {
		t.Fatalf("unexpected resource anchor: %#v", reader.query)
	}
}

func TestCorrelationServiceFailsClosedOnTruncatedCandidates(t *testing.T) {
	candidate := validIncident(StatusOpen)
	observation := correlationObservation(candidate)
	reader := &correlationReaderStub{
		page: ListPage{
			Items:   []Incident{candidate},
			HasMore: true,
			Next:    &ListCursor{StartedAt: candidate.StartedAt, ObjectID: candidate.ObjectID},
		},
	}

	_, err := NewCorrelationService(reader).Evaluate(context.Background(), observation)
	if !errors.Is(err, ErrInvalidCorrelationDependency) {
		t.Fatalf("Evaluate() error = %v, want ErrInvalidCorrelationDependency", err)
	}
}

func TestCorrelationServiceFailsClosedOnRepositoryError(t *testing.T) {
	observation := correlationObservation(validIncident(StatusOpen))
	reader := &correlationReaderStub{err: errors.New("storage unavailable")}

	_, err := NewCorrelationService(reader).Evaluate(context.Background(), observation)
	if !errors.Is(err, ErrInvalidCorrelationDependency) {
		t.Fatalf("Evaluate() error = %v, want ErrInvalidCorrelationDependency", err)
	}
}

func TestCorrelationServiceRejectsInvalidObservationBeforeStorage(t *testing.T) {
	observation := correlationObservation(validIncident(StatusOpen))
	observation.Signal.ObservedAt = time.Time{}
	reader := &correlationReaderStub{}

	_, err := NewCorrelationService(reader).Evaluate(context.Background(), observation)
	if !errors.Is(err, ErrInvalidCorrelationObservation) {
		t.Fatalf("Evaluate() error = %v, want ErrInvalidCorrelationObservation", err)
	}
	if reader.calls != 0 {
		t.Fatalf("List() calls = %d, want 0", reader.calls)
	}
}

func TestCorrelationServiceRequiresReader(t *testing.T) {
	observation := correlationObservation(validIncident(StatusOpen))
	_, err := NewCorrelationService(nil).Evaluate(context.Background(), observation)
	if !errors.Is(err, ErrInvalidCorrelationDependency) {
		t.Fatalf("Evaluate() error = %v, want ErrInvalidCorrelationDependency", err)
	}
}
