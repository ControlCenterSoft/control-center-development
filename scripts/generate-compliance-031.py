#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
import subprocess
import sys
import urllib.parse
import uuid
from pathlib import Path

ALLOW = {
    "MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC",
    "PostgreSQL", "Zlib", "Unicode-3.0",
}
REVIEW = {"MPL-2.0", "EPL-2.0", "LGPL-2.1-only", "LGPL-3.0-only"}
BLOCK_PREFIXES = ("GPL-", "AGPL-", "SSPL-", "BUSL-", "BSL-", "Elastic-")

def run(cmd):
    return subprocess.check_output(cmd, text=True, stderr=subprocess.STDOUT)

def json_stream(cmd):
    raw = run(cmd)
    dec = json.JSONDecoder()
    out = []
    i = 0
    n = len(raw)
    while i < n:
        while i < n and raw[i].isspace():
            i += 1
        if i >= n:
            break
        obj, i = dec.raw_decode(raw, i)
        out.append(obj)
    return out

def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

def detect_license(text):
    t = text.lower()
    if "apache license" in t and "version 2.0" in t:
        return "Apache-2.0"
    if "mozilla public license" in t and "2.0" in t:
        return "MPL-2.0"
    if "eclipse public license" in t and "2.0" in t:
        return "EPL-2.0"
    if "gnu affero general public license" in t:
        return "AGPL-3.0-only"
    if "gnu lesser general public license" in t:
        return "LGPL-3.0-only"
    if "gnu general public license" in t:
        return "GPL-3.0-only"
    if "permission is hereby granted, free of charge, to any person obtaining a copy" in t:
        return "MIT"
    if "permission to use, copy, modify, and/or distribute this software for any purpose with or without fee" in t:
        return "ISC"
    if "redistribution and use in source and binary forms" in t:
        if "neither the name" in t:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if "postgresql database management system" in t and "permission to use, copy, modify, and distribute" in t:
        return "PostgreSQL"
    if "this software is provided 'as-is', without any express or implied warranty" in t and "altered source versions must be plainly marked" in t:
        return "Zlib"
    if "unicode, inc. license agreement" in t or ("unicode" in t and "copyright and permission notice" in t):
        return "Unicode-3.0"
    return "UNKNOWN"

def module_ref(path, version):
    quoted = urllib.parse.quote(path, safe="/.-_~")
    v = urllib.parse.quote(version, safe=".-_~+")
    return f"pkg:golang/{quoted}@{v}"

def find_evidence_files(module_dir):
    files = []
    for p in sorted(Path(module_dir).iterdir(), key=lambda x: x.name.lower()):
        if not p.is_file():
            continue
        if re.match(r"(?i)^(license|licence|copying|notice)([._-].*)?$", p.name):
            files.append(p)
    return files

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--candidate-sha", required=True)
    ap.add_argument("--binary", required=True)
    ap.add_argument("--output-dir", required=True)
    args = ap.parse_args()

    if not re.fullmatch(r"[0-9a-f]{40}", args.candidate_sha):
        raise SystemExit("candidate SHA must be exact 40-char lowercase hex")
    outdir = Path(args.output_dir)
    outdir.mkdir(parents=True, exist_ok=True)
    binary = Path(args.binary)
    if not binary.is_file():
        raise SystemExit(f"binary missing: {binary}")

    subprocess.check_call(["go", "mod", "verify"])
    if re.search(r"(?m)^\s*replace\s+", Path("go.mod").read_text(encoding="utf-8")):
        raise SystemExit("replace directives require explicit commercial/security disposition")

    downloads = json_stream(["go", "mod", "download", "-json", "all"])
    packages = json_stream(["go", "list", "-deps", "-json", "./cmd/control-center"])
    runtime_modules = {
        p.get("Module", {}).get("Path")
        for p in packages
        if p.get("Module") and p["Module"].get("Path") and p["Module"].get("Path") != "control-center"
    }

    components = []
    by_key = {}
    notices = []
    review_count = block_count = unknown_count = 0

    for mod in sorted(downloads, key=lambda m: (m.get("Path", ""), m.get("Version", ""))):
        path = mod.get("Path")
        version = mod.get("Version")
        directory = mod.get("Dir")
        zip_path = mod.get("Zip")
        if not path or not version or not directory or not zip_path:
            raise SystemExit(f"incomplete module evidence: {path!r} {version!r}")
        ev_files = find_evidence_files(directory)
        license_files = [p for p in ev_files if re.match(r"(?i)^(license|licence|copying)", p.name)]
        if not license_files:
            raise SystemExit(f"no license file for {path}@{version}")

        detected = {detect_license(p.read_text(encoding="utf-8", errors="replace")) for p in license_files}
        detected.discard("UNKNOWN") if len(detected) > 1 else None
        if len(detected) != 1:
            spdx = "UNKNOWN"
        else:
            spdx = next(iter(detected))

        if spdx == "UNKNOWN":
            unknown_count += 1
            policy = "UNKNOWN"
        elif spdx in ALLOW:
            policy = "ALLOW"
        elif spdx in REVIEW:
            review_count += 1
            policy = "REVIEW"
        elif spdx.startswith(BLOCK_PREFIXES):
            block_count += 1
            policy = "BLOCK"
        else:
            review_count += 1
            policy = "REVIEW"

        scope = "runtime" if path in runtime_modules else "build_graph_not_linked"
        evidence = []
        copyright_lines = []
        for p in ev_files:
            txt = p.read_text(encoding="utf-8", errors="replace")
            evidence.append({"name": p.name, "sha256": sha256_file(p)})
            for line in txt.splitlines():
                if "copyright" in line.lower() and line.strip() not in copyright_lines:
                    copyright_lines.append(line.strip())
            notices.append(
                f"===== {path} {version} | {spdx} | {scope} | {p.name} =====\n"
                + txt.rstrip() + "\n"
            )

        ref = module_ref(path, version)
        comp = {
            "module": path,
            "version": version,
            "purl": ref,
            "scope": scope,
            "spdx": spdx,
            "policy": policy,
            "sum": mod.get("Sum", ""),
            "go_mod_sum": mod.get("GoModSum", ""),
            "module_zip_sha256": sha256_file(zip_path),
            "license_evidence": evidence,
            "copyright": copyright_lines,
            "modified_or_forked": False,
            "source": (mod.get("Origin") or {}).get("URL") or path,
        }
        components.append(comp)
        by_key[f"{path}@{version}"] = ref

    if not components:
        raise SystemExit("empty module graph")
    if review_count or block_count or unknown_count:
        raise SystemExit(
            f"dependency license policy not clear: review={review_count} block={block_count} unknown={unknown_count}"
        )

    inventory = {
        "schema": "control-center.oss-license-inventory.v1",
        "candidate_version": "0.31.0",
        "candidate_sha": args.candidate_sha,
        "policy_baseline": "OSS И THIRD-PARTY LICENSE POLICY v0.1",
        "status": "PASS_ALLOW_ONLY",
        "legal_clearance_claimed": False,
        "components": components,
    }
    (outdir / "license-inventory.json").write_text(
        json.dumps(inventory, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

    notices_header = (
        "CONTROL CENTER 0.31.0 — THIRD-PARTY NOTICES\n"
        f"Candidate SHA: {args.candidate_sha}\n"
        "Generated from exact Go module license/NOTICE files. "
        "This engineering evidence does not replace qualified legal review.\n\n"
    )
    (outdir / "THIRD_PARTY_NOTICES.txt").write_text(
        notices_header + "\n".join(notices),
        encoding="utf-8",
    )

    root_ref = f"pkg:generic/control-center@0.31.0?commit={args.candidate_sha}"
    cdx_components = []
    for c in components:
        cdx_components.append({
            "type": "library",
            "bom-ref": c["purl"],
            "name": c["module"],
            "version": c["version"],
            "scope": "required" if c["scope"] == "runtime" else "excluded",
            "purl": c["purl"],
            "hashes": [{"alg": "SHA-256", "content": c["module_zip_sha256"]}],
            "licenses": [{"license": {"id": c["spdx"]}}],
            "properties": [
                {"name": "control-center:dependency-scope", "value": c["scope"]},
                {"name": "control-center:license-policy", "value": c["policy"]},
                {"name": "control-center:modified-or-forked", "value": "false"},
            ],
        })

    deps = {root_ref: set()}
    graph = run(["go", "mod", "graph"]).splitlines()
    for line in graph:
        parts = line.split()
        if len(parts) != 2:
            continue
        parent, child = parts
        child_ref = by_key.get(child)
        if child_ref is None:
            continue
        if parent == "control-center":
            parent_ref = root_ref
        else:
            parent_ref = by_key.get(parent)
        if parent_ref is None:
            continue
        deps.setdefault(parent_ref, set()).add(child_ref)
        deps.setdefault(child_ref, set())
    for c in components:
        if c["scope"] == "runtime":
            deps.setdefault(root_ref, set()).add(c["purl"])

    serial = uuid.uuid5(uuid.NAMESPACE_URL, f"control-center:0.31.0:{args.candidate_sha}")
    bom = {
        "bomFormat": "CycloneDX",
        "specVersion": "1.7",
        "serialNumber": f"urn:uuid:{serial}",
        "version": 1,
        "metadata": {
            "component": {
                "type": "application",
                "bom-ref": root_ref,
                "name": "control-center",
                "version": "0.31.0",
                "hashes": [{"alg": "SHA-256", "content": sha256_file(binary)}],
                "properties": [{"name": "control-center:candidate-sha", "value": args.candidate_sha}],
            }
        },
        "components": cdx_components,
        "dependencies": [
            {"ref": ref, "dependsOn": sorted(children)}
            for ref, children in sorted(deps.items())
        ],
    }
    (outdir / "sbom.cdx.json").write_text(
        json.dumps(bom, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

    summary = {
        "schema": "control-center.commercial-oss-engineering-evidence.v1",
        "candidate_version": "0.31.0",
        "candidate_sha": args.candidate_sha,
        "status": "ENGINEERING_PASS_ALLOW_ONLY",
        "commercial_legal_clearance_claimed": False,
        "component_count": len(components),
        "runtime_component_count": sum(c["scope"] == "runtime" for c in components),
        "review_count": review_count,
        "block_count": block_count,
        "unknown_count": unknown_count,
        "license_inventory_sha256": sha256_file(outdir / "license-inventory.json"),
        "third_party_notices_sha256": sha256_file(outdir / "THIRD_PARTY_NOTICES.txt"),
        "cyclonedx_sbom_sha256": sha256_file(outdir / "sbom.cdx.json"),
    }
    (outdir / "commercial-oss-evidence.json").write_text(
        json.dumps(summary, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

if __name__ == "__main__":
    main()
