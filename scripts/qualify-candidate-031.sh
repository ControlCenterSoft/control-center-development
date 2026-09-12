#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

for command in curl createdb dropdb pg_dump pg_restore psql sha256sum tar python3 go; do
  command -v "$command" >/dev/null 2>&1 || { echo "required command missing: $command" >&2; exit 2; }
done

candidate_version=0.31.0
stable_version=0.30.0
stable_sha256=02d15e8ff13bbcb52b6d0c9293ab8804500991fbb41c8b575e88306a8a5ce0f2
candidate_sha="${CANDIDATE_SHA:-$(git rev-parse HEAD)}"
[[ "$candidate_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid exact candidate SHA" >&2; exit 2; }

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"
export PGUSER="${PGUSER:-postgres}"
export PGPASSWORD="${PGPASSWORD:-postgres}"

run_token="${GITHUB_RUN_ID:-$$}_${GITHUB_RUN_ATTEMPT:-1}"
run_token="${run_token//[^0-9A-Za-z_]/_}"
clean_db="cc031_clean_${run_token}"
upgrade_db="cc031_upgrade_${run_token}"
work="$(mktemp -d)"
cleanup() {
  dropdb --if-exists "$clean_db" >/dev/null 2>&1 || true
  dropdb --if-exists "$upgrade_db" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

CANDIDATE_SHA="$candidate_sha" bash scripts/build-candidate-031.sh
out="$repo_root/dist/candidate-0.31.0"
binary_artifact="$out/control-center-$candidate_version-linux-amd64.tar.gz"
source_artifact="$out/control-center-$candidate_version-source.tar.gz"
(
  cd "$out"
  sha256sum -c "control-center-$candidate_version-linux-amd64.tar.gz.sha256"
)

mkdir -p "$work/candidate" "$work/stable"
tar -xzf "$binary_artifact" -C "$work/candidate"
candidate_root="$work/candidate/control-center-$candidate_version"
[[ -x "$candidate_root/bin/control-center" ]]
[[ "$(tr -d '\r\n' < "$candidate_root/VERSION")" == "$candidate_version" ]]
[[ "$(tr -d '\r\n' < "$candidate_root/REVISION")" == "$candidate_sha" ]]
[[ -f "$candidate_root/deploy/systemd/control-center.service" ]]
[[ -f "$candidate_root/config/control-center.env.example" ]]
[[ -x "$candidate_root/scripts/migrate.sh" ]]

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
clean_applied="$(PGDATABASE="$clean_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")"
[[ "$clean_applied" == "1" ]] || { echo "clean install did not apply latest migration $latest_version" >&2; exit 1; }

createdb "$upgrade_db"
PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$stable_root/migrations" "$stable_root/scripts/migrate.sh"
stable_count="$(PGDATABASE="$upgrade_db" psql -X -Atqc 'SELECT count(*) FROM schema_migrations')"
[[ "$stable_count" =~ ^[1-9][0-9]*$ ]]
pre_upgrade_dump="$work/pre-upgrade.dump"
PGDATABASE="$upgrade_db" pg_dump --format=custom --file="$pre_upgrade_dump"
[[ -s "$pre_upgrade_dump" ]]

PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$candidate_root/migrations" "$candidate_root/scripts/migrate.sh"
upgrade_applied="$(PGDATABASE="$upgrade_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")"
[[ "$upgrade_applied" == "1" ]] || { echo "artifact upgrade did not apply latest migration $latest_version" >&2; exit 1; }

# Roll back by restoring the exact pre-upgrade database snapshot, then prove a fresh
# forward recovery using the candidate artifact again. This is deliberately data-
# preserving rather than a destructive down-migration path.
dropdb "$upgrade_db"
createdb "$upgrade_db"
PGDATABASE="$upgrade_db" pg_restore --no-owner --no-privileges --exit-on-error --dbname="$upgrade_db" "$pre_upgrade_dump"
rollback_count="$(PGDATABASE="$upgrade_db" psql -X -Atqc 'SELECT count(*) FROM schema_migrations')"
[[ "$rollback_count" == "$stable_count" ]] || { echo "rollback snapshot migration count mismatch" >&2; exit 1; }
rollback_latest="$(PGDATABASE="$upgrade_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")"
[[ "$rollback_latest" == "0" ]] || { echo "rollback retained candidate-only migration $latest_version" >&2; exit 1; }
PGDATABASE="$upgrade_db" MIGRATIONS_DIR="$candidate_root/migrations" "$candidate_root/scripts/migrate.sh"
forward_applied="$(PGDATABASE="$upgrade_db" psql -X -Atqc "SELECT count(*) FROM schema_migrations WHERE version='$latest_version'")"
[[ "$forward_applied" == "1" ]] || { echo "forward recovery did not reapply candidate migration" >&2; exit 1; }

binary_digest="sha256:$(sha256sum "$binary_artifact" | awk '{print $1}')"
source_digest="sha256:$(sha256sum "$source_artifact" | awk '{print $1}')"
sidecar="$out/control-center-$candidate_version-linux-amd64.tar.gz.sha256"
sidecar_digest="sha256:$(sha256sum "$sidecar" | awk '{print $1}')"
qualification="$out/control-center-$candidate_version.qualification.json"
provenance="$out/control-center-$candidate_version.provenance.json"
release_manifest="$out/control-center-$candidate_version.release-manifest.json"

python3 - "$qualification" "$candidate_sha" "$binary_digest" "$source_digest" "$stable_sha256" <<'PY'
import json,sys
path,sha,binary_digest,source_digest,stable_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-qualification.v1",
  "status":"PARTIAL_PASS_NOT_RC",
  "candidate_version":"0.31.0",
  "candidate_sha":sha,
  "stable_base":{"version":"0.30.0","artifact_digest":"sha256:"+stable_digest},
  "artifacts":{"linux_amd64":binary_digest,"source":source_digest},
  "gates":{
    "candidate_artifact_packaging":"PASS",
    "clean_install":"PASS",
    "upgrade_from_stable_0_30":"PASS",
    "rollback_forward_recovery":"PASS"
  },
  "not_claimed":["security_privacy","commercial_legal_clearance","release_metadata","public_stable_promotion"]
}
with open(path,"w",encoding="utf-8") as f: json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

python3 - "$provenance" "$candidate_sha" "$binary_digest" "$source_digest" <<'PY'
import json,os,sys
path,sha,binary_digest,source_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-provenance.v1",
  "candidate_version":"0.31.0",
  "candidate_sha":sha,
  "source_repository":"ControlCenterSoft/control-center-development",
  "builder":"github-actions" if os.getenv("GITHUB_ACTIONS")=="true" else "local-qualified-runner",
  "subjects":[
    {"name":"control-center-0.31.0-linux-amd64.tar.gz","digest":binary_digest},
    {"name":"control-center-0.31.0-source.tar.gz","digest":source_digest}
  ],
  "publication_authority":False
}
with open(path,"w",encoding="utf-8") as f: json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

qualification_digest="sha256:$(sha256sum "$qualification" | awk '{print $1}')"
provenance_digest="sha256:$(sha256sum "$provenance" | awk '{print $1}')"
python3 - "$release_manifest" "$candidate_sha" "$binary_digest" "$sidecar_digest" "$source_digest" "$qualification_digest" "$provenance_digest" <<'PY'
import json,sys
path,sha,binary_digest,sidecar_digest,source_digest,qualification_digest,provenance_digest=sys.argv[1:]
data={
  "schema":"control-center.candidate-release-manifest.v1",
  "status":"candidate-only-not-public-stable",
  "version":"0.31.0",
  "revision":sha,
  "stable_base":"0.30.0",
  "publication_authority":False,
  "artifacts":{
    "control-center-0.31.0-linux-amd64.tar.gz":binary_digest,
    "control-center-0.31.0-linux-amd64.tar.gz.sha256":sidecar_digest,
    "control-center-0.31.0-source.tar.gz":source_digest,
    "control-center-0.31.0.qualification.json":qualification_digest,
    "control-center-0.31.0.provenance.json":provenance_digest
  }
}
with open(path,"w",encoding="utf-8") as f: json.dump(data,f,sort_keys=True,separators=(",",":")); f.write("\n")
PY

(
  cd "$out"
  sha256sum \
    "control-center-$candidate_version-linux-amd64.tar.gz" \
    "control-center-$candidate_version-linux-amd64.tar.gz.sha256" \
    "control-center-$candidate_version-source.tar.gz" \
    "control-center-$candidate_version.provenance.json" \
    "control-center-$candidate_version.qualification.json" \
    "control-center-$candidate_version.release-manifest.json" > SHA256SUMS
)

# Validate the exact seven-file candidate set through the repository's fail-closed
# releasecandidate contract without adding a second workflow/job.
validator_dir="$(mktemp -d "$repo_root/.candidate-validator.XXXXXX")"
trap 'rm -rf "$validator_dir"; cleanup' EXIT
python3 - "$out" "$validator_dir/evidence.json" "$candidate_sha" <<'PY'
import hashlib,json,os,sys
root,out,sha=sys.argv[1:]
names=[
 "control-center-0.31.0-linux-amd64.tar.gz",
 "control-center-0.31.0-linux-amd64.tar.gz.sha256",
 "control-center-0.31.0-source.tar.gz",
 "control-center-0.31.0.provenance.json",
 "control-center-0.31.0.qualification.json",
 "control-center-0.31.0.release-manifest.json",
 "SHA256SUMS",
]
artifacts=[]
for name in names:
    p=os.path.join(root,name)
    with open(p,"rb") as f: digest=hashlib.sha256(f.read()).hexdigest()
    artifacts.append({"name":name,"digest":"sha256:"+digest})
with open(out,"w",encoding="utf-8") as f:
    json.dump({"schema":"control-center.release-candidate-artifacts.v1","candidate_version":"0.31.0","candidate_sha":sha,"artifacts":artifacts},f)
PY
cat > "$validator_dir/main.go" <<'GO'
package main
import (
 "encoding/json"
 "fmt"
 "os"
 "control-center/internal/releasecandidate"
)
func main() {
 raw, err := os.ReadFile(os.Args[1]); if err != nil { panic(err) }
 var m releasecandidate.ArtifactManifest
 if err := json.Unmarshal(raw,&m); err != nil { panic(err) }
 if err := releasecandidate.ValidateArtifactManifest(m); err != nil { panic(err) }
 fmt.Println("CANDIDATE_ARTIFACT_SET=PASS")
}
GO
go run "$validator_dir/main.go" "$validator_dir/evidence.json"
rm -rf "$validator_dir"
trap cleanup EXIT

echo "CANDIDATE_PACKAGE=PASS"
echo "CLEAN_INSTALL=PASS"
echo "UPGRADE_FROM_STABLE_0_30=PASS"
echo "ROLLBACK_FORWARD_RECOVERY=PASS"
echo "CANDIDATE_SHA=$candidate_sha"
