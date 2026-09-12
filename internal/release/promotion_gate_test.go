package release

import (
	"strings"
	"testing"
)

func approvedCommercialEvidence() CommercialEvidence {
	return CommercialEvidence{
		Disposition:               CommercialDispositionApproved,
		EvidenceDigest:            "sha256:" + strings.Repeat("a", 64),
		DependenciesReviewed:      true,
		RedistributionReviewed:    true,
		NoticesPrepared:           true,
		SourceObligationsResolved: true,
		SBOMPrepared:              true,
		LegalTermsDispositioned:   true,
		ReleaseClaimsReviewed:     true,
	}
}

func TestEvaluatePromotionGateStable(t *testing.T) {
	decision, err := EvaluatePromotionGate("stable", PromotionEvidence{
		Version:          "1.0.0",
		Revision:         strings.Repeat("b", 40),
		TestsPassed:      true,
		SecurityPassed:   true,
		ArtifactSigned:   true,
		RollbackPrepared: true,
		Commercial:       approvedCommercialEvidence(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed || len(decision.Blockers) != 0 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestEvaluatePromotionGateStableReportsBlockers(t *testing.T) {
	decision, err := EvaluatePromotionGate("stable", PromotionEvidence{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"tests",
		"security",
		"revision",
		"signature",
		"rollback",
		"commercial_disposition",
		"commercial_evidence",
		"third_party_dependencies",
		"redistribution",
		"third_party_notices",
		"source_obligations",
		"sbom",
		"legal_terms",
		"release_claims",
	}
	if len(decision.Blockers) != len(want) {
		t.Fatalf("blockers=%#v want=%#v", decision.Blockers, want)
	}
	for i := range want {
		if decision.Blockers[i] != want[i] {
			t.Fatalf("blockers=%#v want=%#v", decision.Blockers, want)
		}
	}
}

func TestEvaluatePromotionGateCandidateRequiresRollbackAndCommercialEvidence(t *testing.T) {
	decision, err := EvaluatePromotionGate("candidate", PromotionEvidence{
		Version:        "0.31.0",
		Revision:       strings.Repeat("c", 40),
		TestsPassed:    true,
		SecurityPassed: true,
		ArtifactSigned: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatal("candidate promotion unexpectedly allowed")
	}
	if len(decision.Blockers) == 0 || decision.Blockers[0] != "rollback" {
		t.Fatalf("unexpected blockers: %#v", decision.Blockers)
	}
}

func TestEvaluatePromotionGateDevelopmentDoesNotRequireReleaseEvidence(t *testing.T) {
	decision, err := EvaluatePromotionGate("development", PromotionEvidence{Version: "1.0.0-dev", TestsPassed: true, SecurityPassed: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("unexpected blockers: %#v", decision.Blockers)
	}
}

func TestEvaluatePromotionGateRejectsUnboundRevision(t *testing.T) {
	decision, err := EvaluatePromotionGate("candidate", PromotionEvidence{
		Version:          "0.31.0",
		Revision:         "latest",
		TestsPassed:      true,
		SecurityPassed:   true,
		ArtifactSigned:   true,
		RollbackPrepared: true,
		Commercial:       approvedCommercialEvidence(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed || len(decision.Blockers) != 1 || decision.Blockers[0] != "revision" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}
