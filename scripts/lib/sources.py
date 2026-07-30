from pathlib import Path
import re

import yaml


def load_sources(path: Path) -> list[dict]:
    document = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    return list(document.get("sources") or [])


def is_applications_source(
    source: dict,
    buckets: dict[str, list[str]] | None = None,
    skipped: list[str] | None = None,
) -> bool:
    if source.get("name") == "applications":
        return True
    if buckets is None or skipped is None:
        return False
    return (
        sum(len(values) for values in buckets.values()) == 0
        and bool(skipped)
        and all(
            re.fullmatch(r"PROCESS(?:-[A-Z0-9]+)+", rule.strip().split(",", 1)[0])
            for rule in skipped
        )
    )
