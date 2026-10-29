#!/usr/bin/env python3
"""Offline process-boundary tests, using a JSON-speaking gh fake."""
from __future__ import annotations

import hashlib
import json
import os
import shutil
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SHA = "0123456789abcdef0123456789abcdef01234567"


class ManualReleaseTest(unittest.TestCase):
    def test_boundaries(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            tools = work / "tools"
            tools.mkdir()
            gh = tools / "gh"
            gh.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ['TRACE'], 'a') as trace:
    trace.write(json.dumps(args) + '\\n')
config = json.loads(Path(os.environ['CONFIG']).read_text())
if config.get('fail') and config['fail'] in ' '.join(args):
    sys.exit('simulated API failure')
if args[0] == 'release':
    print('{}')
elif '--method' in args:
    print(json.dumps(config.get('created_response', {'ref': os.environ['RELEASE_REF'], 'object': {'sha': os.environ['RELEASE_SHA']}})))
elif 'repos/example/repo/git/ref/heads/main' in args:
    print(json.dumps(config.get('main_response', {'ref': 'refs/heads/main', 'object': {'sha': config.get('sha', os.environ['RELEASE_SHA'])}})))
elif 'repos/example/repo/git/matching-refs/tags/' in args:
    pages = [[{'ref': 'refs/tags/' + tag, 'object': {'type': 'commit', 'sha': os.environ['RELEASE_SHA']}} for tag in page] for page in config.get('tag_pages', [config.get('tags', [])])]
    print(json.dumps(config.get('tags_response', pages)))
elif 'repos/example/repo/releases' in args:
    print(config.get('release', ''))
elif any('/commits/' in arg for arg in args):
    print(os.environ['RELEASE_SHA'])
else:
    sys.exit('unexpected invocation: ' + repr(args))
''')
            gh.chmod(0o755)
            bash = tools / "bash"
            bash.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
config = json.loads(Path(os.environ['CONFIG']).read_text())
if sys.argv[-1] == 'identity' and config.get('fail_identity_ref') == os.environ.get('RELEASE_REF'):
    sys.exit('simulated identity execution failure')
os.execv(os.environ['REAL_BASH'], [os.environ['REAL_BASH'], *sys.argv[1:]])
''')
            bash.chmod(0o755)
            real_bash = shutil.which("bash")
            if real_bash is None:
                raise RuntimeError("bash is required for release-script tests")
            assets = work / "assets"
            assets.mkdir()
            for name in ("sidecar_darwin_arm64.tar.gz", "sidecar_linux_amd64.tar.gz"):
                (assets / name).write_bytes(b"verified fixture bytes")
                (assets / (name + ".sha256")).write_text(
                    f"{hashlib.sha256((assets / name).read_bytes()).hexdigest()}  {name}\n")
            env = {**os.environ, "REAL_BASH": real_bash, "PATH": f"{tools}:{os.environ['PATH']}",
                   "TRACE": str(work / "trace"), "CONFIG": str(work / "config"),
                   "GITHUB_OUTPUT": str(work / "output"), "GH_REPO": "example/repo",
                   "GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_REF": "refs/heads/main",
                   "RELEASE_SHA": SHA, "RELEASE_REF": "refs/tags/v0.0.1",
                   "RELEASE_BASE": "refs/tags/v0.0.0", "RELEASE_BUMP": "patch",
                   "RELEASE_MODE": "manual", "RELEASE_OUT": str(assets)}

            def invoke(*, mode: str, config: dict[str, object], overrides: dict[str, str] | None = None,
                       success: bool = True) -> list[list[str]]:
                (work / "config").write_text(json.dumps(config))
                (work / "trace").write_text("")
                (work / "output").write_text("")
                command = (["bash", "scripts/publish.sh"] if mode == "publish" else
                           ["python3", "scripts/manual_release.py", mode])
                result = subprocess.run(command, cwd=ROOT, env={**env, **(overrides or {})},
                                        text=True, capture_output=True, timeout=15)
                self.assertEqual(result.returncode == 0, success, result.stderr)
                return [json.loads(line) for line in (work / "trace").read_text().splitlines()]

            for bump, expected in (("patch", "0.0.1"), ("minor", "0.1.0"), ("major", "1.0.0")):
                invoke(mode="plan", config={}, overrides={"RELEASE_BUMP": bump})
                self.assertIn(f"ref=refs/tags/v{expected}\n", (work / "output").read_text())
            for tags, expected in ((["v9.99.9", "v10.0.0", "latest", "v010.9.9", "v11.0.0-rc.1"], "10.0.1"),
                                   (["v999999999999999999999999999.2.3"], "999999999999999999999999999.2.4")):
                invoke(mode="plan", config={"tags": tags})
                self.assertIn(f"ref=refs/tags/v{expected}\n", (work / "output").read_text())
            for bump, expected in (("minor", "12.35.0"), ("major", "13.0.0")):
                invoke(mode="plan", config={"tags": ["v12.34.56"]}, overrides={"RELEASE_BUMP": bump})
                self.assertIn(f"ref=refs/tags/v{expected}\n", (work / "output").read_text())
            invoke(mode="plan", config={"tag_pages": [["v9.99.9"], ["v11.2.3"], ["v10.99.99"]]})
            self.assertIn("ref=refs/tags/v11.2.4\n", (work / "output").read_text())
            main_ref = "refs/heads/main"
            for response in (None, [], {}, {"ref": main_ref}, {"ref": main_ref, "object": []},
                             {"ref": main_ref, "object": {}}, {"ref": main_ref, "object": {"sha": 123}},
                             {"ref": main_ref, "object": {"sha": "bad"}},
                             {"ref": "refs/heads/topic", "object": {"sha": SHA}}):
                invoke(mode="plan", config={"main_response": response}, success=False)
                self.assertEqual((work / "output").read_text(), "")
            for response in (None, {}, [None], [{}], [[None]], [[{}]], [[{"ref": 123}]],
                             [[{"ref": "refs/heads/main"}]], [[{"ref": "refs/tags/"}]]):
                invoke(mode="plan", config={"tags_response": response}, success=False)
                self.assertEqual((work / "output").read_text(), "")
            invoke(mode="plan", config={"tags": ["v10.0.0"], "fail_identity_ref": "refs/tags/v10.0.0"}, success=False)
            self.assertEqual((work / "output").read_text(), "")
            for overrides in ({"RELEASE_BUMP": "oops"}, {"GITHUB_REF": "refs/heads/topic"},
                              {"GITHUB_EVENT_NAME": "push"}):
                self.assertEqual(invoke(mode="plan", config={}, overrides=overrides, success=False), [])
            invoke(mode="plan", config={"sha": "bad"}, success=False)
            invoke(mode="plan", config={"fail": "matching-refs"}, success=False)
            for config in ({"tags": ["v0.0.1"]}, {"tags": ["v1.0.0"]},
                           {"release": "v0.0.1"}, {"fail": "matching-refs"}, {"fail": "releases"}):
                calls = invoke(mode="publish", config=config, success=False)
                self.assertFalse(any('--method' in call or call[0] == 'release' for call in calls))
            calls = invoke(mode="publish", config={}, overrides={"RELEASE_REF": "refs/tags/v0.0.2"}, success=False)
            self.assertFalse(any('--method' in call for call in calls))
            calls = invoke(mode="publish", config={})
            writes = [call for call in calls if '--method' in call or call[0] == 'release']
            self.assertEqual(len(writes), 2)
            self.assertIn('ref=refs/tags/v0.0.1', writes[0])
            self.assertIn('sha=' + SHA, writes[0])
            self.assertEqual(writes[1][:4], ['release', 'create', 'v0.0.1', '--verify-tag'])
            for failure in ('--method POST', '/commits/', 'release create'):
                calls = invoke(mode="publish", config={"fail": failure}, success=False)
                self.assertIn(failure, ' '.join(calls[-1]))
            for response in (None, {"ref": "refs/tags/wrong", "object": {"sha": SHA}},
                             {"ref": "refs/tags/v0.0.1", "object": {"sha": "0" * 40}}):
                calls = invoke(mode="publish", config={"created_response": response}, success=False)
                self.assertFalse(any(call[0] == 'release' for call in calls))
            calls = invoke(mode="publish", config={}, overrides={"RELEASE_MODE": "existing"})
            self.assertFalse(any('--method' in call for call in calls))
            (assets / "sidecar_linux_amd64.tar.gz").write_bytes(b"tampered")
            self.assertEqual(invoke(mode="publish", config={}, success=False), [])


if __name__ == '__main__':
    unittest.main()
