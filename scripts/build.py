#!/usr/bin/env python3
"""
clash-rules-srs builder
把 sources.yaml 里每个 Clash 规则源 1:1 转成 sing-box .srs。

流程：拉源 → 按 format(yaml|text)/behavior(domain|ipcidr|classical) 解析 →
归一化到五桶(domain/domain_suffix/domain_keyword/domain_regex/ip_cidr) →
生成 sing-box source JSON(每类一个 rule，rule 间 OR) → sing-box rule-set compile。
"""
import json
import re
import subprocess
import sys
import urllib.request
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
DIST = ROOT / "dist"
UA = "Mozilla/5.0 (compatible; clash-rules-srs-builder/1.0)"


def glob_to_regex(g):
    """Clash 通配域名 → sing-box domain_regex（锚定）。* → [^.]*，? → [^.]。"""
    out = ["^"]
    for ch in g:
        if ch == "*":
            out.append("[^.]*")
        elif ch == "?":
            out.append("[^.]")
        else:
            out.append(re.escape(ch))
    out.append("$")
    return "".join(out)


def classify_domain(s, buckets):
    """Clash domain-behavior payload 的一项 → 桶。"""
    s = str(s).strip().strip("'\"")
    if not s:
        return
    if s.startswith("+.") or s.startswith("*."):
        buckets["domain_suffix"].append(s[2:])  # +.foo / *.foo → 后缀 foo
    elif "*" in s or "?" in s:
        buckets["domain_regex"].append(glob_to_regex(s))
    else:
        buckets["domain"].append(s)  # 精确域名


def classify_rule(line, buckets, skipped):
    """classical/text 的一行规则(TYPE,value[,attr...]) → 桶；attr 如 no-resolve 忽略。"""
    parts = [p.strip() for p in line.split(",")]
    if not parts or not parts[0]:
        return
    t = parts[0]
    val = parts[1] if len(parts) > 1 else ""
    if t == "DOMAIN":
        buckets["domain"].append(val)
    elif t == "DOMAIN-SUFFIX":
        buckets["domain_suffix"].append(val)
    elif t == "DOMAIN-KEYWORD":
        buckets["domain_keyword"].append(val)
    elif t == "DOMAIN-REGEX":
        buckets["domain_regex"].append(val)
    elif t in ("IP-CIDR", "IP-CIDR6", "IP-SUFFIX"):
        buckets["ip_cidr"].append(val)
    elif t.startswith("#"):
        return
    else:
        skipped.append(line)  # PROCESS-NAME / 逻辑规则 / 其他不支持


def parse_source(src):
    """拉源并解析。返回 (buckets, raw_count, skipped)。"""
    url = src["url"]
    fmt = src.get("format", "yaml")
    behavior = src["behavior"]
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    content = urllib.request.urlopen(req, timeout=60).read().decode("utf-8")

    buckets = {k: [] for k in ("domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr")}
    skipped = []

    if fmt == "yaml":
        data = yaml.safe_load(content) or {}
        items = data.get("payload", []) or []
        if behavior == "domain":
            for it in items:
                classify_domain(it, buckets)
        elif behavior == "ipcidr":
            for it in items:
                s = str(it).strip().strip("'\"")
                if s:
                    buckets["ip_cidr"].append(s)
        elif behavior == "classical":
            for it in items:
                classify_rule(str(it), buckets, skipped)
        raw_count = len(items)
    else:  # text
        lines = [l for l in content.splitlines() if l.strip() and not l.strip().startswith("#")]
        if behavior == "classical":
            for l in lines:
                classify_rule(l, buckets, skipped)
        elif behavior == "domain":
            for l in lines:
                classify_domain(l, buckets)
        elif behavior == "ipcidr":
            for l in lines:
                s = l.strip().strip("'\"")
                if s:
                    buckets["ip_cidr"].append(s)
        raw_count = len(lines)

    return buckets, raw_count, skipped


def to_source_json(buckets, version=2):
    """五桶 → sing-box source JSON。每类一个 rule（单 rule 多字段是 AND，故分类）。"""
    rules = []
    if buckets["domain_suffix"]:
        rules.append({"domain_suffix": buckets["domain_suffix"]})
    if buckets["domain"]:
        rules.append({"domain": buckets["domain"]})
    if buckets["domain_keyword"]:
        rules.append({"domain_keyword": buckets["domain_keyword"]})
    if buckets["domain_regex"]:
        rules.append({"domain_regex": buckets["domain_regex"]})
    if buckets["ip_cidr"]:
        rules.append({"ip_cidr": buckets["ip_cidr"]})
    return {"version": version, "rules": rules}


def main():
    DIST.mkdir(exist_ok=True)
    sources = yaml.safe_load((ROOT / "sources.yaml").read_text(encoding="utf-8"))["sources"]
    ok, fail = 0, 0
    for src in sources:
        name = src["name"]
        try:
            buckets, raw_count, skipped = parse_source(src)
            sj = to_source_json(buckets)
            json_path = DIST / f"{name}.json"
            srs_path = DIST / f"{name}.srs"
            json_path.write_text(json.dumps(sj, ensure_ascii=False), encoding="utf-8")
            res = subprocess.run(
                ["sing-box", "rule-set", "compile", str(json_path), "-o", str(srs_path)],
                check=False, capture_output=True, text=True,
            )
            if res.returncode != 0:
                raise RuntimeError(f"sing-box compile 失败: {res.stderr.strip()[:200]}")
            total = sum(len(v) for v in buckets.values())
            sk = f"  skipped={len(skipped)}" if skipped else ""
            print(f"  ✓ {name:30s} raw={raw_count:6d} → {total:6d}{sk}")
            ok += 1
        except Exception as e:
            print(f"  ✗ {name:30s} {type(e).__name__}: {e}")
            fail += 1
    print(f"\n完成: {ok} 成功, {fail} 失败。输出目录: {DIST}")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
