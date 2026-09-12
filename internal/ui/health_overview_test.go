package ui

import (
	"errors"
	"testing"
	"time"

	corehealth "control-center/internal/health"
)

func TestBuildHealthOverviewUnavailableWithoutLoadedEvidence(t *testing.T) {
	view, err := BuildHealthOverview(HealthOverviewInput{})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if view.DataState != HealthOverviewUnavailable || view.OverallState != corehealth.StateUnknown {
		t.Fatalf("unexpected unavailable view: %+v", view)
	}
	if view.MutationAuthorized {
		t.Fatal("read model must never authorize mutation")
	}
}

func TestBuildHealthOverviewFreshHealthy(t *testing.T) {
	now := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       true,
		Now:          now,
		StaleAfter:   5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{{
			ID:           "signal-api-ready",
			ResourceKind: "service",
			ResourceID:   "api",
			CheckName:    "readiness",
			State:        corehealth.StateHealthy,
			ObservedAt:   now.Add(-time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if view.OverallState != corehealth.StateHealthy || view.SignalCount != 1 {
		t.Fatalf("unexpected healthy view: %+v", view)
	}
	if got := view.Signals[0]; got.Freshness != HealthFreshnessCurrent || got.EffectiveState != corehealth.StateHealthy {
		t.Fatalf("unexpected projected signal: %+v", got)
	}
}

func TestBuildHealthOverviewDoesNotShowStaleHealthyAsHealthy(t *testing.T) {
	now := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       true,
		Now:          now,
		StaleAfter:   5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{{
			ID:           "signal-db-ready",
			ResourceKind: "service",
			ResourceID:   "postgres",
			CheckName:    "readiness",
			State:        corehealth.StateHealthy,
			ObservedAt:   now.Add(-10 * time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if view.OverallState != corehealth.StateDegraded {
		t.Fatalf("stale healthy evidence must degrade overview, got %q", view.OverallState)
	}
	if got := view.Signals[0]; got.SourceState != corehealth.StateHealthy || got.EffectiveState != corehealth.StateDegraded || got.Freshness != HealthFreshnessStale {
		t.Fatalf("unexpected stale signal: %+v", got)
	}
}

func TestBuildHealthOverviewDoesNotShowExpiredHealthyAsHealthy(t *testing.T) {
	now := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       true,
		Now:          now,
		StaleAfter:   5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{{
			ID:           "signal-agent-heartbeat",
			ResourceKind: "node",
			ResourceID:   "node-01",
			CheckName:    "heartbeat",
			State:        corehealth.StateHealthy,
			ObservedAt:   now.Add(-20 * time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if view.OverallState != corehealth.StateUnknown {
		t.Fatalf("expired healthy evidence must become unknown, got %q", view.OverallState)
	}
	if got := view.Signals[0]; got.EffectiveState != corehealth.StateUnknown || got.Freshness != HealthFreshnessExpired {
		t.Fatalf("unexpected expired signal: %+v", got)
	}
}

func TestBuildHealthOverviewAggregatesWorstStateAndSafeEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       true,
		Now:          now,
		StaleAfter:   5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{
			{
				ID:           "signal-node-cpu",
				ResourceKind: "node",
				ResourceID:   "node-01",
				CheckName:    "cpu",
				State:        corehealth.StateDegraded,
				ObservedAt:   now.Add(-2 * time.Minute),
				RunbookRef:   "runbook.node.capacity",
				EvidenceRefs: []string{"evidence:z", "evidence:a", "evidence:a"},
			},
			{
				ID:           "signal-node-disk",
				ResourceKind: "node",
				ResourceID:   "node-01",
				CheckName:    "disk",
				State:        corehealth.StateUnhealthy,
				ObservedAt:   now.Add(-time.Minute),
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if view.OverallState != corehealth.StateUnhealthy || len(view.AffectedResources) != 1 {
		t.Fatalf("unexpected aggregate: %+v", view)
	}
	if resource := view.AffectedResources[0]; resource.State != corehealth.StateUnhealthy || resource.SignalCount != 2 {
		t.Fatalf("unexpected resource aggregate: %+v", resource)
	}
	var degraded HealthSignalView
	for _, signal := range view.Signals {
		if signal.ID == "signal-node-cpu" {
			degraded = signal
		}
	}
	if degraded.RunbookRef != "runbook.node.capacity" || len(degraded.EvidenceRefs) != 2 || degraded.EvidenceRefs[0] != "evidence:a" || degraded.EvidenceRefs[1] != "evidence:z" {
		t.Fatalf("unexpected safe evidence projection: %+v", degraded)
	}
}

func TestBuildHealthOverviewRejectsAmbiguousEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	base := HealthSignal{
		ID:           "signal-1",
		ResourceKind: "service",
		ResourceID:   "api",
		CheckName:    "ready",
		State:        corehealth.StateHealthy,
		ObservedAt:   now,
	}

	tests := []struct {
		name    string
		signals []HealthSignal
	}{
		{name: "duplicate id", signals: []HealthSignal{base, base}},
		{name: "future observation", signals: []HealthSignal{{ID: "signal-2", ResourceKind: "service", ResourceID: "api", CheckName: "ready", State: corehealth.StateHealthy, ObservedAt: now.Add(time.Second)}}},
		{name: "non canonical id", signals: []HealthSignal{{ID: " signal-3", ResourceKind: "service", ResourceID: "api", CheckName: "ready", State: corehealth.StateHealthy, ObservedAt: now}}},
		{name: "invalid state", signals: []HealthSignal{{ID: "signal-4", ResourceKind: "service", ResourceID: "api", CheckName: "ready", State: corehealth.State("ok"), ObservedAt: now}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildHealthOverview(HealthOverviewInput{
				Loaded:       true,
				Now:          now,
				StaleAfter:   5 * time.Minute,
				ExpiredAfter: 15 * time.Minute,
				Signals:      tt.signals,
			})
			if !errors.Is(err, ErrInvalidHealthOverview) {
				t.Fatalf("expected ErrInvalidHealthOverview, got %v", err)
			}
		})
	}
}
