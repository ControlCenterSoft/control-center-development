package releasecandidate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func release032Root(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readRelease032(t *testing.T, root, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRelease032QualificationContract(t *testing.T) {
	root := release032Root(t)
	if got := strings.TrimSpace(readRelease032(t, root, "VERSION")); got != "0.32.0" {
		t.Fatalf("VERSION=%q", got)
	}

	gate := readRelease032(t, root, ".github/workflows/qualify-release-032.yml")
	for _, marker := range []string{
		"name: Qualify Control Center 0.32 exact SHA",
		"workflow_run:",
		"- Public CI",
		"github.event.workflow_run.conclusion == 'success'",
		"github.event.workflow_run.head_branch == 'main'",
		"QUALIFICATION_STALE_MAIN",
		"bash scripts/qualify-candidate-032.sh",
		"control-center-0.32.0-release-evidence-${{ env.CANDIDATE_SHA }}",
		"CC_032_EXACT_SHA_GATE=PASS",
	} {
		if !strings.Contains(gate, marker) {
			t.Fatalf("qualification workflow missing %q", marker)
		}
	}

	publisher := readRelease032(t, root, ".github/workflows/publish-release.yml")
	for _, marker := range []string{
		"- Qualify Control Center 0.32 exact SHA",
		"0.32.0)",
		"RELEASE_STALLED: Control Center 0.32.0 requires the dedicated exact-SHA qualification workflow",
		"control-center-0.32.0-release-evidence-${RELEASE_SHA}",
		"PUBLIC_CI_RUN_NOT_EXACT_PASS",
		"Stable promotion remains a separate required step",
	} {
		if !strings.Contains(publisher, marker) {
			t.Fatalf("publisher missing %q", marker)
		}
	}

	qualifier := readRelease032(t, root, "scripts/qualify-candidate-032.sh")
	for _, marker := range []string{
		"stable_version=\"0.31.1\"",
		"stable_sha256=\"b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97\"",
		"qualification checkout is not exact candidate SHA",
		"reproducible_packaging",
		"upgrade_from_stable_0_31_1",
		"rollback_forward_recovery",
		"publication_authority\":False",
	} {
		if !strings.Contains(qualifier, marker) {
			t.Fatalf("qualifier missing %q", marker)
		}
	}
}

func TestRelease032ScriptsAndSBOM(t *testing.T) {
	root := release032Root(t)
	for _, script := range []string{"scripts/build-candidate-032.sh", "scripts/qualify-candidate-032.sh"} {
		cmd := exec.Command("bash", "-n", filepath.Join(root, filepath.FromSlash(script)))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash -n %s: %v\n%s", script, err, out)
		}
	}

	outPath := filepath.Join(t.TempDir(), "sbom.json")
	sha := strings.Repeat("a", 40)
	cmd := exec.Command(
		"python3",
		filepath.Join(root, "scripts", "generate-sbom-032.py"),
		filepath.Join(root, "third_party", "manifest-0.32.json"),
		outPath,
		sha,
	)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate 0.32 SBOM: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var bom struct {
		Metadata struct {
			Component struct {
				Version    string `json:"version"`
				Properties []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"properties"`
			} `json:"component"`
		} `json:"metadata"`
		Components []json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(raw, &bom); err != nil {
		t.Fatal(err)
	}
	if bom.Metadata.Component.Version != "0.32.0" || len(bom.Components) != 9 {
		t.Fatalf("unexpected SBOM identity/version: version=%q components=%d", bom.Metadata.Component.Version, len(bom.Components))
	}
	foundSHA := false
	for _, p := range bom.Metadata.Component.Properties {
		if p.Name == "control-center:candidate-sha" && p.Value == sha {
			foundSHA = true
		}
	}
	if !foundSHA {
		t.Fatal("SBOM is not bound to exact candidate SHA")
	}
}
