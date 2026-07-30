import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from build import BuildError, emit_sources


class TestBuildOrchestration(unittest.TestCase):
    def test_emit_sources_derives_expected_tags_and_skips_process_only(self):
        sources = [
            {"name": "site", "behavior": "domain", "url": "site", "sides": ["geosite"]},
            {"name": "mixed", "behavior": "classical", "url": "mixed", "sides": ["geosite", "geoip"]},
            {"name": "desktop", "behavior": "classical", "url": "desktop", "sides": []},
        ]
        content = {
            "site": "payload:\n  - '+.example.com'\n",
            "mixed": "payload:\n  - 'DOMAIN-SUFFIX,mixed.test'\n  - 'IP-CIDR,1.2.3.0/24'\n",
            "desktop": "payload:\n  - 'PROCESS-NAME,Chrome'\n",
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            expected = emit_sources(
                sources,
                content.__getitem__,
                root / "data",
                root / "ip",
            )

            self.assertEqual(expected["geosite"], ["mixed", "site"])
            self.assertEqual(expected["geoip"], ["mixed"])
            self.assertEqual(expected["required"]["geosite"], ["cn", "mixed", "site"])
            self.assertEqual(expected["required"]["geoip"], ["cn", "mixed", "private"])
            self.assertIn("desktop", expected["forbidden"]["geosite"])
            self.assertIn("site", expected["forbidden"]["geoip"])
            self.assertFalse((root / "data" / "desktop").exists())

    def test_declared_source_sides_are_an_independent_contract(self):
        source = {
            "name": "wrong-side",
            "behavior": "classical",
            "url": "source",
            "sides": ["geosite"],
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "wrong-side.*declared sides"):
                emit_sources(
                    [source],
                    lambda _: "payload:\n  - 'IP-CIDR,1.2.3.0/24'\n",
                    root / "data",
                    root / "ip",
                )

    def test_domain_source_must_not_be_empty(self):
        source = {"name": "empty-domain", "behavior": "domain", "url": "empty"}
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "empty-domain.*domain"):
                emit_sources([source], lambda _: "payload: []\n", root / "data", root / "ip")

    def test_ipcidr_source_must_not_be_empty(self):
        source = {"name": "empty-ip", "behavior": "ipcidr", "url": "empty"}
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "empty-ip.*IP"):
                emit_sources([source], lambda _: "payload: []\n", root / "data", root / "ip")

    def test_classical_source_must_have_domain_unless_process_only(self):
        source = {"name": "ip-only-classical", "behavior": "classical", "url": "ip"}
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "ip-only-classical.*domain"):
                emit_sources(
                    [source],
                    lambda _: "payload:\n  - 'IP-CIDR,1.2.3.0/24'\n",
                    root / "data",
                    root / "ip",
                )

    def test_fetch_error_is_fatal(self):
        source = {"name": "offline", "behavior": "domain", "url": "bad"}
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            def fail_fetch(_url):
                raise OSError("network down")

            with self.assertRaisesRegex(BuildError, "offline.*network down"):
                emit_sources([source], fail_fetch, root / "data", root / "ip")

    def test_duplicate_source_name_is_fatal(self):
        sources = [
            {"name": "same", "behavior": "domain", "url": "one"},
            {"name": "same", "behavior": "domain", "url": "two"},
        ]
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "duplicate.*same"):
                emit_sources(
                    sources,
                    lambda _: "payload:\n  - example.com\n",
                    root / "data",
                    root / "ip",
                )


if __name__ == "__main__":
    unittest.main()
