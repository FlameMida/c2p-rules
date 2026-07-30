from pathlib import Path

import yaml

from lib.buckets import classify_domain, classify_rule, empty_buckets
from lib.dlc_emit import buckets_to_dlc_lines, buckets_to_ip_lines


def parse_source_content(
    source: dict,
    content: str,
) -> tuple[dict[str, list[str]], int, list[str]]:
    source_format = source.get("format", "yaml")
    behavior = source["behavior"]
    if source_format not in {"yaml", "text"}:
        raise ValueError(f"unsupported source format: {source_format}")
    if behavior not in {"domain", "ipcidr", "classical"}:
        raise ValueError(f"unsupported source behavior: {behavior}")
    buckets = empty_buckets()
    skipped = []

    if source_format == "yaml":
        document = yaml.safe_load(content)
        if document is None:
            document = {}
        if not isinstance(document, dict):
            raise ValueError("YAML root must be a mapping")
        items = document.get("payload")
        if items is None:
            items = []
        if not isinstance(items, list):
            raise ValueError("YAML payload must be a list")
        if any(not isinstance(item, str) for item in items):
            raise ValueError("YAML payload items must be strings")
    else:
        items = [
            line
            for line in content.splitlines()
            if line.strip() and not line.strip().startswith("#")
        ]

    for item in items:
        value = item
        if behavior == "domain":
            classify_domain(value, buckets)
        elif behavior == "ipcidr":
            value = value.strip().strip("'\"")
            if value:
                buckets["ip_cidr"].append(value)
        elif behavior == "classical":
            classify_rule(value, buckets, skipped)

    return buckets, len(items), skipped


def emit_source_files(
    name: str,
    buckets: dict[str, list[str]],
    data_dir: Path,
    ip_dir: Path,
) -> dict[str, bool | int]:
    domain_lines = buckets_to_dlc_lines(buckets)
    ip_lines = buckets_to_ip_lines(buckets)
    metadata = {
        "geosite": bool(domain_lines),
        "geoip": bool(ip_lines),
        "domain_count": len(domain_lines),
        "ip_count": len(ip_lines),
    }

    if domain_lines:
        data_dir.mkdir(parents=True, exist_ok=True)
        (data_dir / name).write_text(
            "\n".join(domain_lines) + "\n",
            encoding="utf-8",
        )
    if ip_lines:
        ip_dir.mkdir(parents=True, exist_ok=True)
        (ip_dir / f"{name}.txt").write_text(
            "\n".join(ip_lines) + "\n",
            encoding="utf-8",
        )

    return metadata
