package operationsview

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildMaintenanceWindowEvidenceDerivesState(t *testing.T) {
	start := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	cases := []struct {
		name string
		now  time.Time
		want MaintenanceWindowState
	}{
		{name: "scheduled", now: start.Add(-time.Minute), want: MaintenanceWindowScheduled},
		{name: "active at boundary", now: start, want: MaintenanceWindowActive},
		{name: "active", now: start.Add(time.Hour), want: MaintenanceWindowActive},
		{name: "expired at boundary", now: end, want: MaintenanceWindowExpired},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			evidence, err := BuildMaintenanceWindowEvidence(MaintenanceWindowInput{
				WindowID: "window-a", ChangeID: "change-a", RevisionID: "revision-a",
				RevisionDigest: "sha256:" + strings.Repeat("a", 64),
				StartsAt: start, EndsAt: end, ObservedAt: start.Add(-2 * time.Minute),
			}, test.now)
			if err != nil {
				t.Fatal(err)
			}
			if evidence.State != test.want {
				t.Fatalf("state=%q want=%q", evidence.State, test.want)
			}
			if err := ValidateMaintenanceWindowEvidence(evidence); err != nil {
				t.Fatalf("built evidence must validate: %v", err)
			}
		})
	}
}

func TestBuildMaintenanceWindowEvidenceRejectsFutureSourceEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	_, err := BuildMaintenanceWindowEvidence(MaintenanceWindowInput{
		WindowID: "window-a", ChangeID: "change-a", RevisionID: "revision-a",
		RevisionDigest: "sha256:" + strings.Repeat("b", 64),
		StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), ObservedAt: now.Add(time.Minute),
	}, now)
	if !errors.Is(err, ErrInvalidMaintenanceWindow) {
		t.Fatalf("expected future evidence rejection, got %v", err)
	}
}

func TestValidateMaintenanceWindowEvidenceRejectsInconsistentStateAndRange(t *testing.T) {
	start := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	base := MaintenanceWindowEvidence{
		ContractVersion: MaintenanceWindowContractVersion,
		WindowID: "window-a", ChangeID: "change-a", RevisionID: "revision-a",
		RevisionDigest: "sha256:" + strings.Repeat("c", 64),
		StartsAt: start, EndsAt: start.Add(time.Hour), ObservedAt: start.Add(-time.Minute),
		EvaluatedAt: start.Add(30 * time.Minute), State: MaintenanceWindowActive,
	}
	wrongState := base
	wrongState.State = MaintenanceWindowScheduled
	if err := ValidateMaintenanceWindowEvidence(wrongState); !errors.Is(err, ErrInvalidMaintenanceWindow) {
		t.Fatalf("expected inconsistent state rejection, got %v", err)
	}
	wrongRange := base
	wrongRange.EndsAt = wrongRange.StartsAt
	if err := ValidateMaintenanceWindowEvidence(wrongRange); !errors.Is(err, ErrInvalidMaintenanceWindow) {
		t.Fatalf("expected invalid range rejection, got %v", err)
	}
}

func TestValidateMaintenanceWindowEvidenceRejectsNonCanonicalDigest(t *testing.T) {
	start := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	evidence := MaintenanceWindowEvidence{
		ContractVersion: MaintenanceWindowContractVersion,
		WindowID: "window-a", ChangeID: "change-a", RevisionID: "revision-a",
		RevisionDigest: "sha256:" + strings.Repeat("A", 64),
		StartsAt: start, EndsAt: start.Add(time.Hour), ObservedAt: start.Add(-time.Minute),
		EvaluatedAt: start, State: MaintenanceWindowActive,
	}
	if err := ValidateMaintenanceWindowEvidence(evidence); !errors.Is(err, ErrInvalidMaintenanceWindow) {
		t.Fatalf("expected canonical digest rejection, got %v", err)
	}
}
