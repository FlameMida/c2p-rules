import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from probe_tags import main, probe_one


class TestProbeTags(unittest.TestCase):
    def test_missing_dat_fails(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            expected = root / "expected.json"
            expected.write_text('{"geosite": [], "geoip": []}\n', encoding="utf-8")

            result = main(
                [
                    "--dat",
                    str(root / "missing.dat"),
                    "--expect",
                    str(expected),
                    "--side",
                    "geosite",
                ]
            )

            self.assertEqual(result, 1)

    def test_probe_one_requires_nonempty_converter_output(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            dat = root / "geosite.dat"
            dat.write_bytes(b"dat")
            converter = root / "fake-geoview"
            converter.write_text(
                "#!/bin/sh\n"
                "while [ \"$#\" -gt 0 ]; do\n"
                "  if [ \"$1\" = -output ]; then shift; printf converted > \"$1\"; exit 0; fi\n"
                "  shift\n"
                "done\n"
                "exit 2\n",
                encoding="utf-8",
            )
            converter.chmod(0o755)

            self.assertTrue(probe_one(str(converter), "geosite", dat, "cn"))

    def test_main_fails_when_any_expected_tag_is_missing(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            dat = root / "geosite.dat"
            dat.write_bytes(b"dat")
            expected = root / "expected.json"
            expected.write_text(
                json.dumps({"geosite": ["custom"], "geoip": []}),
                encoding="utf-8",
            )
            converter = root / "fake-geoview"
            converter.write_text("#!/bin/sh\nexit 1\n", encoding="utf-8")
            converter.chmod(0o755)

            result = main(
                [
                    "--dat",
                    str(dat),
                    "--expect",
                    str(expected),
                    "--side",
                    "geosite",
                    "--geoview",
                    str(converter),
                ]
            )

            self.assertEqual(result, 1)

    def test_forbidden_probe_fails_when_tag_exists(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            dat = root / "geoip.dat"
            dat.write_bytes(b"dat")
            expected = root / "expected.json"
            expected.write_text(
                json.dumps(
                    {
                        "required": {"geosite": [], "geoip": []},
                        "forbidden": {"geosite": [], "geoip": ["domain-only"]},
                    }
                ),
                encoding="utf-8",
            )
            converter = root / "fake-geoview"
            converter.write_text(
                "#!/bin/sh\n"
                "while [ \"$#\" -gt 0 ]; do\n"
                "  if [ \"$1\" = -output ]; then shift; printf converted > \"$1\"; exit 0; fi\n"
                "  shift\n"
                "done\n",
                encoding="utf-8",
            )
            converter.chmod(0o755)

            result = main(
                [
                    "--dat", str(dat),
                    "--expect", str(expected),
                    "--side", "geoip",
                    "--forbid",
                    "--geoview", str(converter),
                ]
            )

            self.assertEqual(result, 1)


if __name__ == "__main__":
    unittest.main()
