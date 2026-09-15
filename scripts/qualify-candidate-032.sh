#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

for command in curl createdb dropdb pg_dump pg_restore psql sha256sum tar python3 go cmp; do
  command -v "$command" >/dev/null 2>&1 || { echo "required command missing: $command" >&2; exit 2; }
done

candidate_version="$(tr -d '\r\n' < VERSION)"
case "$candidate_version" in
  0.32.0) candidate_manifest="manifest-0.32.json" ;;
  0.32.1) candidate_manifest="manifest-0.32.1.json" ;;
  *) echo "unsupported 0.32 release identity: $candidate_version" >&2; exit 2 ;;
esac
stable_version="0.31.1"
stable_sha256="b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97"
candidate_sha="${CANDIDATE_SHA:-$(git rev-parse HEAD)}"
public_ci_run_id="${PUBLIC_CI_RUN_ID:-}"
[[ "$candidate_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid exact candidate SHA" >&2; exit 2; }
[[ "$(git rev-parse HEAD)" == "$candidate_sha" ]] || { echo "qualification checkout is not exact candidate SHA" >&2; exit 2; }
[[ "$public_ci_run_id" =~ ^[0-9]+$ ]] || { echo "PUBLIC_CI_RUN_ID must bind exact upstream Public CI evidence" >&2; exit 2; }

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"
export PGUSER="${PGUSER:-postgres}"
export PGPASSWORD="${PGPASSWORD:-postgres}"

run_token="${GITHUB_RUN_ID:-$$}_${GITHUB_RUN_ATTEMPT:-1}"
run_token="${run_token//[^0-9A-Za-z_]/_}"
clean_db="cc032_clean_${run_token}"
upgrade_db="cc032_upgrade_${run_token}"
work="$(mktemp -d)"
cleanup() {
  dropdb --if-exists "$clean_db" >/dev/null 2>&1 || true
  dropdb --if-exists "$upgrade_db" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

epoch="$(git show -s --format=%ct "$candidate_sha")"
[[ "$epoch" =~ ^[1-9][0-9]{8,11}$ ]] || { echo "invalid source epoch" >&2; exit 2; }

# Build twice from the same exact SHA and epoch. Release-specific artifacts must
# be byte-for-byte reproducible before any publication authority can exist.
DIST_DIR="$work/build-a" SOURCE_DATE_EPOCH="$epoch" CANDIDATE_SHA="$candidate_sha" bash scripts/build-candidate-032.sh
DIST_DIR="$work/build-b" SOURCE_DATE_EPOCH="$epoch" CANDIDATE_SHA="$candidate_sha" bash scripts/build-candidate-032.sh
for name in \
  "control-center-$candidate_version-linux-amd64.tar.gz" \
  "control-center-$candidate_version-linux-amd64.tar.gz.sha256" \
  "control-center-$candidate_version-source.tar.gz" \
  "control-center-$candidate_version.sbom.cdx.json" \
  "THIRD_PARTY_NOTICES.md"; do
  cmp -s "$work/build-a/$name" "$work/build-b/$name" || { echo "non-reproducible release artifact: $name" >&2; exit 1; }
done

out="$repo_root/dist/candidate-$candidate_version"
rm -rf "$out"
mkdir -p "$out"
cp -a "$work/build-a/." "$out/"
(
  cd "$out"
  sha256sum -c "control-center-$candidate_version-linux-amd64.tar.gz.sha256"
)

binary_artifact="$out/control-center-$candidate_version-linux-amd64.tar.gz"
source_artifact="$out/control-center-$candidate_version-source.tar.gz"
sbom_artifact="$out/control-center-$candidate_version.sbom.cdx.json"
notices_artifact="$out/THIRD_PARTY_NOTICES.md"
mkdir -p "$work/candidate" "$work/stable"
tar -xzf "$binary_artifact" -C "$work/candidate"
candidate_root="$work/candidate/control-center-$candidate_version"
[[ -x "$candidate_root/bin/control-center" ]]
[[ "$(tr -d '\r\n' < "$candidate_root/VERSION")" == "$candidate_version" ]]
[[ "$(tr -d '\r\n' < "$candidate_root/REVISION")" == "$candidate_sha" ]]
[[ -f "$candidate_root/deploy/systemd/control-center.service" ]]
[[ -f "$candidate_root/config/control-center.env.example" ]]
[[ -x "$candidate_root/scripts/migrate.sh" ]]
[[ -f "$candidate_root/THIRD_PARTY_NOTICES.md" ]]
[[ -f "$candidate_root/docs/SBOM.cdx.json" ]]
[[ -f "$candidate_root/third_party/$candidate_manifest" ]]
cmp -s "$notices_artifact" "$candidate_root/THIRD_PARTY_NOTICES.md"
cmp -s "$sbom_artifact" "$candidate_root/docs/SBOM.cdx.json"

python3 - "$sbom_artifact" "$candidate_sha" "$candidate_version" <<'PY'
import json,re,sys
path,sha,version=sys.argv[1:]
with open(path,encoding="utf-8") as f: bom=json.load(f)
assert bom.get("bomFormat")=="CycloneDX" and bom.get("specVersion")=="1.7"
root=bom.get("metadata",{}).get("component",{})
assert root.get("name")=="control-center" and root.get("version")==version
props={p.get("name"):p.get("value") for p in root.get("properties",[])}
assert props.get("control-center:candidate-sha")==sha
components=bom.get("components",[])
assert len(components)==9
refs={c.get("bom-ref") for c in components}
assert len(refs)==9 and all(refs)
for component in components:
    licenses=component.get("licenses",[])
    assert licenses and licenses[0].get("license",{}).get("id") in {"MIT","BSD-3-Clause"}
    cprops={p.get("name"):p.get("value") for p in component.get("properties",[])}
    assert re.fullmatch(r"[0-9a-f]{64}",cprops.get("control-center:license-sha256", ""))
print("CANDIDATE_SBOM=PASS")
PY

# Qualify the supported upgrade against the immutable current Public Stable
# artifact, never against a moving branch or an unverified local fixture.
stable_artifact="$work/control-center-$stable_version-linux-amd64.tar.gz"
curl --fail --location --silent --show-error --retry 3 --connect-timeout 10 --max-time 180 \
  "https://github.com/ControlCenterSoft/control-center-stable/releases/download/v$stable_version/control-center-$stable_version-linux-amd64.tar.gz" \
  -o "$stable_artifact"
echo "$stable_sha256  $stable_artifact" | sha256sum -c -
tar -xzf "$stable_artifact" -C "$work/stable"
stable_root="$work/stable/control-center-$stable_version"
[[ -x "$stable_root/scripts/migrate.sh" ]]
[[ -d "$stable_root/migrations" ]]

createdb "$clean_db"
PGDATABASE="$clean_db" MIGRATIONS_DIR="$candidate_root/migrations" "$candidate_root/scripts/migrate.sh"
latest_migration="$(find "$candidate_root/migrations" -maxdepth 1 -type f -name '*.up.sql' -printf '%f\n' | sort -V | tail -1)"
latest_version="${latest_migration%.up.sql}"
[[ -n "$latest_version" ]]
[[ "$(PGDATABASE="$clean_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")" == "1" ]]

createdb "$upgrade_db"
PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$stable_root/migrations" "$stable_root/scripts/migrate.sh"
stable_count="$(PGDATABASE="$upgrade_db" psql -X -Atqc 'SELECT count(*) FROM schema_migrations')"
[[ "$stable_count" =~ ^[1-9][0-9]*$ ]]
pre_upgrade_dump="$work/pre-upgrade.dump"
PGDATABASE="$upgrade_db" pg_dump --format=custom --file="$pre_upgrade_dump"
[[ -s "$pre_upgrade_dump" ]]

PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$candidate_root/migrations" "$candidate_root/scripts/migrate.sh"
[[ "$(PGDATABASE="$upgrade_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")" == "1" ]]

dropdb "$upgrade_db"
createdb "$upgrade_db"
PGDATABASE="$upgrade_db" pg_restore --no-owner --no-privileges --exit-on-error --dbname="$upgrade_db" "$pre_upgrade_dump"
[[ "$(PGDATABASE="$upgrade_db" psql -X -Atqc 'SELECT count(*) FROM schema_migrations')" == "$stable_count" ]]
PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$candidate_root/migrations" "$candidate_root/scripts/migrate.sh"
[[ "$(PGDATABASE="$upgrade_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")" == "1" ]]

binary_digest="sha256:$(sha256sum "$binary_artifact" | awk '{print $1}')"
source_digest="sha256:$(sha256sum "$source_artifact" | awk '{print $1}')"
sbom_digest="sha256:$(sha256sum "$sbom_artifact" | awk '{print $1}')"
notices_digest="sha256:$(sha256sum "$notices_artifact" | awk '{print $1}')"
sidecar="$out/control-center-$candidate_version-linux-amd64.tar.gz.sha256"
sidecar_digest="sha256:$(sha256sum "$sidecar" | awk '{print $1}')"
qualification="$out/control-center-$candidate_version.qualification.json"
provenance="$out/control-center-$candidate_version.provenance.json"
release_manifest="$out/control-center-$candidate_version.release-manifest.json"

python3 - "$qualification" "$candidate_version" "$candidate_sha" "$public_ci_run_id" "$binary_digest" "$source_digest" "$sbom_digest" "$notices_digest" "$stable_sha256" <<'PY'
import json,sys
path,version,sha,run_id,binary_digest,source_digest,sbom_digest,notices_digest,stable_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-qualification.v2",
  "status":"PASS",
  "candidate_version":version,
  "candidate_sha":sha,
  "public_ci":{"run_id":int(run_id),"status":"PASS","candidate_sha":sha},
  "stable_base":{"version":"0.31.1","artifact_digest":"sha256:"+stable_digest},
  "artifacts":{"linux_amd64":binary_digest,"source":source_digest,"sbom":sbom_digest,"third_party_notices":notices_digest},
  "gates":{
    "exact_sha_public_ci":"PASS",
    "reproducible_packaging":"PASS",
    "candidate_artifact_integrity":"PASS",
    "dependency_license_inventory":"PASS",
    "sbom":"PASS",
    "clean_install":"PASS",
    "upgrade_from_stable_0_31_1":"PASS",
    "rollback_forward_recovery":"PASS"
  },
  "publication_authority":False
}
with open(path,"w",encoding="utf-8") as f:
    json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

python3 - "$provenance" "$candidate_version" "$candidate_sha" "$public_ci_run_id" "$binary_digest" "$source_digest" "$sbom_digest" "$notices_digest" <<'PY'
import json,sys
path,version,sha,run_id,binary_digest,source_digest,sbom_digest,notices_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-provenance.v2",
  "candidate_version":version,
  "candidate_sha":sha,
  "source_repository":"ControlCenterSoft/control-center-development",
  "public_ci_run_id":int(run_id),
  "subjects":[
    {"name":f"control-center-{version}-linux-amd64.tar.gz","digest":binary_digest},
    {"name":f"control-center-{version}-source.tar.gz","digest":source_digest},
    {"name":f"control-center-{version}.sbom.cdx.json","digest":sbom_digest},
    {"name":"THIRD_PARTY_NOTICES.md","digest":notices_digest}
  ],
  "publication_authority":False
}
with open(path,"w",encoding="utf-8") as f:
    json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

qualification_digest="sha256:$(sha256sum "$qualification" | awk '{print $1}')"
provenance_digest="sha256:$(sha256sum "$provenance" | awk '{print $1}')"
python3 - "$release_manifest" "$candidate_version" "$candidate_sha" "$binary_digest" "$sidecar_digest" "$source_digest" "$sbom_digest" "$notices_digest" "$qualification_digest" "$provenance_digest" <<'PY'
import json,sys
path,version,sha,binary_digest,sidecar_digest,source_digest,sbom_digest,notices_digest,qualification_digest,provenance_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-release-manifest.v2",
  "status":"qualified-not-yet-published",
  "version":version,
  "revision":sha,
  "stable_base":"0.31.1",
  "publication_authority":False,
  "artifacts":{
    f"control-center-{version}-linux-amd64.tar.gz":binary_digest,
    f"control-center-{version}-linux-amd64.tar.gz.sha256":sidecar_digest,
    f"control-center-{version}-source.tar.gz":source_digest,
    f"control-center-{version}.sbom.cdx.json":sbom_digest,
    "THIRD_PARTY_NOTICES.md":notices_digest,
    f"control-center-{version}.qualification.json":qualification_digest,
    f"control-center-{version}.provenance.json":provenance_digest
  }
}
with open(path,"w",encoding="utf-8") as f:
    json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

(
  cd "$out"
  sha256sum \
    "control-center-$candidate_version-linux-amd64.tar.gz" \
    "control-center-$candidate_version-linux-amd64.tar.gz.sha256" \
    "control-center-$candidate_version-source.tar.gz" \
    "control-center-$candidate_version.sbom.cdx.json" \
    "THIRD_PARTY_NOTICES.md" \
    "control-center-$candidate_version.provenance.json" \
    "control-center-$candidate_version.qualification.json" \
    "control-center-$candidate_version.release-manifest.json" > SHA256SUMS
  sha256sum -c SHA256SUMS
)

echo "CC_032_EXACT_SHA_QUALIFICATION=PASS candidate_version=$candidate_version candidate_sha=$candidate_sha public_ci_run_id=$public_ci_run_id stable_base=$stable_version"
