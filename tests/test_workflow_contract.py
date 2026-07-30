import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github" / "workflows" / "build.yml"
EXPECTED_RELEASE_ASSETS = {
    "geosite.dat",
    "geoip.dat",
    "geosite.dat.sha256sum",
    "geoip.dat.sha256sum",
}


class TestWorkflowContract(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text(encoding="utf-8")
        self.workflow = yaml.safe_load(self.text)

    def test_build_and_publish_have_separate_least_privilege_tokens(self):
        jobs = self.workflow["jobs"]
        self.assertEqual(jobs["build"]["permissions"], {"contents": "read"})
        self.assertEqual(jobs["publish"]["permissions"], {"contents": "write"})
        self.assertEqual(jobs["publish"]["needs"], "build")
        checkout = next(step for step in jobs["build"]["steps"] if str(step.get("uses", "")).startswith("actions/checkout@"))
        self.assertFalse(checkout["with"]["persist-credentials"])

    def test_all_actions_are_pinned_to_full_commit_sha(self):
        for job in self.workflow["jobs"].values():
            for step in job.get("steps", []):
                if "uses" in step:
                    self.assertRegex(step["uses"], r"^[^@]+@[0-9a-f]{40}$")

    def test_draft_release_is_verified_before_becoming_latest(self):
        publish_text = "\n".join(
            step.get("run", "") for step in self.workflow["jobs"]["publish"]["steps"]
        )
        self.assertIn("gh release create", publish_text)
        self.assertIn("--draft", publish_text)
        self.assertIn("gh release upload", publish_text)
        self.assertIn("gh api", publish_text)
        self.assertIn("gh release edit", publish_text)
        self.assertRegex(publish_text, r"--latest")
        self.assertLess(publish_text.index("gh api"), publish_text.index("gh release edit"))
        for asset in EXPECTED_RELEASE_ASSETS:
            self.assertIn(asset, publish_text)
        self.assertNotIn("softprops/action-gh-release", self.text)

    def test_build_gates_artifact_on_all_tests_probes_and_checksums(self):
        build_text = "\n".join(
            step.get("run", "") for step in self.workflow["jobs"]["build"]["steps"]
        )
        self.assertIn("python -m unittest discover", build_text)
        self.assertIn("npm ci", build_text)
        self.assertIn("npm test", build_text)
        self.assertIn("--tag-manifest build/expected_tags.json", build_text)
        self.assertIn("verify_manifest_refs.cjs", build_text)
        self.assertIn("--side geosite", build_text)
        self.assertIn("--side geoip", build_text)
        self.assertGreaterEqual(build_text.count("--forbid"), 2)
        self.assertIn("sha256sum -c geosite.dat.sha256sum", build_text)
        self.assertIn("sha256sum -c geoip.dat.sha256sum", build_text)
        self.assertNotIn("dist/*.srs", build_text)


class TestBootstrapContract(unittest.TestCase):
    def test_executable_tool_repositories_are_exactly_pinned(self):
        text = (ROOT / "scripts" / "bootstrap_vendor.sh").read_text(encoding="utf-8")
        for commit in (
            "efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d",
            "85084dfbe282e4e9cb460b07196e6eecfd126d19",
            "3c91926d360b8f49d47520639e574608318baf12",
        ):
            self.assertIn(commit, text)
        self.assertNotRegex(text, r"git[^\n]+pull")
        self.assertRegex(text, r"checkout[^\n]+--detach")


if __name__ == "__main__":
    unittest.main()
