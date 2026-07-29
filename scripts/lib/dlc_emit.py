def buckets_to_dlc_lines(buckets: dict[str, list[str]]) -> list[str]:
    lines = []
    lines.extend(f"domain:{value}" for value in buckets.get("domain_suffix", []))
    lines.extend(f"full:{value}" for value in buckets.get("domain", []))
    lines.extend(f"keyword:{value}" for value in buckets.get("domain_keyword", []))
    lines.extend(f"regexp:{value}" for value in buckets.get("domain_regex", []))
    return lines


def buckets_to_ip_lines(buckets: dict[str, list[str]]) -> list[str]:
    return list(buckets.get("ip_cidr", []))
