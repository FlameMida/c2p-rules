import sys
import tempfile
import unittest
import urllib.error
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from build import BuildError, emit_sources, fetch


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
                    lambda _: (
                        "payload:\n"
                        "  - 'DOMAIN-SUFFIX,wrong-side.example'\n"
                        "  - 'IP-CIDR,1.2.3.0/24'\n"
                    ),
                    root / "data",
                    root / "ip",
                )

    def test_source_sides_are_mandatory(self):
        source = {"name": "missing-sides", "behavior": "domain", "url": "source"}
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "missing-sides.*sides.*required"):
                emit_sources(
                    [source],
                    lambda _: "payload:\n  - example.com\n",
                    root / "data",
                    root / "ip",
                )

    def test_process_only_source_must_match_declared_nonempty_side(self):
        content = "payload:\n  - 'PROCESS-NAME,Chrome'\n"
        for side in ("geosite", "geoip"):
            source = {
                "name": f"process-{side}",
                "behavior": "classical",
                "url": "source",
                "sides": [side],
            }
            with self.subTest(side=side), tempfile.TemporaryDirectory() as temporary_directory:
                root = Path(temporary_directory)
                with self.assertRaisesRegex(BuildError, f"process-{side}.*declared sides"):
                    emit_sources([source], lambda _: content, root / "data", root / "ip")

    def test_domain_source_must_not_be_empty(self):
        source = {
            "name": "empty-domain",
            "behavior": "domain",
            "url": "empty",
            "sides": ["geosite"],
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "empty-domain.*domain"):
                emit_sources([source], lambda _: "payload: []\n", root / "data", root / "ip")

    def test_ipcidr_source_must_not_be_empty(self):
        source = {
            "name": "empty-ip",
            "behavior": "ipcidr",
            "url": "empty",
            "sides": ["geoip"],
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            with self.assertRaisesRegex(BuildError, "empty-ip.*IP"):
                emit_sources([source], lambda _: "payload: []\n", root / "data", root / "ip")

    def test_classical_source_must_have_domain_unless_process_only(self):
        source = {
            "name": "ip-only-classical",
            "behavior": "classical",
            "url": "ip",
            "sides": ["geosite", "geoip"],
        }
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
        source = {
            "name": "offline",
            "behavior": "domain",
            "url": "bad",
            "sides": ["geosite"],
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)

            def fail_fetch(_url):
                raise OSError("network down")

            with self.assertRaisesRegex(BuildError, "offline.*network down"):
                emit_sources([source], fail_fetch, root / "data", root / "ip")

    def test_http_404_is_fatal(self):
        source = {
            "name": "missing",
            "behavior": "domain",
            "url": "https://example.test/404",
            "sides": ["geosite"],
        }
        error = urllib.error.HTTPError(source["url"], 404, "Not Found", {}, None)
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            with mock.patch("build.urllib.request.urlopen", side_effect=error):
                with self.assertRaisesRegex(BuildError, "missing.*404"):
                    emit_sources([source], fetch, root / "data", root / "ip")

    def test_duplicate_source_name_is_fatal(self):
        sources = [
            {"name": "same", "behavior": "domain", "url": "one", "sides": ["geosite"]},
            {"name": "same", "behavior": "domain", "url": "two", "sides": ["geosite"]},
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
