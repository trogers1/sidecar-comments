#!/usr/bin/env python3
"""Offline public command tests with real commits/remotes and a JSON gh fake."""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class VersionReleaseTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name)
        self.repo = self.work / "repo"
        self.repo.mkdir()
        (self.repo / "scripts").mkdir()
        shutil.copy(ROOT / "scripts/version_release.py", self.repo / "scripts/version_release.py")
        (self.repo / ".githooks").mkdir()
        shutil.copy(ROOT / ".githooks/pre-push", self.repo / ".githooks/pre-push")
        self.git(args=["init", "-b", "main"])
        self.git(args=["config", "user.name", "Test"])
        self.git(args=["config", "user.email", "test@example.invalid"])
        self.remote = self.work / "remote.git"
        self.git(args=["init", "--bare", str(self.remote)])
        self.sha = self.commit(text="0.1.2\n")
        self.git(args=["push", str(self.remote), "HEAD:refs/heads/main"])
        self.tools = self.work / "tools"
        self.tools.mkdir()
        gh = self.tools / "gh"
        gh.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ['TRACE'], 'a') as f:
    f.write(json.dumps(args) + '\\n')
config = json.loads(Path(os.environ['CONFIG']).read_text())
if config.get('fail'):
    sys.exit('API unavailable')
if '--method' in args:
    print(json.dumps(config.get('created', {'ref': os.environ['RELEASE_REF'], 'object': {'sha': os.environ['RELEASE_SHA']}})))
else:
    print(json.dumps(config.get('response', [[{'ref': 'refs/tags/' + tag} for tag in config.get('tags', [])]])))
''')
        gh.chmod(0o755)
        self.env = {**os.environ, "PYTHONDONTWRITEBYTECODE": "1", "PATH": f"{self.tools}:{os.environ['PATH']}",
                    "GH_REPO": "example/repo", "GH_TOKEN": "fake", "GITHUB_EVENT_NAME": "push",
                    "GITHUB_REF": "refs/heads/main", "GITHUB_SHA": self.sha,
                    "RELEASE_SHA": self.sha, "RELEASE_REF": "refs/tags/v0.1.2", "RELEASE_BASE": "refs/tags/v0.0.0",
                    "GITHUB_OUTPUT": str(self.work / "output"), "TRACE": str(self.work / "trace"),
                    "CONFIG": str(self.work / "config")}

    def git(self, *, args: list[str]) -> str:
        return subprocess.run(["git", *args], cwd=self.repo, check=True, capture_output=True, text=True).stdout.strip()

    def commit(self, *, text: str) -> str:
        (self.repo / "VERSION").write_text(text)
        self.git(args=["add", "VERSION"])
        self.git(args=["commit", "--allow-empty", "-m", "version"])
        return self.git(args=["rev-parse", "HEAD"])

    def invoke(self, *, args: list[str], success: bool = True, config: dict[str, object] | None = None,
               overrides: dict[str, str] | None = None, stdin: str = "", hook: bool = False) -> list[list[str]]:
        (self.work / "config").write_text(json.dumps(config or {}))
        (self.work / "trace").write_text("")
        (self.work / "output").write_text("")
        command = ([str(self.repo / ".githooks/pre-push")] if hook else
                   ["python3", str(self.repo / "scripts/version_release.py")])
        result = subprocess.run([*command, *args], cwd=self.repo / "scripts", env={**self.env, **(overrides or {})},
                                input=stdin, capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode == 0, success, result.stderr)
        return [json.loads(line) for line in (self.work / "trace").read_text().splitlines()]

    def update(self, *, sha: str | None = None, destination: str = "refs/heads/main") -> str:
        return f"refs/heads/topic {sha or self.sha} {destination} {'0' * 40}\n"

    def test_plan_and_create(self) -> None:
        calls = self.invoke(args=["plan"], config={"tags": ["v0.1.1", "v01.2.3", "v10.0.0-rc.1", "latest"]})
        self.assertFalse(any("--method" in call for call in calls))
        self.assertEqual((self.work / "output").read_text(),
                         f"sha={self.sha}\nref=refs/tags/v0.1.2\nbase=refs/tags/v0.1.1\n")
        self.invoke(args=["plan"], overrides={"GITHUB_EVENT_NAME": "pull_request", "GITHUB_REF": "refs/pull/123/merge"})
        calls = self.invoke(args=["create"])
        self.assertEqual(len(calls), 2)
        self.assertIn("ref=refs/tags/v0.1.2", calls[1])
        self.assertIn("sha=" + self.sha, calls[1])
        self.assertEqual(self.git(args=["tag", "--list"]), "")

    def test_numeric_highest_and_pagination(self) -> None:
        sha = self.commit(text="11.0.0\n")
        self.invoke(args=["plan"], overrides={"GITHUB_SHA": sha}, config={"response": [
            [{"ref": "refs/tags/v9.99.99"}],
            [{"ref": "refs/tags/v10.2.3"}, {"ref": "refs/tags/v010.99.99"}],
            [{"ref": "refs/tags/v10.10.0"}, {"ref": "refs/tags/v12.0.0-rc.1"}]]})
        self.assertIn("base=refs/tags/v10.10.0\n", (self.work / "output").read_text())
        # A dirty worktree does not change the committed CI candidate either.
        (self.repo / "VERSION").write_text("999.0.0\n")
        self.invoke(args=["plan"], overrides={"GITHUB_SHA": sha})
        self.assertIn("ref=refs/tags/v11.0.0\n", (self.work / "output").read_text())
        for tag in ("v9.99.99", "v10.10.0", "v010.99.99", "v12.0.0-rc.1"):
            self.git(args=["tag", tag])
            self.git(args=["push", str(self.remote), "refs/tags/" + tag])
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(sha=sha))

    def test_ci_failure_has_no_writes(self) -> None:
        for config in ({"tags": ["v0.1.2"]}, {"tags": ["v0.2.0"]}, {"fail": True},
                       {"response": None}, {"response": [[{}]]}, {"response": [[{"ref": "refs/heads/main"}]]}):
            for mode in ("plan", "create"):
                calls = self.invoke(args=[mode], config=config, success=False)
                self.assertFalse(any("--method" in call for call in calls))
                self.assertEqual((self.work / "output").read_text(), "")
        for overrides in ({"GITHUB_EVENT_NAME": "workflow_dispatch"}, {"GITHUB_REF": "refs/heads/topic"},
                          {"GITHUB_SHA": "0" * 40}):
            self.assertEqual(self.invoke(args=["plan"], overrides=overrides, success=False), [])
        for overrides in ({"RELEASE_SHA": "0" * 40}, {"RELEASE_REF": "refs/tags/v0.1.3"},
                          {"GITHUB_EVENT_NAME": "pull_request", "GITHUB_REF": "refs/pull/1/merge"}):
            self.assertEqual(self.invoke(args=["create"], overrides=overrides, success=False), [])
        calls = self.invoke(args=["create"], config={"tags": ["v0.1.1"]}, success=False)
        self.assertFalse(any("--method" in call for call in calls))
        self.invoke(args=["create"], config={"created": {"ref": "refs/tags/v0.1.2", "object": {"sha": "0" * 40}}}, success=False)

    def test_malformed_committed_versions(self) -> None:
        for text in ("v0.1.2\n", "01.2.3\n", "1.2\n", "1.2.3-rc.1\n", "1.2.3 \n", "1.2.3\n\n", "１.2.3\n"):
            sha = self.commit(text=text)
            self.assertEqual(self.invoke(args=["plan"], overrides={"GITHUB_SHA": sha}, success=False), [])
            self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(sha=sha), success=False)
        (self.repo / "VERSION").unlink()
        self.git(args=["add", "VERSION"])
        self.git(args=["commit", "-m", "missing"])
        self.invoke(args=["pre-push", "origin", str(self.remote)],
                    stdin=self.update(sha=self.git(args=["rev-parse", "HEAD"])), success=False)

    def test_hook_uses_exact_commit_and_remote_tags(self) -> None:
        self.git(args=["tag", "v0.1.1"])
        self.git(args=["push", str(self.remote), "refs/tags/v0.1.1"])
        self.git(args=["tag", "v99.0.0"])
        (self.repo / "VERSION").write_text("bad worktree\n")
        self.invoke(args=["origin", str(self.remote)], stdin=self.update(), hook=True)
        newer_sha = self.commit(text="0.2.0\n")
        self.git(args=["tag", "v0.1.2"])
        self.git(args=["push", str(self.remote), "refs/tags/v0.1.2"])
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(), success=False)
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(sha=newer_sha))
        old_sha = self.commit(text="0.1.0\n")
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(sha=old_sha), success=False)
        self.invoke(args=["pre-push", "origin", str(self.remote)],
                    stdin=self.update(destination="refs/heads/topic") + self.update(sha=newer_sha))
        self.invoke(args=["pre-push", "origin", str(self.remote)],
                    stdin=self.update(sha=newer_sha) + self.update(sha=old_sha), success=False)
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin=self.update(sha="0" * 40), success=False)
        missing = str(self.work / "missing")
        self.invoke(args=["pre-push", "origin", missing], stdin=self.update(destination="refs/heads/topic"))
        self.invoke(args=["pre-push", "origin", missing], stdin=self.update(), success=False)
        self.invoke(args=["pre-push", "origin", str(self.remote)], stdin="malformed\n", success=False)
        self.assertEqual(self.git(args=["ls-remote", "--tags", str(self.remote)]).count("refs/tags/"), 2)

    def test_bump(self) -> None:
        for bump, expected in (("patch", "0.1.3\n"), ("minor", "0.2.0\n"), ("major", "1.0.0\n")):
            (self.repo / "VERSION").write_text("0.1.2\n")
            self.assertEqual(self.invoke(args=["bump", bump]), [])
            self.assertEqual((self.repo / "VERSION").read_text(), expected)
        self.invoke(args=["bump", "oops"], success=False)


if __name__ == "__main__":
    unittest.main()
