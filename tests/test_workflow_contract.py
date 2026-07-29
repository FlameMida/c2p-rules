import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github" / "workflows" / "build.yml"
EXPECTED_RELEASE_ASSETS = {
    "publish/geosite.dat",
    "publish/geoip.dat",
    "publish/geosite.dat.sha256sum",
    "publish/geoip.dat.sha256sum",
}


class TestWorkflowContract(unittest.TestCase):
    def test_release_contains_exactly_four_geodata_assets(self):
        workflow = yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))
        steps = workflow["jobs"]["build"]["steps"]
        release_step = next(
            step for step in steps if str(step.get("uses", "")).startswith("softprops/action-gh-release@")
        )
        assets = {
            line.strip()
            for line in release_step["with"]["files"].splitlines()
            if line.strip()
        }

        self.assertEqual(assets, EXPECTED_RELEASE_ASSETS)
        self.assertTrue(release_step["with"]["make_latest"])

    def test_build_gates_release_on_tests_probes_and_checksums(self):
        text = WORKFLOW.read_text(encoding="utf-8")

        self.assertIn("python -m unittest discover", text)
        self.assertIn("--side geosite", text)
        self.assertIn("--side geoip", text)
        self.assertIn("sha256sum -c geosite.dat.sha256sum", text)
        self.assertIn("sha256sum -c geoip.dat.sha256sum", text)
        self.assertNotIn("dist/*.srs", text)


if __name__ == "__main__":
    unittest.main()
