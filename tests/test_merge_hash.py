import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.merge_and_hash import (
    assert_no_name_collision,
    build_geoip_config,
    merge_data_dirs,
    write_sha256,
)


ROOT = Path(__file__).resolve().parents[1]


class TestMergeHash(unittest.TestCase):
    def test_base_config_declares_official_dat_input(self):
        config = json.loads((ROOT / "config" / "geoip.base.json").read_text(encoding="utf-8"))

        self.assertEqual(config["input"][0]["type"], "v2rayGeoIPDat")
        self.assertEqual(config["output"][0]["args"]["outputName"], "geoip.dat")

    def test_collision_raises_before_merge(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            community = Path(temporary_directory) / "community"
            community.mkdir()
            (community / "cn").write_text("domain:example.cn\n", encoding="utf-8")

            with self.assertRaises(SystemExit):
                assert_no_name_collision({"cn"}, community)

    def test_merge_copies_community_and_custom_lists(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            community = root / "community"
            custom = root / "custom"
            output = root / "merged"
            community.mkdir()
            custom.mkdir()
            (community / "cn").write_text("domain:example.cn\n", encoding="utf-8")
            (custom / "my-list").write_text("full:example.com\n", encoding="utf-8")

            merge_data_dirs(community, custom, output)

            self.assertEqual({path.name for path in output.iterdir()}, {"cn", "my-list"})

    def test_sha256_uses_two_spaces_and_basename(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            dat = Path(temporary_directory) / "geosite.dat"
            dat.write_bytes(b"abc")

            checksum = write_sha256(dat)

            digest = hashlib.sha256(b"abc").hexdigest()
            self.assertEqual(checksum.read_text(encoding="utf-8"), f"{digest}  geosite.dat\n")

    def test_geoip_config_starts_with_base_and_adds_sorted_custom_tags(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            ip_dir = root / "ip"
            publish_dir = root / "publish"
            output_json = root / "geoip-config.json"
            ip_dir.mkdir()
            (ip_dir / "zeta.txt").write_text("2.2.2.0/24\n", encoding="utf-8")
            (ip_dir / "alpha.txt").write_text("1.1.1.0/24\n", encoding="utf-8")

            build_geoip_config(
                "https://example.com/geoip.dat",
                ip_dir,
                output_json,
                publish_dir,
                template_path=ROOT / "config" / "geoip.base.json",
            )

            config = json.loads(output_json.read_text(encoding="utf-8"))
            self.assertEqual(config["input"][0]["type"], "v2rayGeoIPDat")
            self.assertEqual(
                [entry["args"].get("name") for entry in config["input"][1:]],
                ["alpha", "zeta"],
            )
            self.assertEqual(config["output"][0]["args"]["outputName"], "geoip.dat")

    def test_geoip_config_preserves_template_options(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            template = root / "template.json"
            template.write_text(
                json.dumps(
                    {
                        "input": [{"type": "v2rayGeoIPDat", "action": "add", "args": {"uri": "old", "wantedList": ["cn"]}}],
                        "output": [{"type": "v2rayGeoIPDat", "action": "output", "args": {"outputDir": "old", "outputName": "old.dat", "onlyIPType": "IPv4"}}],
                    }
                ),
                encoding="utf-8",
            )
            output = root / "generated.json"

            build_geoip_config(
                "https://example.test/base.dat",
                root / "ip",
                output,
                root / "publish",
                template_path=template,
            )

            generated = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(generated["input"][0]["args"]["wantedList"], ["cn"])
            self.assertEqual(generated["output"][0]["args"]["onlyIPType"], "IPv4")


if __name__ == "__main__":
    unittest.main()
