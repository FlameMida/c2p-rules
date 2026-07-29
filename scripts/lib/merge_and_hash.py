import hashlib
import json
import shutil
import sys
from pathlib import Path


def assert_no_name_collision(custom_names: set[str], community_data: Path) -> None:
    if not community_data.is_dir():
        return
    community_names = {path.name for path in community_data.iterdir() if path.is_file()}
    collisions = custom_names & community_names
    if collisions:
        print(
            f"ERROR: custom list names collide with community data: {sorted(collisions)}",
            file=sys.stderr,
        )
        raise SystemExit(2)


def merge_data_dirs(community: Path, custom: Path, output: Path) -> None:
    if output.exists():
        shutil.rmtree(output)
    output.mkdir(parents=True)
    for source in (community, custom):
        if not source.is_dir():
            continue
        for path in source.iterdir():
            if path.is_file():
                shutil.copy2(path, output / path.name)


def write_sha256(path: Path) -> Path:
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    checksum = path.with_name(path.name + ".sha256sum")
    checksum.write_text(f"{digest}  {path.name}\n", encoding="utf-8")
    return checksum


def build_geoip_config(
    base_dat_uri: str,
    ip_dir: Path,
    output_json: Path,
    publish_dir: Path,
) -> None:
    inputs = [
        {
            "type": "v2rayGeoIPDat",
            "action": "add",
            "args": {"uri": base_dat_uri},
        }
    ]
    if ip_dir.is_dir():
        for path in sorted(ip_dir.glob("*.txt")):
            inputs.append(
                {
                    "type": "text",
                    "action": "add",
                    "args": {"name": path.stem, "uri": str(path.resolve())},
                }
            )

    config = {
        "input": inputs,
        "output": [
            {
                "type": "v2rayGeoIPDat",
                "action": "output",
                "args": {
                    "outputDir": str(publish_dir.resolve()),
                    "outputName": "geoip.dat",
                },
            }
        ],
    }
    output_json.write_text(json.dumps(config, indent=2) + "\n", encoding="utf-8")
