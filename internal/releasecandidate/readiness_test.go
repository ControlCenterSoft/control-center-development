package releasecandidate

import (
	"strings"
	"testing"
)

func completeSnapshot() Snapshot {
	candidateSHA := strings.Repeat("b", 40)
	digest := "sha256:" + strings.Repeat("c", 64)
	gates := make([]GateEvidence, 0, len(RequiredGates))
	for _, gate := range RequiredGates {
		gates = append(gates, GateEvidence{
			Gate:           gate,
			Status:         GatePass,
			CandidateSHA:   candidateSHA,
			EvidenceDigest: digest,
		})
	}
	return Snapshot{
		Schema:               SchemaV1,
		StableVersion:        StableVersion,
		StableTag:            StableTag,
		StableArtifactDigest: "sha256:" + strings.Repeat("a", 64),
		CandidateVersion:     CandidateVersion,
		CandidateSHA:         candidateSHA,
		Gates:                gates,
	}
}

func TestEvaluateReadyOnlyWhenEveryRequiredGatePasses(t *testing.T) {
	result, err := Evaluate(completeSnapshot())
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Ready {
		t.Fatalf("Ready = false, blockers = %v", result.Blockers)
	}
	if len(result.Blockers) != 0 {
		t.Fatalf("Blockers = %v, want empty", result.Blockers)
	}
}

func TestEvaluateMissingGateBlocksReadiness(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates = snapshot.Gates[:len(snapshot.Gates)-1]

	result, err := Evaluate(snapshot)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Ready {
		t.Fatal("Ready = true, want false")
	}
	if len(result.Blockers) != 1 || result.Blockers[0] != GateReleaseMetadata {
		t.Fatalf("Blockers = %v, want [%s]", result.Blockers, GateReleaseMetadata)
	}
}

func TestEvaluatePendingGateBlocksWithoutInventingEvidence(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates[2].Status = GatePending
	snapshot.Gates[2].EvidenceDigest = ""

	result, err := Evaluate(snapshot)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Ready {
		t.Fatal("Ready = true, want false")
	}
	if len(result.Blockers) != 1 || result.Blockers[0] != GatePackaging {
		t.Fatalf("Blockers = %v, want [%s]", result.Blockers, GatePackaging)
	}
}

func TestEvaluateRejectsDuplicateGate(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates = append(snapshot.Gates, snapshot.Gates[0])

	if _, err := Evaluate(snapshot); err == nil {
		t.Fatal("Evaluate() error = nil, want duplicate gate error")
	}
}

func TestEvaluateRejectsUnknownGate(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates[0].Gate = GateID("future_unreviewed_gate")

	if _, err := Evaluate(snapshot); err == nil {
		t.Fatal("Evaluate() error = nil, want unknown gate error")
	}
}

func TestEvaluateRejectsEvidenceForAnotherCandidate(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates[0].CandidateSHA = strings.Repeat("d", 40)

	if _, err := Evaluate(snapshot); err == nil {
		t.Fatal("Evaluate() error = nil, want candidate binding error")
	}
}

func TestEvaluateRejectsPassWithoutDigest(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Gates[0].EvidenceDigest = ""

	if _, err := Evaluate(snapshot); err == nil {
		t.Fatal("Evaluate() error = nil, want missing evidence digest error")
	}
}

func TestEvaluateRejectsUnexpectedStableIdentity(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.StableVersion = "0.29.0"

	if _, err := Evaluate(snapshot); err == nil {
		t.Fatal("Evaluate() error = nil, want stable identity error")
	}
}
