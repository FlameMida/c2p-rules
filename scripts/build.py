#!/usr/bin/env python3
"""Build geosite.dat and geoip.dat from sources.yaml."""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
import urllib.request
from collections.abc import Callable, Iterable
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "scripts"))

from lib.fetch_emit import emit_source_files, parse_source_content
from lib.merge_and_hash import (
    assert_no_name_collision,
    build_geoip_config,
    merge_data_dirs,
    write_sha256,
)
from lib.sources import is_applications_source, load_sources


UA = "clash-rules-srs-builder/2.0-dat"
BUILD = ROOT / "build"
CUSTOM_DATA = BUILD / "data"
IP_DIR = BUILD / "ip"
MERGED = BUILD / "data-merged"
PUBLISH = ROOT / "publish"
COMMUNITY = ROOT / "vendor" / "domain-list-community" / "data"
GEOIP_BASE = "https://github.com/Loyalsoldier/geoip/releases/latest/download/geoip.dat"
TAG_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
RELEASE_ASSETS = {
    "geosite.dat",
    "geoip.dat",
    "geosite.dat.sha256sum",
    "geoip.dat.sha256sum",
}


class BuildError(RuntimeError):
    """A fatal build-contract violation."""


def fetch(url: str) -> str:
    request = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(request, timeout=60) as response:
        return response.read().decode("utf-8")


def emit_sources(
    sources: Iterable[dict],
    fetcher: Callable[[str], str],
    data_dir: Path,
    ip_dir: Path,
) -> dict[str, list[str]]:
    expected_site: set[str] = set()
    expected_ip: set[str] = set()
    seen_names: set[str] = set()
    declared_sides: dict[str, set[str]] = {}

    for source in sources:
        name = source.get("name")
        if not isinstance(name, str) or not TAG_PATTERN.fullmatch(name):
            raise BuildError(f"invalid source name: {name!r}")
        if name in seen_names:
            raise BuildError(f"duplicate source name: {name}")
        seen_names.add(name)

        declared = source.get("sides")
        if declared is not None:
            if not isinstance(declared, list) or not all(
                side in {"geosite", "geoip"} for side in declared
            ):
                raise BuildError(f"{name}: sides must contain only geosite/geoip")
            if len(set(declared)) != len(declared):
                raise BuildError(f"{name}: duplicate declared side")
            declared_sides[name] = set(declared)

        if name == "applications":
            print("  · skip applications")
            continue

        try:
            content = fetcher(source["url"])
            buckets, raw_count, skipped = parse_source_content(source, content)
        except Exception as error:
            raise BuildError(f"{name}: fetch/parse failed: {error}") from error

        if is_applications_source(source, buckets, skipped):
            print(f"  · skip {name} (process-only)")
            continue

        metadata = emit_source_files(name, buckets, data_dir, ip_dir)
        actual_sides = {
            side
            for side, present in (
                ("geosite", metadata["geosite"]),
                ("geoip", metadata["geoip"]),
            )
            if present
        }
        if name in declared_sides and actual_sides != declared_sides[name]:
            raise BuildError(
                f"{name}: emitted sides {sorted(actual_sides)} do not match "
                f"declared sides {sorted(declared_sides[name])}"
            )
        behavior = source["behavior"]
        if behavior in {"domain", "classical"} and not metadata["geosite"]:
            raise BuildError(f"{name}: expected domain tag is empty")
        if behavior == "ipcidr" and not metadata["geoip"]:
            raise BuildError(f"{name}: expected IP tag is empty")

        if metadata["geosite"]:
            expected_site.add(name)
        if metadata["geoip"]:
            expected_ip.add(name)

        print(
            f"  ✓ {name:30s} raw={raw_count:6d} "
            f"domain={metadata['domain_count']:6d} ip={metadata['ip_count']:6d} "
            f"skipped={len(skipped):4d}"
        )

    all_names = set(seen_names) | {"applications"}
    required_site = expected_site | {"cn"}
    required_ip = expected_ip | {"cn", "private"}
    return {
        "schema_version": 1,
        "geosite": sorted(expected_site),
        "geoip": sorted(expected_ip),
        "required": {
            "geosite": sorted(required_site),
            "geoip": sorted(required_ip),
        },
        "forbidden": {
            "geosite": sorted(all_names - required_site),
            "geoip": sorted(all_names - required_ip),
        },
    }


def reset_output_directories() -> None:
    for path in (BUILD, PUBLISH):
        if path.exists():
            shutil.rmtree(path)
    CUSTOM_DATA.mkdir(parents=True)
    IP_DIR.mkdir(parents=True)
    PUBLISH.mkdir(parents=True)


def run_command(command: list[str], *, cwd: Path | None = None) -> None:
    result = subprocess.run(command, cwd=cwd, check=False)
    if result.returncode != 0:
        raise BuildError(f"command failed ({result.returncode}): {' '.join(command)}")


def compile_geosite() -> None:
    tool_directory = ROOT / "vendor" / "domain-list-custom"
    if not tool_directory.is_dir():
        raise BuildError("vendor/domain-list-custom is missing")
    run_command(
        [
            "go",
            "run",
            ".",
            f"--datapath={MERGED}",
            "--datname=geosite.dat",
            f"--outputpath={PUBLISH}",
            "--exportlists=",
            "--togfwlist=",
        ],
        cwd=tool_directory,
    )
    for extra in PUBLISH.glob("*.txt"):
        extra.unlink()


def compile_geoip(config_path: Path) -> None:
    installed_tool = shutil.which("geoip")
    vendored_binary = ROOT / "vendor" / "bin" / "geoip"
    source_directory = ROOT / "vendor" / "geoip"
    if installed_tool:
        command = [installed_tool, "convert", "-c", str(config_path)]
        run_command(command)
    elif vendored_binary.is_file():
        run_command([str(vendored_binary), "convert", "-c", str(config_path)])
    elif source_directory.is_dir():
        run_command(
            ["go", "run", ".", "convert", "-c", str(config_path)],
            cwd=source_directory,
        )
    else:
        raise BuildError("Loyalsoldier geoip tool is missing")


def finalize_assets() -> None:
    for filename in ("geosite.dat", "geoip.dat"):
        path = PUBLISH / filename
        if not path.is_file() or path.stat().st_size == 0:
            raise BuildError(f"missing or empty publish asset: {filename}")
        write_sha256(path)

    actual_assets = {path.name for path in PUBLISH.iterdir() if path.is_file()}
    if actual_assets != RELEASE_ASSETS:
        raise BuildError(
            f"unexpected publish assets: {sorted(actual_assets - RELEASE_ASSETS)}"
        )


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--skip-compile", action="store_true")
    parser.add_argument("--sources", type=Path, default=ROOT / "sources.yaml")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    reset_output_directories()
    try:
        expected = emit_sources(load_sources(args.sources), fetch, CUSTOM_DATA, IP_DIR)
        if COMMUNITY.is_dir():
            assert_no_name_collision(set(expected["geosite"]), COMMUNITY)
            merge_data_dirs(COMMUNITY, CUSTOM_DATA, MERGED)
        elif args.skip_compile:
            merge_data_dirs(Path("/nonexistent"), CUSTOM_DATA, MERGED)
        else:
            raise BuildError("vendor/domain-list-community/data is missing")

        expected_path = BUILD / "expected_tags.json"
        expected_path.write_text(
            json.dumps(expected, indent=2) + "\n",
            encoding="utf-8",
        )
        config_path = BUILD / "geoip-config.json"
        build_geoip_config(
            GEOIP_BASE,
            IP_DIR,
            config_path,
            PUBLISH,
            template_path=ROOT / "config" / "geoip.base.json",
        )

        if args.skip_compile:
            print("skip-compile: emit done")
            return 0

        compile_geosite()
        compile_geoip(config_path)
        finalize_assets()
        print("publish:", sorted(RELEASE_ASSETS))
        return 0
    except BuildError as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
