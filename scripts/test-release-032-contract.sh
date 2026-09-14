#!/usr/bin/env bash
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

test "$(tr -d '\r\n' < VERSION)" = "0.32.1"
bash -n scripts/build-candidate-032.sh
bash -n scripts/qualify-candidate-032.sh
python3 scripts/generate-sbom-032.py third_party/manifest-0.32.json "$(mktemp)" aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

grep -Fq 'name: Qualify Control Center 0.32 exact SHA' .github/workflows/qualify-release-032.yml
grep -Fq 'QUALIFICATION_STALE_MAIN' .github/workflows/qualify-release-032.yml
grep -Fq 'Qualify Control Center 0.32 exact SHA' .github/workflows/publish-release.yml
grep -Fq 'RELEASE_STALLED: Control Center 0.32.1 requires the dedicated exact-SHA qualification workflow' .github/workflows/publish-release.yml

echo RELEASE_032_CONTRACT=PASS
