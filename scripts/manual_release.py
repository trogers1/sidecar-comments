#!/usr/bin/env python3
"""Read-only manual planning and serialized, post-verification tag creation."""
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent


def run(*, args: list[str], env: dict[str, str] | None = None) -> str:
    return subprocess.run(args, env=env, check=True, text=True, stdout=subprocess.PIPE).stdout.strip()


def identity(*, ref: str, sha: str) -> str:
    return run(args=["bash", str(ROOT / "scripts/release.sh"), "identity"],
               env={**os.environ, "RELEASE_REF": ref, "RELEASE_SHA": sha})


def api(*, endpoint: str, fields: dict[str, str] | None = None, pages: bool = False) -> object:
    args = ["gh", "api"]
    if pages:
        args += ["--paginate", "--slurp"]
    if fields is not None:
        args += ["--method", "POST"]
    args += [f"repos/{os.environ['GH_REPO']}/{endpoint}"]
    for key, value in (fields or {}).items():
        args += ["-f", f"{key}={value}"]
    return json.loads(run(args=args))


def mapping(*, value: object) -> dict[str, object]:
    if not isinstance(value, dict):
        raise ValueError("API response must contain an object")
    result: dict[str, object] = {}
    for key, item in value.items():
        if not isinstance(key, str):
            raise ValueError("API object keys must be strings")
        result[key] = item
    return result


def array(*, value: object) -> list[object]:
    if not isinstance(value, list):
        raise ValueError("tag API must return arrays of refs")
    return list(value)


def ref_sha(*, response: object, expected_ref: str) -> str:
    entry = mapping(value=response)
    if entry.get("ref") != expected_ref:
        raise ValueError("ref API returned an unexpected ref")
    target = mapping(value=entry.get("object"))
    sha = target.get("sha")
    if not isinstance(sha, str):
        raise ValueError("ref API missing SHA")
    identity(ref=expected_ref, sha=sha)
    return sha


def tag_refs(*, response: object) -> set[str]:
    refs: set[str] = set()
    for page in array(value=response):
        for item in array(value=page):
            ref = mapping(value=item).get("ref")
            if not isinstance(ref, str) or not ref.startswith("refs/tags/") or ref == "refs/tags/":
                raise ValueError("tag API entry must contain a tag ref")
            refs.add(ref)
    return refs


def highest(*, sha: str) -> tuple[str, set[str]]:
    refs = tag_refs(response=api(endpoint="git/matching-refs/tags/", pages=True))
    stable: list[tuple[tuple[int, ...], str]] = []
    for ref in refs:
        if not ref.startswith("refs/tags/v"):
            continue
        parts = ref.removeprefix("refs/tags/v").split(".")
        # Filter known noncanonical tag names, not identity execution failures.
        if len(parts) != 3 or any(
            not part.isascii() or not part.isdecimal() or
            (len(part) > 1 and part.startswith("0")) for part in parts
        ):
            continue
        version = identity(ref=ref, sha=sha)
        stable.append((tuple(int(part) for part in version.split(".")), ref))
    return max(stable)[1] if stable else "refs/tags/v0.0.0", refs


def candidate(*, base: str, bump: str, sha: str) -> str:
    version = identity(ref=base, sha=sha)
    if bump not in ("major", "minor", "patch"):
        raise ValueError("bump must be major, minor or patch")
    parts = [int(part) for part in version.split(".")]
    index = ("major", "minor", "patch").index(bump)
    parts[index] += 1
    parts[index + 1:] = [0] * (2 - index)
    ref = "refs/tags/v" + ".".join(str(part) for part in parts)
    identity(ref=ref, sha=sha)
    return ref


def main() -> None:
    mode = sys.argv[1] if len(sys.argv) == 2 else ""
    if mode not in ("plan", "create"):
        raise ValueError("usage: manual_release.py plan|create")
    if os.environ.get("GITHUB_EVENT_NAME") != "workflow_dispatch" or os.environ.get("GITHUB_REF") != "refs/heads/main":
        raise ValueError("manual release requires workflow_dispatch on main")
    bump = os.environ["RELEASE_BUMP"]
    if bump not in ("major", "minor", "patch"):
        raise ValueError("bump must be major, minor or patch")
    if mode == "plan":
        sha = ref_sha(response=api(endpoint="git/ref/heads/main"), expected_ref="refs/heads/main")
        base, refs = highest(sha=sha)
        ref = candidate(base=base, bump=bump, sha=sha)
        if ref in refs:
            raise ValueError("candidate tag already exists")
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write(f"sha={sha}\nref={ref}\nbase={base}\n")
    else:
        sha = os.environ["RELEASE_SHA"]
        ref = os.environ["RELEASE_REF"]
        identity(ref=ref, sha=sha)
        base, refs = highest(sha=sha)
        if base != os.environ["RELEASE_BASE"]:
            raise ValueError("stable release base changed; dispatch again")
        if ref != candidate(base=base, bump=bump, sha=sha) or ref in refs:
            raise ValueError("candidate changed or already exists")
        # Atomic create-only API: a competing creator fails, never retargets.
        created_sha = ref_sha(response=api(endpoint="git/refs", fields={"ref": ref, "sha": sha}), expected_ref=ref)
        if created_sha != sha:
            raise ValueError("created tag does not match selected SHA")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        sys.exit(f"manual release failed: {error}")
