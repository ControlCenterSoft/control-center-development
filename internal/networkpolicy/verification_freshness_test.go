package networkpolicy

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateVerificationFreshnessReady(t *testing.T) {
	now := time.Date(2026, 9, 11, 7, 10, 0, 0, time.UTC)
	evidence := validVerificationEvidence(now)
	verdict, err := EvaluateVerificationFreshness(
		now,
		evidence.PlanID,
		evidence.RevisionID,
		evidence,
		VerificationFreshnessPolicy{
			MaxAge:         10 * time.Minute,
			MaxFutureSkew: 30 * time.Second,
			RequiredChecks: []string{"control_plane", "link_state"},
		},
	)
	if err != nil {
		t.Fatalf("EvaluateVerificationFreshness() error = %v", err)
	}
	if !verdict.Ready {
		t.Fatalf("verdict.Ready = false, verdict = %#v", verdict)
	}
}

func TestEvaluateVerificationFreshnessFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 11, 7, 10, 0, 0, time.UTC)
	tests := []struct {
		name          string
		mutate        func(*VerificationEvidence, *VerificationFreshnessPolicy)
		wantError     bool
		wantReady     bool
		wantMissing   string
		wantStale     string
		wantFailed    string
	}{
		{
			name: "plan mismatch",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.PlanID = digestID("b")
			},
			wantError: true,
		},
		{
			name: "future verification",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.VerifiedAt = now.Add(2 * time.Minute)
			},
			wantError: true,
		},
		{
			name: "stale verification and check",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.VerifiedAt = now.Add(-20 * time.Minute)
				evidence.Checks[0].ObservedAt = evidence.VerifiedAt
				evidence.Checks[1].ObservedAt = evidence.VerifiedAt
			},
			wantStale: "verification",
		},
		{
			name: "missing required check",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.Checks = evidence.Checks[:1]
			},
			wantMissing: "link_state",
		},
		{
			name: "failed required check",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.Checks[0].Status = VerificationCheckFail
			},
			wantFailed: "control_plane",
		},
		{
			name: "duplicate evidence",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.Checks = append(evidence.Checks, evidence.Checks[0])
			},
			wantError: true,
		},
		{
			name: "malformed digest",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.Checks[0].EvidenceDigest = "sha256:not-a-digest"
			},
			wantError: true,
		},
		{
			name: "check newer than envelope",
			mutate: func(evidence *VerificationEvidence, _ *VerificationFreshnessPolicy) {
				evidence.Checks[0].ObservedAt = evidence.VerifiedAt.Add(2 * time.Minute)
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := validVerificationEvidence(now)
			policy := VerificationFreshnessPolicy{
				MaxAge:         10 * time.Minute,
				MaxFutureSkew: 30 * time.Second,
				RequiredChecks: []string{"control_plane", "link_state"},
			}
			expectedPlanID := evidence.PlanID
			test.mutate(&evidence, &policy)
			verdict, err := EvaluateVerificationFreshness(now, expectedPlanID, "rev-028", evidence, policy)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, verdict = %#v", verdict)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if verdict.Ready != test.wantReady {
				t.Fatalf("verdict.Ready = %v, want %v; verdict = %#v", verdict.Ready, test.wantReady, verdict)
			}
			if test.wantMissing != "" && !containsString(verdict.MissingChecks, test.wantMissing) {
				t.Fatalf("missing checks = %v, want %q", verdict.MissingChecks, test.wantMissing)
			}
			if test.wantStale != "" && !containsString(verdict.StaleChecks, test.wantStale) {
				t.Fatalf("stale checks = %v, want %q", verdict.StaleChecks, test.wantStale)
			}
			if test.wantFailed != "" && !containsString(verdict.FailedChecks, test.wantFailed) {
				t.Fatalf("failed checks = %v, want %q", verdict.FailedChecks, test.wantFailed)
			}
		})
	}
}

func validVerificationEvidence(now time.Time) VerificationEvidence {
	verifiedAt := now.Add(-time.Minute)
	return VerificationEvidence{
		SchemaVersion: VerificationFreshnessSchemaVersion,
		PlanID:        digestID("a"),
		RevisionID:    "rev-028",
		VerifiedAt:    verifiedAt,
		Checks: []VerificationCheckEvidence{
			{
				Name:           "control_plane",
				Status:         VerificationCheckPass,
				EvidenceDigest: digestID("c"),
				ObservedAt:     verifiedAt.Add(-10 * time.Second),
			},
			{
				Name:           "link_state",
				Status:         VerificationCheckPass,
				EvidenceDigest: digestID("d"),
				ObservedAt:     verifiedAt.Add(-20 * time.Second),
			},
		},
	}
}

func digestID(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
