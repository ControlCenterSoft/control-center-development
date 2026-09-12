package audit

import (
	"context"
	"errors"
	"fmt"
)

const (
	DefaultExportLimit = 250
	MaxExportLimit     = 1000
)

type ExportRequest struct {
	Query Query
	Limit int
}

type ExportResult struct {
	Events               []Event
	Truncated            bool
	NextBeforeSequenceID int64
}

func ReadExport(ctx context.Context, reader Reader, request ExportRequest) (ExportResult, error) {
	if reader == nil {
		return ExportResult{}, errors.New("audit export reader is required")
	}
	limit := request.Limit
	if limit == 0 {
		limit = DefaultExportLimit
	}
	if limit < 1 || limit > MaxExportLimit {
		return ExportResult{}, fmt.Errorf("audit export limit must be between 1 and %d", MaxExportLimit)
	}

	query := request.Query
	if query.Limit != 0 {
		return ExportResult{}, errors.New("audit export query limit must be zero; use export limit")
	}
	query.Limit = min(MaxReadLimit, limit)
	normalized, err := NormalizeQuery(query)
	if err != nil {
		return ExportResult{}, err
	}

	result := ExportResult{Events: make([]Event, 0, limit)}
	before := normalized.BeforeSequenceID
	for len(result.Events) < limit {
		if err := ctx.Err(); err != nil {
			return ExportResult{}, err
		}
		remaining := limit - len(result.Events)
		pageQuery := normalized
		pageQuery.BeforeSequenceID = before
		pageQuery.Limit = min(MaxReadLimit, remaining)

		page, err := reader.Read(ctx, pageQuery)
		if err != nil {
			return ExportResult{}, fmt.Errorf("read audit export page: %w", err)
		}
		if len(page.Entries) == 0 {
			if page.HasMore {
				return ExportResult{}, errors.New("audit export reader returned has_more without entries")
			}
			break
		}

		for _, entry := range page.Entries {
			if entry.SequenceID <= 0 || (before > 0 && entry.SequenceID >= before) {
				return ExportResult{}, errors.New("audit export reader returned a non-decreasing sequence")
			}
			event := entry.Event
			event.Details = Redact(event.Details)
			result.Events = append(result.Events, event)
			before = entry.SequenceID
			if len(result.Events) == limit {
				break
			}
		}

		if !page.HasMore {
			break
		}
		if len(result.Events) == limit {
			result.Truncated = true
			result.NextBeforeSequenceID = before
			break
		}
	}
	return result, nil
}
