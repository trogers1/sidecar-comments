#!/usr/bin/env python3
"""Committed VERSION release planning, create-only tagging, and local push gate."""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", re.ASCII)
SHA = re.compile(r"[0-9a-f]{40}")


def run(*, args: list[str]) -> str:
    return subprocess.run(args, cwd=ROOT, check=True, text=True, stdout=subprocess.PIPE).stdout.removesuffix("\n")


def version(*, text: str) -> tuple[int, ...]:
    if VERSION.fullmatch(text) is None:
        raise ValueError("VERSION must be canonical MAJOR.MINOR.PATCH")
    return tuple(int(part) for part in text.split("."))


def committed_version(*, sha: str) -> str:
    if SHA.fullmatch(sha) is None:
        raise ValueError("expected a full lowercase Git SHA")
    text = run(args=["git", "show", f"{sha}:VERSION"])
    version(text=text)
    return text


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


def tag_refs(*, response: object) -> set[str]:
    refs: set[str] = set()
    for page in array(value=response):
        for item in array(value=page):
            ref = mapping(value=item).get("ref")
            if not isinstance(ref, str) or not ref.startswith("refs/tags/") or ref == "refs/tags/":
                raise ValueError("tag API entry must contain a tag ref")
            refs.add(ref)
    return refs


def highest(*, refs: set[str]) -> str:
    stable = [(version(text=ref.removeprefix("refs/tags/v")), ref)
              for ref in refs if ref.startswith("refs/tags/v")
              and VERSION.fullmatch(ref.removeprefix("refs/tags/v"))]
    return max(stable)[1] if stable else "refs/tags/v0.0.0"


def newer(*, candidate: str, base: str) -> None:
    if version(text=candidate) <= version(text=base.removeprefix("refs/tags/v")):
        raise ValueError(f"VERSION {candidate} must be newer than {base}")


def ci(*, mode: str) -> None:
    event = os.environ["GITHUB_EVENT_NAME"]
    ref = os.environ["GITHUB_REF"]
    push = event == "push" and ref == "refs/heads/main"
    pr = event == "pull_request" and re.fullmatch(r"refs/pull/[1-9][0-9]*/merge", ref)
    if not push and not (mode == "plan" and pr):
        raise ValueError("release requires push on main (or pull_request merge for plan only)")
    sha = os.environ["GITHUB_SHA"]
    if run(args=["git", "rev-parse", "HEAD"]) != sha:
        raise ValueError("checkout HEAD must match GITHUB_SHA")
    candidate = committed_version(sha=sha)
    candidate_ref = "refs/tags/v" + candidate
    if mode == "create":
        if os.environ["RELEASE_SHA"] != sha or os.environ["RELEASE_REF"] != candidate_ref:
            raise ValueError("release identity does not match checked out commit VERSION")
    refs = tag_refs(response=api(endpoint="git/matching-refs/tags/", pages=True))
    base = highest(refs=refs)
    if mode == "create" and base != os.environ["RELEASE_BASE"]:
        raise ValueError("stable release base changed; plan again")
    if candidate_ref in refs:
        raise ValueError("candidate tag already exists")
    newer(candidate=candidate, base=base)
    if mode == "plan":
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write(f"sha={sha}\nref={candidate_ref}\nbase={base}\n")
    else:
        # Create-only API: a competing creator fails; never retarget a tag.
        response = mapping(value=api(endpoint="git/refs", fields={"ref": candidate_ref, "sha": sha}))
        if response.get("ref") != candidate_ref or mapping(value=response.get("object")).get("sha") != sha:
            raise ValueError("created tag does not match selected release identity")


def pre_push(*, remote: str) -> None:
    commits: list[str] = []
    for line in sys.stdin:
        fields = line.split()
        if len(fields) != 4:
            raise ValueError("expected four fields in pre-push update")
        _, sha, destination, _ = fields
        if destination != "refs/heads/main":
            continue
        if sha == "0" * 40:
            raise ValueError("deleting main is forbidden")
        commits.append(committed_version(sha=sha))
    if not commits:
        return
    refs: set[str] = set()
    for line in run(args=["git", "ls-remote", "--tags", "--", remote]).splitlines():
        fields = line.split()
        if len(fields) != 2 or SHA.fullmatch(fields[0]) is None or not fields[1].startswith("refs/tags/"):
            raise ValueError("malformed ls-remote tag response")
        refs.add(fields[1])
    base = highest(refs=refs)
    for candidate in commits:
        newer(candidate=candidate, base=base)


def main() -> None:
    args = sys.argv[1:]
    if args in (["plan"], ["create"]):
        ci(mode=args[0])
    elif len(args) == 3 and args[0] == "pre-push":
        pre_push(remote=args[2])
    elif len(args) == 2 and args[0] == "bump" and args[1] in ("major", "minor", "patch"):
        parts = list(version(text=(ROOT / "VERSION").read_text().removesuffix("\n")))
        index = ("major", "minor", "patch").index(args[1])
        parts[index] += 1
        parts[index + 1:] = [0] * (2 - index)
        text = ".".join(str(part) for part in parts)
        (ROOT / "VERSION").write_text(text + "\n")
        print(text)
    else:
        raise ValueError("usage: version_release.py plan|create|bump patch|minor|major|pre-push REMOTE LOCATION")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(f"version release failed: {error}")
