#!/usr/bin/env bash
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

version="$(tr -d '\r\n' < VERSION)"
case "$version" in
  0.32.0|0.32.1) ;;
  *) echo "unsupported 0.32 release identity: $version" >&2; exit 2 ;;
esac

bash -n scripts/build-candidate-032.sh
bash -n scripts/qualify-candidate-032.sh
python3 -m py_compile scripts/generate-sbom-032.py

test -f third_party/manifest-0.32.json
test -f third_party/manifest-0.32.1.json
current_manifest="third_party/manifest-${version%.*}.json"
if [[ "$version" == "0.32.1" ]]; then current_manifest="third_party/manifest-0.32.1.json"; fi
current_sbom="$(mktemp)"
python3 scripts/generate-sbom-032.py "$current_manifest" "$current_sbom" aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
python3 - "$current_sbom" "$version" <<'PY'
import json,sys
path,version=sys.argv[1:]
with open(path,encoding='utf-8') as f: bom=json.load(f)
assert bom['metadata']['component']['version'] == version
PY
rm -f "$current_sbom"

grep -Fq 'candidate_version="$(tr -d' scripts/build-candidate-032.sh
grep -Fq '0.32.1) manifest_path="third_party/manifest-0.32.1.json"' scripts/build-candidate-032.sh
grep -Fq 'candidate_version="$(tr -d' scripts/qualify-candidate-032.sh
grep -Fq '0.32.1) candidate_manifest="manifest-0.32.1.json"' scripts/qualify-candidate-032.sh
grep -Fq 'SUPPORTED_VERSIONS = {"0.32.0", "0.32.1"}' scripts/generate-sbom-032.py

# Prove the SBOM path accepts the successor patch identity without leaving the
# checkout dirty after the contract test.
version_backup="$(mktemp)"
cp VERSION "$version_backup"
temp_sbom="$(mktemp)"
restore_version() {
  cp "$version_backup" VERSION
  rm -f "$version_backup" "$temp_sbom"
}
trap restore_version EXIT
printf '0.32.1\n' > VERSION
python3 scripts/generate-sbom-032.py third_party/manifest-0.32.1.json "$temp_sbom" bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
python3 - "$temp_sbom" <<'PY'
import json,sys
with open(sys.argv[1],encoding='utf-8') as f: bom=json.load(f)
assert bom['metadata']['component']['version'] == '0.32.1'
assert bom['metadata']['component']['bom-ref'].startswith('pkg:generic/control-center@0.32.1?revision=')
PY
restore_version
trap - EXIT

grep -Fq 'name: Qualify Control Center 0.32 exact SHA' .github/workflows/qualify-release-032.yml
grep -Fq 'QUALIFICATION_STALE_MAIN' .github/workflows/qualify-release-032.yml
grep -Fq 'Qualify Control Center 0.32 exact SHA' .github/workflows/publish-release.yml
grep -Fq 'RELEASE_STALLED: Control Center 0.32.0 requires the dedicated exact-SHA qualification workflow' .github/workflows/publish-release.yml

test -f .github/workflows/qualify-release-0321.yml
grep -Fq 'name: Qualify Control Center 0.32.1 exact SHA' .github/workflows/qualify-release-0321.yml
grep -Fq 'branches:' .github/workflows/qualify-release-0321.yml
grep -Fq -- "- 'release/0.32.1'" .github/workflows/qualify-release-0321.yml
grep -Fq 'QUALIFICATION_STALE_RELEASE_BRANCH' .github/workflows/qualify-release-0321.yml
grep -Fq 'control-center-0.32.1-release-evidence-' .github/workflows/qualify-release-0321.yml

test -f .github/workflows/publish-release-0321.yml
grep -Fq 'name: Publish Control Center 0.32.1 source release' .github/workflows/publish-release-0321.yml
grep -Fq 'RELEASE_TAG: v0.32.1' .github/workflows/publish-release-0321.yml
grep -Fq 'SOURCE_TAG_SHA_DRIFT' .github/workflows/publish-release-0321.yml
grep -Fq 'PUBLISHED_RELEASE_EVIDENCE_INCOMPLETE' .github/workflows/publish-release-0321.yml
grep -Fq 'gh release create "$RELEASE_TAG" dist/public-stable-evidence/*' .github/workflows/publish-release-0321.yml
grep -Fq 'Public Stable promotion remains a separate PR-based gate' .github/workflows/publish-release-0321.yml

echo RELEASE_032_CONTRACT=PASS
