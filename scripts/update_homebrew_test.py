#!/usr/bin/env python3
"""Offline subprocess contract tests; run python3 scripts/update_homebrew_test.py."""
from __future__ import annotations

from functools import partial, update_wrapper
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
ENTRY = ROOT / "scripts/update_homebrew.py"
# Independently read the CI platform matrix, not the generator's implementation.
PLATFORMS = re.findall(r"platform: ([a-z0-9]+/[a-z0-9]+)", (ROOT / ".github/workflows/release.yml").read_text())
ASSETS = [f"sidecar_{platform.replace('/', '_')}.tar.gz" for platform in PLATFORMS]

FAKE = r'''#!/usr/bin/env python3
import base64, json, os, pathlib, shutil, sys
state_path = pathlib.Path(os.environ['FAKE_STATE'])
s = json.loads(state_path.read_text())
a = sys.argv[1:]
assert os.environ['GH_TOKEN'] == 'offline-test-token'
s['calls'].append(a)
state_path.write_text(json.dumps(s))
if a[:2] == ['release', 'view']:
    r = s['release']
    print(json.dumps({'tagName': r['tag_name'], 'isDraft': r['draft'],
                      'isPrerelease': r['prerelease'], 'publishedAt': r.get('published_at', '2026-01-01T00:00:00Z')}))
    sys.exit(0)
if a[:2] == ['release', 'download']:
    assert a[a.index('--repo')+1] == 'https://github.com/trogers1/sidecar-comments'
    dest = pathlib.Path(a[a.index('--dir')+1])
    for i, arg in enumerate(a):
        if arg == '--pattern':
            name = a[i+1]
            src = pathlib.Path(os.environ['FAKE_ASSETS']) / name
            if not src.exists():
                sys.exit('asset missing')
            shutil.copyfile(src, dest / name)
    sys.exit(0)
assert a[0] == 'api' and a[a.index('--hostname')+1] == 'github.com'
endpoint = next(x for x in a if x.startswith('repos/'))
if endpoint == 'repos/trogers1/homebrew-tap':
    print(json.dumps({'full_name': 'trogers1/homebrew-tap', 'permissions': {'push': True}}))
else:
    assert endpoint == 'repos/trogers1/homebrew-tap/contents/Formula/sidecar.rb'
    if '--method' in a:
        assert a[a.index('--method')+1] == 'PUT' and a[-2:] == ['--input', '-']
        payload = json.load(sys.stdin)
        if s.get('conflict') or payload.get('sha') != s.get('sha'):
            sys.exit('HTTP 409 optimistic SHA conflict')
        assert payload['message'].startswith('sidecar v')
        s['formula'] = base64.b64decode(payload['content'], validate=True).decode()
        s['sha'] = 'a' * 40
        s['writes'] += 1
        state_path.write_text(json.dumps(s))
        print(json.dumps({'content': {'sha': s['sha']}}))
    else:
        assert '--include' in a
        status = s.get('status', 200 if s.get('formula') else 404)
        print('HTTP/2.0 ' + str(status) + ' status\r\nContent-Type: application/json\r\n\r\n', end='')
        if status != 200:
            print(json.dumps({'message': 'Not Found' if status == 404 else 'API error'}))
            sys.exit(1)
        print(json.dumps({'type': 'file', 'path': 'Formula/sidecar.rb', 'encoding': 'base64',
                          'sha': s['sha'], 'content': base64.b64encode(s['formula'].encode()).decode()}))
'''


def save(*, directory: Path, state: dict) -> None:
    (directory / "state.json").write_text(json.dumps(state))


def load(*, directory: Path) -> dict:
    return json.loads((directory / "state.json").read_text())


def invoke(*, directory: Path, tag: str = "v0.1.0") -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(ENTRY), tag], cwd=directory,
                          env={**os.environ, "PATH": str(directory) + os.pathsep + os.environ['PATH'],
                               "GH_TOKEN": "offline-test-token", "FAKE_STATE": str(directory / "state.json"),
                               "FAKE_ASSETS": str(directory / "assets"), "GH_REPO": "wrong/ignored"},
                          capture_output=True, text=True)


def refresh_checksums(*, directory: Path) -> None:
    assets = directory / 'assets'
    (assets / 'checksums.txt').write_text(''.join(
        f'{hashlib.sha256((assets / asset).read_bytes()).hexdigest()}  {asset}\n'
        for asset in ASSETS))


def fixture(*, directory: Path) -> None:
    executable = directory / "gh"
    executable.write_text(FAKE)
    executable.chmod(0o755)
    assets = directory / "assets"
    assets.mkdir()
    for asset in ASSETS:
        with tarfile.open(assets / asset, "w:gz") as archive:
            data = b'#!/bin/sh\necho offline fixture\n'
            info = tarfile.TarInfo("sidecar")
            info.size, info.mode = len(data), 0o755
            archive.addfile(info, io.BytesIO(data))
            for name in ('README.md', 'LICENSE', 'THIRD_PARTY_NOTICES', 'build-info.txt'):
                data = f'offline fixture {name}\n'.encode()
                info = tarfile.TarInfo(name)
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
    refresh_checksums(directory=directory)
    # This is the public v0.1.0 shape: no per-archive checksum assets and no
    # requirement for GitHub's optional immutable-release feature.
    save(directory=directory, state={"release": {"tag_name": "v0.1.0", "draft": False,
         "prerelease": False, "immutable": False}, "calls": [], "writes": 0})


def success(*, result: subprocess.CompletedProcess) -> None:
    assert result.returncode == 0, result.stderr


def failure(*, result: subprocess.CompletedProcess, message: str) -> None:
    assert result.returncode != 0, result.stdout
    assert message in result.stderr, result.stderr


def scenario(*, case: str) -> None:
    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary)
        fixture(directory=directory)
        state = load(directory=directory)
        if case == "create-rerun-upgrade":
            success(result=invoke(directory=directory))
            state = load(directory=directory)
            text = state['formula']
            assert 'license "MIT"' in text and 'version "0.1.0"' in text
            assert 'on_macos do\n    depends_on arch: :arm64' in text
            assert 'on_linux do\n    depends_on arch: :x86_64' in text
            assert 'bin.install "sidecar"' in text
            assert 'doc.install "README.md", "THIRD_PARTY_NOTICES", "build-info.txt"' in text
            assert {path.name for path in (directory / 'assets').iterdir()} == {*ASSETS, 'checksums.txt'}
            assert '--output json version' in text and 'fetch("cli_version")' in text
            for asset in ASSETS:
                assert f'https://github.com/trogers1/sidecar-comments/releases/download/v0.1.0/{asset}' in text
                assert hashlib.sha256((directory / 'assets' / asset).read_bytes()).hexdigest() in text
            result = invoke(directory=directory)
            success(result=result)
            assert 'no-op' in result.stdout and load(directory=directory)['writes'] == 1
            state = load(directory=directory)
            state['release']['tag_name'] = 'v0.2.0'
            save(directory=directory, state=state)
            success(result=invoke(directory=directory, tag='v0.2.0'))
            assert load(directory=directory)['writes'] == 2
            return
        if case in ('downgrade', 'same-version-change', 'conflict', 'malformed-formula'):
            success(result=invoke(directory=directory))
            state = load(directory=directory)
            if case == 'downgrade':
                state['formula'] = state['formula'].replace('0.1.0', '0.2.0')
                message = 'downgrade'
            elif case == 'same-version-change':
                asset = ASSETS[0]
                (directory / 'assets' / asset).write_bytes(b'replaced archive')
                refresh_checksums(directory=directory)
                message = 'same-version hash replacement'
            elif case == 'conflict':
                state['release']['tag_name'] = 'v0.2.0'
                state['conflict'] = True
                message = '409'
            else:
                state['formula'] = 'unexpected file'
                message = 'existing formula'
        elif case in ('draft', 'prerelease', 'wrong-tag', 'unpublished'):
            if case == 'unpublished':
                state['release']['published_at'] = None
            elif case == 'wrong-tag':
                state['release']['tag_name'] = 'v9.0.0'
            else:
                state['release'][case] = True
            message = 'published, stable'
        elif case in ('403', '500'):
            state['status'] = int(case)
            message = 'contents API failed'
        elif case in ('corrupt', 'manifest', 'duplicate-entry', 'missing-entry', 'extra-entry',
                      'missing-asset', 'missing-manifest'):
            asset = directory / 'assets' / ASSETS[0]
            manifest = directory / 'assets' / 'checksums.txt'
            if case == 'corrupt':
                asset.write_bytes(b'corrupt')
                message = 'checksum mismatch'
            elif case in ('manifest', 'duplicate-entry', 'missing-entry', 'extra-entry'):
                lines = manifest.read_text().splitlines(keepends=True)
                if case == 'manifest':
                    lines[0] = '0' * 64 + '  ../unexpected\n'
                elif case == 'duplicate-entry':
                    lines.append(lines[0])
                elif case == 'missing-entry':
                    lines.pop()
                else:
                    lines.append('0' * 64 + '  unexpected.tar.gz\n')
                manifest.write_text(''.join(lines))
                message = 'invalid checksum manifest'
            else:
                (asset if case == 'missing-asset' else manifest).unlink()
                message = 'asset missing'
        else:
            raise AssertionError(case)
        writes = state['writes']
        save(directory=directory, state=state)
        failure(result=invoke(directory=directory, tag='v0.2.0' if case == 'conflict' else 'v0.1.0'), message=message)
        assert load(directory=directory)['writes'] == writes


def invalid_tags() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary)
        fixture(directory=directory)
        for tag in ('main', '0.1.0', 'v01.1.0', 'v1.0', 'v1.0.0-rc.1', 'v1.0.0+build', 'v1.0.0/evil'):
            result = invoke(directory=directory, tag=tag)
            assert result.returncode != 0
        assert not load(directory=directory)['calls']


def local_render() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary)
        fixture(directory=directory)
        checksums = {asset: hashlib.sha256((directory / 'assets' / asset).read_bytes()).hexdigest()
                     for asset in ASSETS}
        # The exported renderer is a public entry point too. A gh fake is on PATH,
        # but any invocation would be recorded and fail (no fake state env).
        code = ('import json, runpy, sys; '
                'render = runpy.run_path(sys.argv[1])["render_formula"]; '
                'print(render(version="0.1.0", checksums=json.loads(sys.argv[2])))')
        result = subprocess.run([sys.executable, '-c', code, str(ENTRY), json.dumps(checksums)],
                                cwd=directory, capture_output=True, text=True,
                                env={**os.environ, 'PATH': str(directory) + os.pathsep + os.environ['PATH']})
        success(result=result)
        assert 'class Sidecar < Formula' in result.stdout
        assert 'version "0.1.0"' in result.stdout
        assert not load(directory=directory)['calls']


def main() -> None:
    cases = ('create-rerun-upgrade', 'downgrade', 'same-version-change', 'conflict',
             'malformed-formula', 'draft', 'prerelease', 'wrong-tag', 'unpublished',
             '403', '500', 'corrupt', 'manifest', 'duplicate-entry', 'missing-entry',
             'extra-entry', 'missing-asset', 'missing-manifest')
    suite = unittest.TestSuite([unittest.FunctionTestCase(update_wrapper(partial(scenario, case=case), scenario), description=case) for case in cases])
    suite.addTest(unittest.FunctionTestCase(invalid_tags))
    suite.addTest(unittest.FunctionTestCase(local_render))
    if not unittest.TextTestRunner(verbosity=2).run(suite).wasSuccessful():
        sys.exit(1)


if __name__ == '__main__':
    main()
