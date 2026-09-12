package release

import "testing"

func TestEvaluateArtifactGateCandidate(t *testing.T) {
	blockers := EvaluateArtifactGate("candidate", candidateArtifactEvidence())
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %#v", blockers)
	}
}

func TestEvaluateArtifactGateStable(t *testing.T) {
	blockers := EvaluateArtifactGate("stable", stableArtifactEvidence())
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %#v", blockers)
	}
}

func TestEvaluateArtifactGateStableRequiresPublicReleaseMetadata(t *testing.T) {
	blockers := EvaluateArtifactGate("stable", candidateArtifactEvidence())
	want := []string{"source_artifact_digest", "sha256sums", "release_manifest"}
	if len(blockers) != len(want) {
		t.Fatalf("blockers=%#v want=%#v", blockers, want)
	}
	for i := range want {
		if blockers[i] != want[i] {
			t.Fatalf("blockers=%#v want=%#v", blockers, want)
		}
	}
}

func TestEvaluateArtifactGateDoesNotInventSignatureRequirement(t *testing.T) {
	evidence := candidateArtifactEvidence()
	blockers := EvaluateArtifactGate("candidate", evidence)
	for _, blocker := range blockers {
		if blocker == "signature" {
			t.Fatal("checksum/provenance model must not invent detached signature support")
		}
	}
}

func TestEvaluateArtifactGateRejectsInvalidDigest(t *testing.T) {
	evidence := candidateArtifactEvidence()
	evidence.BinaryDigest = "sha256:not-a-digest"
	blockers := EvaluateArtifactGate("candidate", evidence)
	if len(blockers) != 1 || blockers[0] != "artifact_digest" {
		t.Fatalf("unexpected blockers: %#v", blockers)
	}
}
