#!/usr/bin/env python3
"""Probe required list names in a geosite.dat or geoip.dat file."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import tempfile
from pathlib import Path


def probe_one(geoview: str, data_type: str, dat: Path, tag: str) -> bool:
    with tempfile.TemporaryDirectory() as temporary_directory:
        output = Path(temporary_directory) / "probe.srs"
        try:
            result = subprocess.run(
                [
                    geoview,
                    "-type",
                    data_type,
                    "-action",
                    "convert",
                    "-input",
                    str(dat),
                    "-list",
                    tag,
                    "-output",
                    str(output),
                    "-lowmem=true",
                ],
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError:
            return False
        return result.returncode == 0 and output.is_file() and output.stat().st_size > 0


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dat", type=Path, required=True)
    parser.add_argument("--expect", type=Path, required=True)
    parser.add_argument("--side", choices=["geosite", "geoip"], required=True)
    parser.add_argument("--geoview", default="geoview")
    parser.add_argument("--also", nargs="*", default=["cn"])
    parser.add_argument("--forbid", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if not args.dat.is_file():
        print(f"missing dat: {args.dat}", file=sys.stderr)
        return 1
    if not args.expect.is_file():
        print(f"missing expected-tags manifest: {args.expect}", file=sys.stderr)
        return 1

    expected = json.loads(args.expect.read_text(encoding="utf-8"))
    if args.forbid:
        tags = expected.get("forbidden", {}).get(args.side, [])
    else:
        required = expected.get("required", expected)
        tags = list(dict.fromkeys([*required[args.side], *args.also]))
    failed = []
    for tag in tags:
        success = probe_one(args.geoview, args.side, args.dat, tag)
        accepted = not success if args.forbid else success
        print(("✓" if accepted else "✗"), tag, "(absent)" if args.forbid else "")
        if not accepted:
            failed.append(tag)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
