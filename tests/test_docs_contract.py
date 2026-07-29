import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class TestDocsContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.readme = (ROOT / "README.md").read_text(encoding="utf-8")
        cls.context = (ROOT / "context.md").read_text(encoding="utf-8")

    def test_readme_documents_latest_release_assets(self):
        for filename in (
            "geosite.dat",
            "geoip.dat",
            "geosite.dat.sha256sum",
            "geoip.dat.sha256sum",
        ):
            self.assertIn(f"releases/latest/download/{filename}", self.readme)

    def test_docs_use_geodata_only_delivery_language(self):
        combined = self.readme + self.context

        self.assertIn("轻量完整增强底", combined)
        self.assertIn("不发布 `.srs`", combined)
        self.assertIn("domain-list-community", combined)
        self.assertIn("Loyalsoldier/geoip", combined)

    def test_readme_documents_local_build_and_dat_converter(self):
        self.assertIn("scripts/bootstrap_vendor.sh", self.readme)
        self.assertIn("scripts/build.py", self.readme)
        self.assertIn("scripts/probe_tags.py", self.readme)
        self.assertIn("clash2passwall.js", self.readme)
        self.assertIn("--dat", self.readme)
        self.assertIn("geoview >= 0.1.10", self.readme)


if __name__ == "__main__":
    unittest.main()
