#!/usr/bin/env python3
"""Publish a verified stable release to the fixed Homebrew tap.

Usage: python3 scripts/update_homebrew.py vMAJOR.MINOR.PATCH
Requires gh on PATH and GH_TOKEN with source read and tap contents write access.
No git checkout or credentials are used. Identical reruns perform no write.
"""
from __future__ import annotations

import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SOURCE = "trogers1/sidecar-comments"
TAP = "trogers1/homebrew-tap"
FORMULA = "Formula/sidecar.rb"
DOWNLOAD = f"https://github.com/{SOURCE}/releases/download"


def run(*, args: list[str], input_text: str | None = None) -> str:
    return subprocess.run(args, input=input_text, check=True, text=True,
                          stdout=subprocess.PIPE).stdout


def identity(*, tag: str) -> str:
    if not tag.startswith("v"):
        raise ValueError("stable tag must start with v")
    return subprocess.run(
        ["bash", str(ROOT / "scripts/release.sh"), "identity"],
        env={**os.environ, "RELEASE_REF": f"refs/tags/{tag}", "RELEASE_SHA": "0" * 40},
        check=True, text=True, stdout=subprocess.PIPE,
    ).stdout.strip()


def platforms() -> list[str]:
    # The packaging script owns the supported native platforms and asset naming.
    script = (ROOT / "scripts/release.sh").read_text()
    match = re.search(r"case \$actual in ([a-z0-9/|]+)\) ;;", script)
    if match is None or 'asset="sidecar_${RELEASE_PLATFORM//\\//_}.tar.gz"' not in script:
        raise ValueError("cannot derive release platforms/assets from release.sh")
    result = match[1].split("|")
    if set(result) != {"darwin/arm64", "linux/amd64"} or len(result) != 2:
        raise ValueError("release platforms changed; update Homebrew platform constraints")
    return result


def object_response(*, text: str) -> dict:
    value = json.loads(text)
    if not isinstance(value, dict):
        raise ValueError("GitHub API response must be an object")
    return value


def api(*, endpoint: str) -> dict:
    return object_response(text=run(args=["gh", "api", "--hostname", "github.com", endpoint]))


def existing_formula() -> tuple[str, str] | None:
    # --include lets us distinguish an actual HTTP 404 from auth/transport errors.
    result = subprocess.run(
        ["gh", "api", "--hostname", "github.com", "--include", f"repos/{TAP}/contents/{FORMULA}"],
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    response = result.stdout.replace("\r\n", "\n")
    headers, separator, body = response.partition("\n\n")
    status = re.match(r"HTTP/\S+ (\d{3})(?: |$)", headers)
    if not separator or status is None:
        raise ValueError(f"contents API missing HTTP status: {result.stderr.strip()}")
    data = object_response(text=body)
    if status[1] == "404" and result.returncode != 0 and data.get("message") == "Not Found":
        # A private/unavailable repo also returns 404. Confirm the tap is accessible.
        repo = api(endpoint=f"repos/{TAP}")
        if repo.get("full_name") != TAP or repo.get("permissions", {}).get("push") is not True:
            raise ValueError("tap is not accessible with contents write permission")
        return None
    if result.returncode or status[1] != "200":
        raise ValueError(f"contents API failed (HTTP {status[1]}): {result.stderr.strip()} {body}")
    if data.get("type") != "file" or data.get("encoding") != "base64" or data.get("path") != FORMULA:
        raise ValueError("unexpected contents API file response")
    sha, content = data.get("sha"), data.get("content")
    if not isinstance(sha, str) or not re.fullmatch(r"[0-9a-f]{40}", sha) or not isinstance(content, str):
        raise ValueError("contents API missing valid SHA/content")
    return sha, base64.b64decode("".join(content.split()), validate=True).decode("utf-8")


def previous(*, text: str, assets: list[str]) -> tuple[str, dict[str, str]]:
    versions = re.findall(r'^  version "([^"]+)"$', text, re.MULTILINE)
    if len(versions) != 1:
        raise ValueError("existing formula must have one explicit stable version")
    version = identity(tag="v" + versions[0])
    pairs = re.findall(r'url "([^"]+)"\s+sha256 "([0-9a-f]{64})"', text)
    hashes = dict(pairs)
    expected = {f"{DOWNLOAD}/v{version}/{asset}" for asset in assets}
    if len(pairs) != len(assets) or set(hashes) != expected:
        raise ValueError("existing formula has unexpected release URLs/checksums")
    return version, {asset: hashes[f"{DOWNLOAD}/v{version}/{asset}"] for asset in assets}


def render_formula(*, version: str, checksums: dict[str, str]) -> str:
    """Render locally without gh or network; version has no leading v.

    Callers must verify the archives themselves before supplying SHA-256 values.
    """
    identity(tag="v" + version)
    assets = {f"sidecar_{platform.replace('/', '_')}.tar.gz" for platform in platforms()}
    if set(checksums) != assets or any(not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{64}", value)
                                       for value in checksums.values()):
        raise ValueError("checksums must map each supported archive filename to a lowercase SHA-256")
    hashes = checksums
    return f'''class Sidecar < Formula
  desc "Code review comments alongside your source"
  homepage "https://github.com/{SOURCE}"
  version "{version}"
  license "MIT"

  on_macos do
    depends_on arch: :arm64
    url "{DOWNLOAD}/v{version}/sidecar_darwin_arm64.tar.gz"
    sha256 "{hashes['sidecar_darwin_arm64.tar.gz']}"
  end

  on_linux do
    depends_on arch: :x86_64
    url "{DOWNLOAD}/v{version}/sidecar_linux_amd64.tar.gz"
    sha256 "{hashes['sidecar_linux_amd64.tar.gz']}"
  end

  def install
    bin.install "sidecar"
    doc.install "README.md", "THIRD_PARTY_NOTICES", "build-info.txt"
  end

  test do
    require "json"
    result = JSON.parse(shell_output("#{{bin}}/sidecar --output json version"))
    assert_equal version.to_s, result.fetch("result").fetch("cli_version")
  end
end
'''


def main() -> None:
    if len(sys.argv) != 2:
        raise ValueError("usage: python3 scripts/update_homebrew.py vMAJOR.MINOR.PATCH")
    tag = sys.argv[1]
    version = identity(tag=tag)
    assets = [f"sidecar_{platform.replace('/', '_')}.tar.gz" for platform in platforms()]
    metadata = object_response(text=run(args=[
        "gh", "release", "view", tag, "--repo", f"https://github.com/{SOURCE}",
        "--json", "tagName,isDraft,isPrerelease,publishedAt",
    ]))
    if (metadata.get("tagName") != tag or metadata.get("isDraft") is not False
            or metadata.get("isPrerelease") is not False
            or not isinstance(metadata.get("publishedAt"), str) or not metadata["publishedAt"]):
        raise ValueError("release must be published, stable and match the requested tag")
    # Stable tags/assets are immutable by repository publication policy, not
    # GitHub's optional immutable-release flag. Hash replacement is guarded below.
    old = existing_formula()
    old_version, old_hashes = previous(text=old[1], assets=assets) if old else (None, None)
    if old_version and tuple(map(int, version.split('.'))) < tuple(map(int, old_version.split('.'))):
        raise ValueError("refusing formula downgrade")
    hashes: dict[str, str] = {}
    with tempfile.TemporaryDirectory(prefix="sidecar-homebrew-") as directory:
        args = ["gh", "release", "download", tag, "--repo", f"https://github.com/{SOURCE}", "--dir", directory]
        for asset in [*assets, "checksums.txt"]:
            args += ["--pattern", asset]
        run(args=args)
        # publish.sh combines the two native shasum manifests into checksums.txt.
        manifest = (Path(directory) / "checksums.txt").read_text(encoding="ascii")
        expected: dict[str, str] = {}
        for line in manifest.splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  ([a-z0-9_]+\.tar\.gz)", line)
            if match is None or match[2] not in assets or match[2] in expected:
                raise ValueError("invalid checksum manifest: malformed, unexpected or duplicate entry")
            expected[match[2]] = match[1]
        if set(expected) != set(assets):
            raise ValueError("invalid checksum manifest: missing archive entry")
        for asset in assets:
            digest = hashlib.sha256((Path(directory) / asset).read_bytes()).hexdigest()
            if digest != expected[asset]:
                raise ValueError(f"checksum mismatch for {asset}")
            hashes[asset] = digest
    if old_version == version and old_hashes != hashes:
        raise ValueError("refusing same-version hash replacement")
    text = render_formula(version=version, checksums=hashes)
    if old and old[1] == text:
        print(f"{TAP}/{FORMULA}: v{version} already current (no-op)")
        return
    payload = {"message": f"sidecar {tag}", "content": base64.b64encode(text.encode()).decode()}
    if old:
        payload["sha"] = old[0]
    run(args=["gh", "api", "--hostname", "github.com", "--method", "PUT",
              f"repos/{TAP}/contents/{FORMULA}", "--input", "-"], input_text=json.dumps(payload))
    print(f"Updated {TAP}/{FORMULA} to {tag}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(f"Homebrew update failed: {error}")
