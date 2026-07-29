import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.buckets import empty_buckets
from lib.fetch_emit import emit_source_files, parse_source_content
from lib.sources import is_applications_source, load_sources


ROOT = Path(__file__).resolve().parents[1]


class TestSources(unittest.TestCase):
    def test_load_sources_yaml(self):
        sources = load_sources(ROOT / "sources.yaml")
        names = {source["name"] for source in sources}

        self.assertIn("loyalsoldier-gfw", names)
        self.assertIn("xiaolin-netflix", names)
        self.assertNotIn("applications", names)

    def test_parse_yaml_domain_payload(self):
        content = "payload:\n  - '+.example.com'\n  - 'www.example.com'\n"
        source = {"name": "test", "behavior": "domain", "format": "yaml"}

        buckets, raw_count, skipped = parse_source_content(source, content)

        self.assertEqual(raw_count, 2)
        self.assertEqual(skipped, [])
        self.assertIn("example.com", buckets["domain_suffix"])

    def test_process_only_source_is_applications_source(self):
        source = {"name": "desktop-only", "behavior": "classical"}
        buckets, _, skipped = parse_source_content(
            source,
            "payload:\n  - 'PROCESS-NAME,Chrome'\n  - 'PROCESS-NAME,Firefox'\n",
        )

        self.assertTrue(is_applications_source(source, buckets, skipped))

    def test_unknown_behavior_is_a_parse_error(self):
        source = {"name": "broken", "behavior": "mystery", "format": "yaml"}

        with self.assertRaisesRegex(ValueError, "behavior"):
            parse_source_content(source, "payload:\n  - example.com\n")

    def test_unknown_format_is_a_parse_error(self):
        source = {"name": "broken", "behavior": "domain", "format": "json"}

        with self.assertRaisesRegex(ValueError, "format"):
            parse_source_content(source, "example.com\n")

    def test_emit_youtube_has_no_geoip_file(self):
        buckets = empty_buckets()
        buckets["domain_suffix"].append("youtube.com")
        with tempfile.TemporaryDirectory() as temporary_directory:
            data_dir = Path(temporary_directory) / "data"
            ip_dir = Path(temporary_directory) / "ip"

            metadata = emit_source_files("xiaolin-youtube", buckets, data_dir, ip_dir)

            self.assertTrue(metadata["geosite"])
            self.assertFalse(metadata["geoip"])
            self.assertTrue((data_dir / "xiaolin-youtube").is_file())
            self.assertFalse((ip_dir / "xiaolin-youtube.txt").exists())

    def test_emit_netflix_has_domain_and_ip_files(self):
        buckets = empty_buckets()
        buckets["domain_suffix"].append("netflix.com")
        buckets["ip_cidr"].append("23.246.0.0/18")
        with tempfile.TemporaryDirectory() as temporary_directory:
            data_dir = Path(temporary_directory) / "data"
            ip_dir = Path(temporary_directory) / "ip"

            metadata = emit_source_files("xiaolin-netflix", buckets, data_dir, ip_dir)

            self.assertTrue(metadata["geosite"] and metadata["geoip"])
            self.assertEqual(
                (data_dir / "xiaolin-netflix").read_text(encoding="utf-8"),
                "domain:netflix.com\n",
            )
            self.assertEqual(
                (ip_dir / "xiaolin-netflix.txt").read_text(encoding="utf-8"),
                "23.246.0.0/18\n",
            )


if __name__ == "__main__":
    unittest.main()
