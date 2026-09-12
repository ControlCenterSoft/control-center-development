package incidents

import (
	"context"
	"fmt"
	"sort"
)

// CorrelationService resolves a bounded candidate set through the repository
// and delegates the actual exact-match decision to EvaluateCorrelation. It is
// intentionally read-only and must be wired to an internal scope-bound Reader;
// it is not an end-user authorization boundary.
type CorrelationService struct {
	reader Reader
}

func NewCorrelationService(reader Reader) *CorrelationService {
	return &CorrelationService{reader: reader}
}

// Evaluate reads only active incidents for the observation scope and one exact
// affected-resource anchor. If that bounded query is truncated, correlation
// fails closed rather than guessing from an incomplete candidate set.
func (s *CorrelationService) Evaluate(ctx context.Context, observation CorrelationObservation) (CorrelationDecision, error) {
	if err := validateCorrelationObservation(observation); err != nil {
		return CorrelationDecision{}, err
	}
	if s == nil || s.reader == nil {
		return CorrelationDecision{}, fmt.Errorf("%w: correlation reader is unavailable", ErrInvalidCorrelationDependency)
	}

	anchor := correlationResourceAnchor(observation.AffectedResources)
	query := ListQuery{
		Limit:        MaxListLimit,
		Statuses:     []Status{StatusOpen, StatusAcknowledged},
		ScopeID:      observation.ScopeID,
		ResourceKind: anchor.Kind,
		ResourceID:   anchor.ID,
	}
	page, err := s.reader.List(ctx, query)
	if err != nil {
		return CorrelationDecision{}, fmt.Errorf("%w: candidate lookup failed: %v", ErrInvalidCorrelationDependency, err)
	}
	if page.HasMore || page.Next != nil {
		return CorrelationDecision{}, fmt.Errorf("%w: candidate lookup was truncated", ErrInvalidCorrelationDependency)
	}
	if len(page.Items) > MaxListLimit {
		return CorrelationDecision{}, fmt.Errorf("%w: candidate lookup exceeded requested bound", ErrInvalidCorrelationDependency)
	}
	return EvaluateCorrelation(observation, page.Items)
}

func correlationResourceAnchor(resources []ResourceRef) ResourceRef {
	ordered := append([]ResourceRef(nil), resources...)
	sort.Slice(ordered, func(i, j int) bool {
		left := ordered[i].Kind + "\x00" + ordered[i].ID + "\x00" + ordered[i].ScopeID
		right := ordered[j].Kind + "\x00" + ordered[j].ID + "\x00" + ordered[j].ScopeID
		return left < right
	})
	return ordered[0]
}
