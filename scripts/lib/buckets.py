import re


BUCKET_KEYS = (
    "domain",
    "domain_suffix",
    "domain_keyword",
    "domain_regex",
    "ip_cidr",
)


def empty_buckets() -> dict[str, list[str]]:
    return {key: [] for key in BUCKET_KEYS}


def glob_to_regex(glob: str) -> str:
    output = ["^"]
    for character in glob:
        if character == "*":
            output.append("[^.]*")
        elif character == "?":
            output.append("[^.]")
        else:
            output.append(re.escape(character))
    output.append("$")
    return "".join(output)


def classify_domain(value: str, buckets: dict[str, list[str]]) -> None:
    value = str(value).strip().strip("'\"")
    if not value:
        return
    if value.startswith(("+.", "*.")):
        buckets["domain_suffix"].append(value[2:])
    elif "*" in value or "?" in value:
        buckets["domain_regex"].append(glob_to_regex(value))
    else:
        buckets["domain"].append(value)


def classify_rule(
    line: str,
    buckets: dict[str, list[str]],
    skipped: list[str],
) -> None:
    parts = [part.strip() for part in line.split(",")]
    if not parts or not parts[0]:
        return

    rule_type = parts[0]
    value = parts[1] if len(parts) > 1 else ""
    target = {
        "DOMAIN": "domain",
        "DOMAIN-SUFFIX": "domain_suffix",
        "DOMAIN-KEYWORD": "domain_keyword",
        "DOMAIN-REGEX": "domain_regex",
        "IP-CIDR": "ip_cidr",
        "IP-CIDR6": "ip_cidr",
        "IP-SUFFIX": "ip_cidr",
    }.get(rule_type)

    if target is not None:
        buckets[target].append(value)
    elif not rule_type.startswith("#"):
        skipped.append(line)
