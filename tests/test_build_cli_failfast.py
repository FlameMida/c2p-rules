import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PYTHON = Path(sys.executable)
RELEASE_ASSETS = {
    "geosite.dat",
    "geoip.dat",
    "geosite.dat.sha256sum",
    "geoip.dat.sha256sum",
}


class TestBuildCliFailFast(unittest.TestCase):
    def run_failure(self, source_text, expected_error, *, community_files=(), missing_source=False):
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary = Path(temporary_directory)
            workspace = temporary / "workspace"
            community = temporary / "community"
            bin_dir = temporary / "bin"
            sentinel = temporary / "compiler-called"
            community.mkdir()
            bin_dir.mkdir()
            for name in community_files:
                (community / name).write_text("domain:existing.example\n", encoding="utf-8")
            for command in ("go", "geoip"):
                executable = bin_dir / command
                executable.write_text(
                    f"#!/bin/sh\ntouch '{sentinel}'\nexit 99\n", encoding="utf-8"
                )
                executable.chmod(0o755)

            if missing_source:
                source_text = source_text.replace(
                    "SOURCE_URL", (temporary / "missing-404.yaml").as_uri()
                )

            sources = temporary / "sources.yaml"
            sources.write_text(source_text, encoding="utf-8")
            env = {
                **os.environ,
                "PATH": str(bin_dir) + os.pathsep + os.environ["PATH"],
                "NO_PROXY": "127.0.0.1,localhost",
                "no_proxy": "127.0.0.1,localhost",
            }
            result = subprocess.run(
                [
                    str(PYTHON),
                    str(ROOT / "scripts" / "build.py"),
                    "--sources",
                    str(sources),
                    "--work-root",
                    str(workspace),
                    "--community",
                    str(community),
                ],
                cwd=ROOT,
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn(expected_error, (result.stdout + result.stderr).lower())
            self.assertFalse(sentinel.exists(), "compiler must not run after an earlier failure")
            publish = workspace / "publish"
            actual = {path.name for path in publish.iterdir()} if publish.is_dir() else set()
            self.assertNotEqual(actual, RELEASE_ASSETS)

    def test_missing_source_stops_before_compile(self):
        self.run_failure(
            "sources:\n"
            "  - {name: offline, behavior: domain, sides: [geosite], url: SOURCE_URL}\n",
            "no such file",
            missing_source=True,
        )

    def test_malformed_payload_stops_before_compile(self):
        with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as payload:
            payload.write("payload: example.com\n")
            payload_path = Path(payload.name)
        self.addCleanup(payload_path.unlink, missing_ok=True)
        self.run_failure(
            "sources:\n"
            f"  - {{name: malformed, behavior: domain, sides: [geosite], url: '{payload_path.as_uri()}'}}\n",
            "payload must be a list",
        )

    def test_collision_stops_before_compile(self):
        with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as payload:
            payload.write("payload:\n  - example.com\n")
            payload_path = Path(payload.name)
        self.addCleanup(payload_path.unlink, missing_ok=True)
        self.run_failure(
            "sources:\n"
            f"  - {{name: collision, behavior: domain, sides: [geosite], url: '{payload_path.as_uri()}'}}\n",
            "collide",
            community_files=("collision",),
        )


if __name__ == "__main__":
    unittest.main()
