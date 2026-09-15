#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

candidate_version="$(tr -d '\r\n' < VERSION)"
case "$candidate_version" in
  0.32.0) manifest_path="third_party/manifest-0.32.json" ;;
  0.32.1) manifest_path="third_party/manifest-0.32.1.json" ;;
  *) echo "unsupported 0.32 release identity: $candidate_version" >&2; exit 2 ;;
esac
release_notes="docs/RELEASE_${candidate_version}_RU.md"
[[ -f "$release_notes" ]] || { echo "release notes missing for $candidate_version" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 2; }

commit="${CANDIDATE_SHA:-$(git rev-parse HEAD)}"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "candidate SHA must be exact 40-char commit" >&2; exit 2; }
[[ "$(git rev-parse HEAD)" == "$commit" ]] || { echo "checked-out HEAD must equal candidate SHA" >&2; exit 2; }
source_date_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct "$commit")}"
[[ "$source_date_epoch" =~ ^[0-9]+$ ]] || { echo "invalid SOURCE_DATE_EPOCH" >&2; exit 2; }
build_time="$(date -u -d "@$source_date_epoch" +%Y-%m-%dT%H:%M:%SZ)"

dist_dir="${DIST_DIR:-$repo_root/dist/candidate-$candidate_version}"
rm -rf "$dist_dir"
mkdir -p "$dist_dir"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bundle="control-center-$candidate_version"
mkdir -p "$stage/$bundle/bin" "$stage/$bundle/api" "$stage/$bundle/config" "$stage/$bundle/deploy/systemd" "$stage/$bundle/migrations" "$stage/$bundle/scripts" "$stage/$bundle/docs"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath -buildvcs=false \
  -ldflags="-s -w -X control-center/internal/buildinfo.Version=$candidate_version -X control-center/internal/buildinfo.Commit=$commit -X control-center/internal/buildinfo.BuildTime=$build_time" \
  -o "$stage/$bundle/bin/control-center" ./cmd/control-center

cp -a api/. "$stage/$bundle/api/"
cp config/control-center.env.example "$stage/$bundle/config/"
cp deploy/systemd/control-center.service "$stage/$bundle/deploy/systemd/"
find migrations -maxdepth 1 -type f \( -name '*.sql' -o -name 'README.md' \) -exec cp {} "$stage/$bundle/migrations/" \;
cp scripts/migrate.sh "$stage/$bundle/scripts/"
cp "$release_notes" "$stage/$bundle/docs/"

cp -a third_party "$stage/$bundle/third_party"
cp THIRD_PARTY_NOTICES.md "$stage/$bundle/THIRD_PARTY_NOTICES.md"
sbom="$dist_dir/control-center-$candidate_version.sbom.cdx.json"
python3 scripts/generate-sbom-032.py "$manifest_path" "$sbom" "$commit"
cp "$sbom" "$stage/$bundle/docs/SBOM.cdx.json"
cp THIRD_PARTY_NOTICES.md "$dist_dir/THIRD_PARTY_NOTICES.md"

printf '%s\n' "$candidate_version" > "$stage/$bundle/VERSION"
printf '%s\n' "$commit" > "$stage/$bundle/REVISION"
printf '%s\n' "$build_time" > "$stage/$bundle/BUILD_TIME"
chmod 0755 "$stage/$bundle/bin/control-center" "$stage/$bundle/scripts/migrate.sh"

artifact="$dist_dir/control-center-$candidate_version-linux-amd64.tar.gz"
tar --sort=name --mtime="@$source_date_epoch" --owner=0 --group=0 --numeric-owner -C "$stage" -cf - "$bundle" | gzip -n > "$artifact"
(
  cd "$dist_dir"
  sha256sum "$(basename "$artifact")" > "$(basename "$artifact").sha256"
)

source_artifact="$dist_dir/control-center-$candidate_version-source.tar.gz"
git archive --format=tar --prefix="control-center-$candidate_version-source/" "$commit" | gzip -n > "$source_artifact"

printf 'CANDIDATE_VERSION=%s\nCANDIDATE_SHA=%s\nBUILD_TIME=%s\nBINARY_ARTIFACT=%s\nSOURCE_ARTIFACT=%s\nSBOM=%s\nTHIRD_PARTY_NOTICES=%s\n' \
  "$candidate_version" "$commit" "$build_time" "$artifact" "$source_artifact" "$sbom" "$dist_dir/THIRD_PARTY_NOTICES.md"
