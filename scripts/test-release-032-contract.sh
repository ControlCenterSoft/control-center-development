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
python3 scripts/generate-sbom-032.py third_party/manifest-0.32.json "$(mktemp)" aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

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
