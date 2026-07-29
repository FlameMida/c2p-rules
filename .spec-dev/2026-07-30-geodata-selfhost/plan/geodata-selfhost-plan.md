# 自建 geodata 全链路 实施计划

> **执行方式**：使用 spec-dev 的 executing-plans skill 逐任务执行本计划；无该 skill 的环境直接从任务 0 起按序执行至最终任务。步骤用复选框（`- [ ]`）语法跟踪；脱离项目携带时连同特性目录（含 spec）整体带走。
>
> **偏差处理**：执行中发现计划与现实不符——小偏差（路径笔误、明显遗漏但意图清楚）就地修正并在提交信息中注明；接口、数据结构等契约级偏差停下向计划作者确认，不猜着改。

**目标**：把 `clash-rules-srs` 重构为产出 `geosite.dat`/`geoip.dat`（轻量完整增强底 + sources 自定义 tag）并经 GitHub Releases 发布；同时为 `clash2passwall` 增加 `--dat` 映射与安装脚本，供 PassWall2 双核消费。

**Spec**：`.spec-dev/2026-07-30-geodata-selfhost/spec/geodata-selfhost-design.md`（`status: active`）

**架构**：Python 拉 sources → 五桶 → dlc 文本 + IP txt → domain-list-custom（community∪自定义）→ geosite.dat；geoip convert（官方 dat 底 + 自定义 CIDR）→ geoip.dat → sha256 → Releases。下游 `clash2passwall --dat` 与 install 脚本写 UCI URL 并覆盖导入 shunt_rules。

**技术栈**：Python 3.11+、PyYAML、Go（domain-list-custom / Loyalsoldier-geoip）、GitHub Actions、Node.js（clash2passwall 零依赖）

## 全局约束

- 发布资产仅：`geosite.dat`、`geoip.dat`、`geosite.dat.sha256sum`、`geoip.dat.sha256sum`；禁止将 `.srs` 作为 Release 资产。
- 自定义 tag = `sources.yaml` 的 `name`；dlc 行必须带前缀 `domain:`/`full:`/`keyword:`/`regexp:`。
- geosite 底 = v2fly/domain-list-community `data/`；geoip 底 = Loyalsoldier/geoip 的 `geoip.dat`；不自备 MaxMind。
- sha256 行格式：`64hex` + 两个空格 + 纯文件名 + `\n`；latest URL：`.../releases/latest/download/<file>`。
- 任一启用源失败 / 底包失败 / 撞名 / 期望 tag 空 → 非零退出且不更新 latest。
- 切片：A 构建发布（任务 1–5）、B 转换（任务 6–7）、C 安装与文档（任务 8–9）。
- 仓库根：`/Users/flame/clash-rules-srs`；转换器：`/Users/flame/clash2passwall`。

---

### 任务 0：建立隔离工作区

- [x] **步骤 1：检测已有隔离**

运行：`git rev-parse --git-dir` 与 `git rev-parse --git-common-dir`  
两者不同、且 `git rev-parse --show-superproject-working-tree` 无输出 → 已在隔离工作区，跳过本任务。

- [x] **步骤 2：建立 worktree**

```bash
cd /Users/flame/clash-rules-srs
git check-ignore -q .worktrees || { echo '.worktrees/' >> .gitignore; git add .gitignore; git commit -m 'chore: ignore .worktrees'; }
git worktree add .worktrees/plan-geodata-selfhost -b plan/2026-07-30-geodata-selfhost
cd .worktrees/plan-geodata-selfhost
```

- [x] **步骤 3：安装依赖并验证基线**

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
# 基线：尚无测试套件时，至少确认 import yaml 成功
.venv/bin/python -c "import yaml; print('ok')"
```

预期：打印 `ok`。若失败 → 停下报告。

---

## 切片 A：构建与发布契约

### 任务 1：五桶解析与 dlc 行发射（纯函数 + 单元测试）

**文件**：
- 创建：`scripts/lib/__init__.py`（空）
- 创建：`scripts/lib/buckets.py`
- 创建：`scripts/lib/dlc_emit.py`
- 创建：`tests/test_buckets_dlc.py`
- 修改：`requirements.txt`（若需；当前 `pyyaml>=6.0` 足够）

**接口**：
- 消费：无
- 产出：
  - `empty_buckets() -> dict[str, list[str]]` 键：`domain,domain_suffix,domain_keyword,domain_regex,ip_cidr`
  - `classify_domain(s: str, buckets: dict) -> None`
  - `classify_rule(line: str, buckets: dict, skipped: list) -> None`
  - `glob_to_regex(g: str) -> str`
  - `buckets_to_dlc_lines(buckets: dict) -> list[str]` 仅域名侧，带前缀
  - `buckets_to_ip_lines(buckets: dict) -> list[str]`

- [x] **步骤 1：写失败测试**

```python
# tests/test_buckets_dlc.py
import unittest
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.buckets import empty_buckets, classify_domain, classify_rule, glob_to_regex
from lib.dlc_emit import buckets_to_dlc_lines, buckets_to_ip_lines

class TestDomainBehavior(unittest.TestCase):
    def test_suffix_and_exact_prefixed(self):
        # Scenario: domain-behavior 后缀与精确
        b = empty_buckets()
        classify_domain("+.example.com", b)
        classify_domain("www.example.com", b)
        lines = buckets_to_dlc_lines(b)
        self.assertIn("domain:example.com", lines)
        self.assertIn("full:www.example.com", lines)
        self.assertNotIn("example.com", lines)  # 禁止裸后缀行

    def test_keyword_classical(self):
        # Scenario: classical 关键词
        b = empty_buckets()
        sk = []
        classify_rule("DOMAIN-KEYWORD,openai", b, sk)
        lines = buckets_to_dlc_lines(b)
        self.assertIn("keyword:openai", lines)

    def test_process_name_skipped(self):
        b = empty_buckets()
        sk = []
        classify_rule("PROCESS-NAME,Chrome", b, sk)
        self.assertEqual(len(sk), 1)
        self.assertEqual(sum(len(v) for v in b.values()), 0)

    def test_netflix_split_buckets(self):
        # 为 Netflix 拆分准备桶
        b = empty_buckets()
        sk = []
        classify_rule("DOMAIN-SUFFIX,netflix.com", b, sk)
        classify_rule("IP-CIDR,23.246.0.0/18,no-resolve", b, sk)
        self.assertIn("netflix.com", b["domain_suffix"])
        self.assertIn("23.246.0.0/18", b["ip_cidr"])
        dlc = buckets_to_dlc_lines(b)
        ips = buckets_to_ip_lines(b)
        self.assertIn("domain:netflix.com", dlc)
        self.assertEqual(ips, ["23.246.0.0/18"])

    def test_glob_regex_anchored(self):
        r = glob_to_regex("foo*bar")
        self.assertTrue(r.startswith("^") and r.endswith("$"))

if __name__ == "__main__":
    unittest.main()
```

- [x] **步骤 2：运行测试确认失败**

```bash
.venv/bin/python -m unittest tests.test_buckets_dlc -v
```

预期：FAIL，`ModuleNotFoundError` 或 import 失败。

- [x] **步骤 3：写最小实现**

```python
# scripts/lib/__init__.py
# empty

# scripts/lib/buckets.py
import re

BUCKET_KEYS = ("domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr")

def empty_buckets():
    return {k: [] for k in BUCKET_KEYS}

def glob_to_regex(g: str) -> str:
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

def classify_domain(s: str, buckets: dict) -> None:
    s = str(s).strip().strip("'\"")
    if not s:
        return
    if s.startswith("+.") or s.startswith("*."):
        buckets["domain_suffix"].append(s[2:])
    elif "*" in s or "?" in s:
        buckets["domain_regex"].append(glob_to_regex(s))
    else:
        buckets["domain"].append(s)

def classify_rule(line: str, buckets: dict, skipped: list) -> None:
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
        skipped.append(line)

# scripts/lib/dlc_emit.py
def buckets_to_dlc_lines(buckets: dict) -> list:
    lines = []
    for v in buckets.get("domain_suffix", []):
        lines.append(f"domain:{v}")
    for v in buckets.get("domain", []):
        lines.append(f"full:{v}")
    for v in buckets.get("domain_keyword", []):
        lines.append(f"keyword:{v}")
    for v in buckets.get("domain_regex", []):
        lines.append(f"regexp:{v}")
    return lines

def buckets_to_ip_lines(buckets: dict) -> list:
    return list(buckets.get("ip_cidr", []))
```

- [x] **步骤 4：运行测试确认通过**

```bash
.venv/bin/python -m unittest tests.test_buckets_dlc -v
```

预期：全部 PASS。

- [x] **步骤 5：提交**

```bash
git add scripts/lib tests/test_buckets_dlc.py
git commit -m "feat(T1): add bucket classify and prefixed dlc emit"
```

---

### 任务 2：源加载、拉取与写 build 树

**文件**：
- 创建：`scripts/lib/sources.py`
- 创建：`scripts/lib/fetch_emit.py`
- 创建：`tests/test_sources_emit.py`
- 修改：可引用 `sources.yaml` 只读

**接口**：
- 消费：`buckets` / `dlc_emit`
- 产出：
  - `load_sources(path: Path) -> list[dict]`
  - `is_applications_source(src: dict, buckets: dict, skipped: list) -> bool`
  - `parse_source_content(src: dict, content: str) -> tuple[dict, int, list]`
  - `emit_source_files(name, buckets, data_dir: Path, ip_dir: Path) -> dict` 返回 `{"geosite": bool, "geoip": bool, "domain_count": int, "ip_count": int}`

- [x] **步骤 1：写失败测试**

```python
# tests/test_sources_emit.py
import unittest
import tempfile
from pathlib import Path
import sys
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.sources import load_sources, is_applications_source
from lib.fetch_emit import parse_source_content, emit_source_files
from lib.buckets import empty_buckets

ROOT = Path(__file__).resolve().parents[1]

class TestSources(unittest.TestCase):
    def test_load_sources_yaml(self):
        srcs = load_sources(ROOT / "sources.yaml")
        names = {s["name"] for s in srcs}
        self.assertIn("loyalsoldier-gfw", names)
        self.assertIn("xiaolin-netflix", names)
        self.assertNotIn("applications", names)  # 清单中无 applications 条目

    def test_parse_yaml_domain_payload(self):
        content = "payload:\n  - '+.example.com'\n  - 'www.example.com'\n"
        src = {"name": "t", "behavior": "domain", "format": "yaml"}
        buckets, raw, skipped = parse_source_content(src, content)
        self.assertEqual(raw, 2)
        self.assertIn("example.com", buckets["domain_suffix"])

    def test_emit_youtube_no_geoip_file(self):
        # Scenario: YouTube 仅域名 → 不写 ip 文件
        b = empty_buckets()
        b["domain_suffix"].append("youtube.com")
        with tempfile.TemporaryDirectory() as td:
            data_dir = Path(td) / "data"
            ip_dir = Path(td) / "ip"
            data_dir.mkdir(); ip_dir.mkdir()
            meta = emit_source_files("xiaolin-youtube", b, data_dir, ip_dir)
            self.assertTrue(meta["geosite"])
            self.assertFalse(meta["geoip"])
            self.assertTrue((data_dir / "xiaolin-youtube").is_file())
            self.assertFalse((ip_dir / "xiaolin-youtube.txt").exists())

    def test_emit_netflix_both(self):
        b = empty_buckets()
        b["domain_suffix"].append("netflix.com")
        b["ip_cidr"].append("23.246.0.0/18")
        with tempfile.TemporaryDirectory() as td:
            data_dir = Path(td) / "data"
            ip_dir = Path(td) / "ip"
            data_dir.mkdir(); ip_dir.mkdir()
            meta = emit_source_files("xiaolin-netflix", b, data_dir, ip_dir)
            self.assertTrue(meta["geosite"] and meta["geoip"])
            self.assertIn("domain:netflix.com", (data_dir / "xiaolin-netflix").read_text())
            self.assertEqual((ip_dir / "xiaolin-netflix.txt").read_text().strip(), "23.246.0.0/18")

if __name__ == "__main__":
    unittest.main()
```

- [x] **步骤 2：运行确认失败**

```bash
.venv/bin/python -m unittest tests.test_sources_emit -v
```

- [x] **步骤 3：最小实现**

```python
# scripts/lib/sources.py
from pathlib import Path
import yaml

def load_sources(path: Path) -> list:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    return list(data.get("sources") or [])

def is_applications_source(src: dict, buckets: dict | None = None, skipped: list | None = None) -> bool:
    if src.get("name") == "applications":
        return True
    if skipped is not None and buckets is not None:
        # 解析后仅进程类：无任何桶条目且 skipped 全为 PROCESS-NAME
        if sum(len(v) for v in buckets.values()) == 0 and skipped:
            return all(s.startswith("PROCESS-NAME") for s in skipped)
    return False

# scripts/lib/fetch_emit.py
import yaml
from pathlib import Path
from lib.buckets import empty_buckets, classify_domain, classify_rule
from lib.dlc_emit import buckets_to_dlc_lines, buckets_to_ip_lines
from lib.sources import is_applications_source

def parse_source_content(src: dict, content: str):
    fmt = src.get("format", "yaml")
    behavior = src["behavior"]
    buckets = empty_buckets()
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
    else:
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

def emit_source_files(name: str, buckets: dict, data_dir: Path, ip_dir: Path) -> dict:
    dlc = buckets_to_dlc_lines(buckets)
    ips = buckets_to_ip_lines(buckets)
    meta = {"geosite": False, "geoip": False, "domain_count": len(dlc), "ip_count": len(ips)}
    if dlc:
        data_dir.mkdir(parents=True, exist_ok=True)
        (data_dir / name).write_text("\n".join(dlc) + "\n", encoding="utf-8")
        meta["geosite"] = True
    if ips:
        ip_dir.mkdir(parents=True, exist_ok=True)
        (ip_dir / f"{name}.txt").write_text("\n".join(ips) + "\n", encoding="utf-8")
        meta["geoip"] = True
    return meta
```

- [x] **步骤 4：运行确认通过**

```bash
.venv/bin/python -m unittest tests.test_sources_emit -v
```

- [x] **步骤 5：提交**

```bash
git add scripts/lib/sources.py scripts/lib/fetch_emit.py tests/test_sources_emit.py
git commit -m "feat(T2): load sources and emit dlc/ip tree"
```

---

### 任务 3：编排 build.py（拉源、合并 community、fail-fast、sha256、geoip-config）

**文件**：
- 重写：`scripts/build.py`
- 创建：`scripts/lib/merge_and_hash.py`
- 创建：`config/geoip.base.json`（模板，uri 由 build 重写）
- 创建：`tests/test_merge_hash.py`

**接口**：
- 消费：任务 1–2
- 产出：
  - `assert_no_name_collision(custom_names: set[str], community_data: Path) -> None` 冲突则 raise
  - `merge_data_dirs(community: Path, custom: Path, out: Path) -> None`
  - `write_sha256(path: Path) -> Path` 写 `path.name + ".sha256sum"` 同目录
  - `build_geoip_config(base_dat_uri: str, ip_dir: Path, out_json: Path, publish_dir: Path) -> None`
  - CLI：`python scripts/build.py [--skip-compile]`  
    默认：拉源 → emit → 若存在 `vendor/domain-list-community/data` 则 merge → 写 geoip-config → 若未 `--skip-compile` 且工具在 PATH 则 compile；始终在 emit 后校验非空期望

- [x] **步骤 1：写失败测试**

```python
# tests/test_merge_hash.py
import unittest
import tempfile
import hashlib
from pathlib import Path
import sys
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.merge_and_hash import assert_no_name_collision, merge_data_dirs, write_sha256, build_geoip_config

class TestMergeHash(unittest.TestCase):
    def test_collision_raises(self):
        # Scenario: 自定义与 community 撞名
        with tempfile.TemporaryDirectory() as td:
            c = Path(td) / "comm"; c.mkdir()
            (c / "cn").write_text("domain:x\n")
            with self.assertRaises(SystemExit):
                assert_no_name_collision({"cn"}, c)

    def test_sha256_two_spaces_basename(self):
        # Scenario: sha256 伴随文件可被标准工具校验
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "geosite.dat"
            p.write_bytes(b"abc")
            sp = write_sha256(p)
            line = sp.read_text()
            h = hashlib.sha256(b"abc").hexdigest()
            self.assertEqual(line, f"{h}  geosite.dat\n")

    def test_geoip_config_lists_tags(self):
        with tempfile.TemporaryDirectory() as td:
            ip = Path(td) / "ip"; ip.mkdir()
            (ip / "loyalsoldier-cncidr.txt").write_text("1.1.1.0/24\n")
            outj = Path(td) / "geoip-config.json"
            pub = Path(td) / "publish"; pub.mkdir()
            build_geoip_config("https://example.com/geoip.dat", ip, outj, pub)
            text = outj.read_text()
            self.assertIn("loyalsoldier-cncidr", text)
            self.assertIn("v2rayGeoIPDat", text)

if __name__ == "__main__":
    unittest.main()
```

- [x] **步骤 2：运行确认失败**

```bash
.venv/bin/python -m unittest tests.test_merge_hash -v
```

- [x] **步骤 3：最小实现**

```python
# scripts/lib/merge_and_hash.py
import hashlib
import json
import shutil
import sys
from pathlib import Path

def assert_no_name_collision(custom_names: set, community_data: Path) -> None:
    if not community_data.is_dir():
        return
    existing = {p.name for p in community_data.iterdir() if p.is_file()}
    hit = custom_names & existing
    if hit:
        print(f"ERROR: custom list names collide with community data: {sorted(hit)}", file=sys.stderr)
        raise SystemExit(2)

def merge_data_dirs(community: Path, custom: Path, out: Path) -> None:
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)
    if community.is_dir():
        for p in community.iterdir():
            if p.is_file():
                shutil.copy2(p, out / p.name)
    if custom.is_dir():
        for p in custom.iterdir():
            if p.is_file():
                shutil.copy2(p, out / p.name)

def write_sha256(path: Path) -> Path:
    h = hashlib.sha256(path.read_bytes()).hexdigest()
    out = path.with_name(path.name + ".sha256sum")
    out.write_text(f"{h}  {path.name}\n", encoding="utf-8")
    return out

def build_geoip_config(base_dat_uri: str, ip_dir: Path, out_json: Path, publish_dir: Path) -> None:
    inputs = [{
        "type": "v2rayGeoIPDat",
        "action": "add",
        "args": {"uri": base_dat_uri},
    }]
    if ip_dir.is_dir():
        for f in sorted(ip_dir.glob("*.txt")):
            inputs.append({
                "type": "text",
                "action": "add",
                "args": {"name": f.stem, "uri": str(f.resolve())},
            })
    cfg = {
        "input": inputs,
        "output": [{
            "type": "v2rayGeoIPDat",
            "action": "output",
            "args": {
                "outputDir": str(publish_dir.resolve()),
                "outputName": "geoip.dat",
            },
        }],
    }
    out_json.write_text(json.dumps(cfg, indent=2), encoding="utf-8")
```

```python
# scripts/build.py  — 完整编排（可执行）
#!/usr/bin/env python3
"""Emit dlc/ip trees from sources.yaml; optional compile to dat."""
from __future__ import annotations
import argparse
import shutil
import subprocess
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "scripts"))

from lib.sources import load_sources, is_applications_source
from lib.fetch_emit import parse_source_content, emit_source_files
from lib.merge_and_hash import (
    assert_no_name_collision, merge_data_dirs, write_sha256, build_geoip_config,
)

UA = "clash-rules-srs-builder/2.0-dat"
BUILD = ROOT / "build"
CUSTOM_DATA = BUILD / "data"
IP_DIR = BUILD / "ip"
MERGED = BUILD / "data-merged"
PUBLISH = ROOT / "publish"
COMMUNITY = ROOT / "vendor" / "domain-list-community" / "data"
GEOIP_BASE = "https://github.com/Loyalsoldier/geoip/releases/latest/download/geoip.dat"

def fetch(url: str) -> str:
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read().decode("utf-8")

def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--skip-compile", action="store_true")
    ap.add_argument("--sources", type=Path, default=ROOT / "sources.yaml")
    args = ap.parse_args()

    if BUILD.exists():
        shutil.rmtree(BUILD)
    CUSTOM_DATA.mkdir(parents=True)
    IP_DIR.mkdir(parents=True)
    PUBLISH.mkdir(parents=True, exist_ok=True)

    sources = load_sources(args.sources)
    expected_site: set[str] = set()
    expected_ip: set[str] = set()
    custom_names: set[str] = set()

    for src in sources:
        name = src["name"]
        if name == "applications":
            print(f"  · skip applications")
            continue
        try:
            content = fetch(src["url"])
            buckets, raw, skipped = parse_source_content(src, content)
        except Exception as e:
            print(f"  ✗ {name}: fetch/parse failed: {e}", file=sys.stderr)
            return 1
        if is_applications_source(src, buckets, skipped):
            print(f"  · skip {name} (process-only)")
            continue
        meta = emit_source_files(name, buckets, CUSTOM_DATA, IP_DIR)
        custom_names.add(name)
        print(f"  ✓ {name:30s} raw={raw:6d} domain={meta['domain_count']} ip={meta['ip_count']}")
        if meta["geosite"]:
            if meta["domain_count"] == 0:
                print(f"ERROR: empty geosite tag {name}", file=sys.stderr)
                return 1
            expected_site.add(name)
        if meta["geoip"]:
            if meta["ip_count"] == 0:
                print(f"ERROR: empty geoip tag {name}", file=sys.stderr)
                return 1
            expected_ip.add(name)
        # ipcidr 源必须有 geoip
        if src["behavior"] == "ipcidr" and not meta["geoip"]:
            print(f"ERROR: ipcidr source {name} produced no IP", file=sys.stderr)
            return 1
        # domain 源必须有 geosite
        if src["behavior"] == "domain" and not meta["geosite"]:
            print(f"ERROR: domain source {name} produced no domain rules", file=sys.stderr)
            return 1

    if COMMUNITY.is_dir():
        assert_no_name_collision(custom_names, COMMUNITY)
        merge_data_dirs(COMMUNITY, CUSTOM_DATA, MERGED)
    else:
        print("WARN: vendor community data missing; merge = custom only", file=sys.stderr)
        merge_data_dirs(Path("/nonexistent"), CUSTOM_DATA, MERGED)

    build_geoip_config(GEOIP_BASE, IP_DIR, BUILD / "geoip-config.json", PUBLISH)
    (BUILD / "expected_tags.json").write_text(
        __import__("json").dumps({"geosite": sorted(expected_site), "geoip": sorted(expected_ip)}, indent=2),
        encoding="utf-8",
    )

    if args.skip_compile:
        print("skip-compile: emit done")
        return 0

    # compile geosite
    dlc_dir = ROOT / "vendor" / "domain-list-custom"
    if not dlc_dir.is_dir():
        print("ERROR: vendor/domain-list-custom missing", file=sys.stderr)
        return 1
    r = subprocess.run(
        ["go", "run", ".", f"--datapath={MERGED}", "--datname=geosite.dat", f"--outputpath={PUBLISH}",
         "--exportlists=", "--togfwlist="],
        cwd=dlc_dir,
    )
    if r.returncode != 0:
        # 若空 exportlists 不被接受，回退默认再删 plaintext
        r = subprocess.run(
            ["go", "run", ".", f"--datapath={MERGED}", "--datname=geosite.dat", f"--outputpath={PUBLISH}"],
            cwd=dlc_dir,
        )
        if r.returncode != 0:
            return r.returncode

    # compile geoip
    r = subprocess.run(["geoip", "convert", "-c", str(BUILD / "geoip-config.json")])
    if r.returncode != 0:
        # 尝试 go run 路径
        geoip_src = ROOT / "vendor" / "geoip"
        if geoip_src.is_dir():
            r = subprocess.run(["go", "run", "./", "convert", "-c", str(BUILD / "geoip-config.json")], cwd=geoip_src)
        if r.returncode != 0:
            return r.returncode or 1

    for name in ("geosite.dat", "geoip.dat"):
        p = PUBLISH / name
        if not p.is_file():
            print(f"ERROR: missing {p}", file=sys.stderr)
            return 1
        write_sha256(p)

    print("publish:", list(PUBLISH.iterdir()))
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

- [x] **步骤 4：运行单元测试 + emit 烟雾**

```bash
.venv/bin/python -m unittest tests.test_merge_hash -v
.venv/bin/python scripts/build.py --skip-compile
```

预期：unittest PASS；`build/data/` 与 `build/ip/` 有文件；进程 exit 0（网络失败则按 fail-fast 非零——若网络不可用，用录制 fixture 测 emit 路径：实施者可临时把 `fetch` 换成读 `tests/fixtures/`，但默认按线上 URL）。

- [x] **步骤 5：提交**

```bash
git add scripts/build.py scripts/lib/merge_and_hash.py config tests/test_merge_hash.py
git commit -m "feat(T3): orchestrate emit merge hash and geoip-config"
```

---

### 任务 4：本地 vendor 工具链与完整 dat 构建 + 探针

**文件**：
- 创建：`scripts/probe_tags.py`
- 创建：`scripts/bootstrap_vendor.sh`
- 创建：`tests/test_sha256_contract.py`（若任务 3 已覆盖可合并）
- 修改：`.gitignore` 增加 `build/`、`publish/`、`vendor/`（或 vendor 用 submodule 说明）

**接口**：
- `probe_tags.py --dat publish/geosite.dat --expect build/expected_tags.json --side geosite`  
  实现策略：优先调用 `geoview`；若无，则用最小 protobuf 解析失败时改为「调用 domain-list-custom 导出 plain 不可用则」——**规范实现**：依赖 `geoview -type geosite -action unpack` 或  
  `go run github.com/v2fly/domain-list-community/main` 不可。  
  **本任务采用**：安装 `geoview` 或使用 Python 包若无则 shell 出：

```bash
# 探针最小实现：用 strings + 计数不可靠；改用 geoview convert 单 tag
geoview -type geosite -action convert -input publish/geosite.dat -list cn -output /tmp/cn.srs -lowmem=true
```

对每个 expected tag 跑 convert，失败或输出 0 字节 → exit 1。

- [x] **步骤 1：写 bootstrap 与探针脚本（先写探针失败路径测试：缺 dat 退出 1）**

```python
# scripts/probe_tags.py
#!/usr/bin/env python3
import argparse, json, subprocess, sys, tempfile
from pathlib import Path

def probe_one(geoview: str, typ: str, dat: Path, tag: str) -> bool:
    with tempfile.TemporaryDirectory() as td:
        out = Path(td) / "t.srs"
        r = subprocess.run(
            [geoview, "-type", typ, "-action", "convert", "-input", str(dat),
             "-list", tag, "-output", str(out), "-lowmem=true"],
            capture_output=True, text=True,
        )
        return r.returncode == 0 and out.is_file() and out.stat().st_size > 0

def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dat", type=Path, required=True)
    ap.add_argument("--expect", type=Path, required=True)
    ap.add_argument("--side", choices=["geosite", "geoip"], required=True)
    ap.add_argument("--geoview", default="geoview")
    ap.add_argument("--also", nargs="*", default=["cn"], help="extra tags e.g. cn")
    args = ap.parse_args()
    if not args.dat.is_file():
        print("missing dat", file=sys.stderr)
        return 1
    exp = json.loads(args.expect.read_text())
    tags = list(exp[args.side]) + list(args.also)
    typ = "geosite" if args.side == "geosite" else "geoip"
    bad = []
    for t in tags:
        ok = probe_one(args.geoview, typ, args.dat, t)
        print(("✓" if ok else "✗"), t)
        if not ok:
            bad.append(t)
    return 1 if bad else 0

if __name__ == "__main__":
    sys.exit(main())
```

```bash
# scripts/bootstrap_vendor.sh
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/vendor"
# community data
if [[ ! -d $ROOT/vendor/domain-list-community/.git ]]; then
  git clone --depth 1 https://github.com/v2fly/domain-list-community.git "$ROOT/vendor/domain-list-community"
else
  git -C "$ROOT/vendor/domain-list-community" pull --ff-only || true
fi
# domain-list-custom
if [[ ! -d $ROOT/vendor/domain-list-custom/.git ]]; then
  git clone --depth 1 https://github.com/Loyalsoldier/domain-list-custom.git "$ROOT/vendor/domain-list-custom"
fi
# geoip tool source
if [[ ! -d $ROOT/vendor/geoip/.git ]]; then
  git clone --depth 1 https://github.com/Loyalsoldier/geoip.git "$ROOT/vendor/geoip"
fi
# build geoip binary into PATH-local
( cd "$ROOT/vendor/geoip" && go build -o "$ROOT/vendor/bin/geoip" . )
mkdir -p "$ROOT/vendor/bin"
echo "export PATH=\"$ROOT/vendor/bin:\$PATH\""
```

- [x] **步骤 2：bootstrap + 全量构建**

```bash
chmod +x scripts/bootstrap_vendor.sh scripts/probe_tags.py
bash scripts/bootstrap_vendor.sh
export PATH="$PWD/vendor/bin:$PATH"
# 若本机无 geoview：brew install 或从 snowie2000/geoview 下二进制
.venv/bin/python scripts/build.py
.venv/bin/python scripts/probe_tags.py --dat publish/geosite.dat --expect build/expected_tags.json --side geosite
.venv/bin/python scripts/probe_tags.py --dat publish/geoip.dat --expect build/expected_tags.json --side geoip
sha256sum -c publish/geosite.dat.sha256sum
sha256sum -c publish/geoip.dat.sha256sum
```

预期：全部 exit 0；`cn` 与自定义 tag 探针通过。

- [x] **步骤 3：提交**

```bash
# 不要提交 vendor/ 大目录；提交脚本与 gitignore
echo -e 'build/\npublish/\nvendor/\n.venv/' >> .gitignore
git add scripts/probe_tags.py scripts/bootstrap_vendor.sh .gitignore
git commit -m "feat(T4): vendor bootstrap, full dat build hooks, tag probe"
```

---

### 任务 5：GitHub Actions 日更 + latest Release 四资产

**文件**：
- 重写：`.github/workflows/build.yml`
- 修改：`README.md`（任务 9 可再润色；本任务至少改订阅 URL 示例）

**接口**：无代码接口；契约 = Release assets 四文件 + `latest/download`。

- [ ] **步骤 1：写 workflow**

```yaml
# .github/workflows/build.yml
name: build-geodata

on:
  schedule:
    - cron: "17 2 * * *"
  workflow_dispatch:

permissions:
  contents: write

env:
  GO_VERSION: "1.22.x"

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - uses: actions/setup-python@v5
        with:
          python-version: "3.x"

      - name: Install geoview
        run: |
          set -e
          curl -fsSL -o geoview \
            "https://github.com/snowie2000/geoview/releases/latest/download/geoview-linux-amd64" \
            || curl -fsSL -o geoview \
            "https://github.com/snowie2000/geoview/releases/download/v0.1.10/geoview-linux-amd64"
          chmod +x geoview
          sudo mv geoview /usr/local/bin/geoview
          geoview -version || true

      - name: Bootstrap vendor
        run: bash scripts/bootstrap_vendor.sh

      - name: Build dat
        run: |
          python -m pip install -r requirements.txt
          export PATH="$PWD/vendor/bin:$PATH"
          python scripts/build.py
          python scripts/probe_tags.py --dat publish/geosite.dat --expect build/expected_tags.json --side geosite
          python scripts/probe_tags.py --dat publish/geoip.dat --expect build/expected_tags.json --side geoip
          sha256sum -c publish/geosite.dat.sha256sum
          sha256sum -c publish/geoip.dat.sha256sum

      - name: Publish to GitHub Release (rolling tag release)
        uses: softprops/action-gh-release@v2
        with:
          tag_name: release
          name: geodata release
          files: |
            publish/geosite.dat
            publish/geoip.dat
            publish/geosite.dat.sha256sum
            publish/geoip.dat.sha256sum
          fail_on_unmatched_files: true
          make_latest: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: publish
          path: publish/
```

- [ ] **步骤 2：本地校验 yaml**

```bash
# 若有 actionlint 则 actionlint .github/workflows/build.yml
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build.yml')); print('yaml ok')"
```

- [ ] **步骤 3：提交**

```bash
git add .github/workflows/build.yml
git commit -m "feat(T5): CI build geodata and publish latest release assets"
```

---

## 切片 B：clash2passwall `--dat`

### 任务 6：DAT_RULESET_MAP 与 --dat 模式映射测试

**文件**：
- 修改：`/Users/flame/clash2passwall/clash2passwall.js`
- 创建：`/Users/flame/clash2passwall/tests/test_dat_map.mjs`（Node 原生 assert）

**接口**：
- 产出：`DAT_RULESET_MAP`（按 spec 表）；`MODE = "dat"`；`applyRuleset` 在 dat 模式写 `geosite:name` / `geoip:name`；Netflix/BilibiliHMT 可写双字段。

- [ ] **步骤 1：写失败测试**

```javascript
// /Users/flame/clash2passwall/tests/test_dat_map.mjs
import assert from "assert";
import { createRequire } from "module";
// 将 clash2passwall 改为可 require 的导出，或复制映射表到 map_dat.js
// 本计划要求把映射与 apply 抽到 map_dat.js 以便测试：

import {
  DAT_RULESET_MAP,
  applyDatRuleset,
} from "../map_dat.js";

function emptyOut() { return { domain: [], ip: [], policy: null }; }

// Scenario: gfw 映射
{
  const out = emptyOut();
  applyDatRuleset("gfw", out);
  assert.ok(out.domain.includes("geosite:loyalsoldier-gfw"));
  assert.ok(!out.domain.includes("geosite:gfw"));
}

// Scenario: proxy 不映射为 geolocation-!cn
{
  const out = emptyOut();
  applyDatRuleset("proxy", out);
  assert.ok(out.domain.includes("geosite:loyalsoldier-proxy"));
  assert.ok(!out.domain.includes("geosite:geolocation-!cn"));
}

// Scenario: reject / AI / telegramcidr
{
  const a = emptyOut(), b = emptyOut(), c = emptyOut();
  applyDatRuleset("reject", a);
  applyDatRuleset("AI", b);
  applyDatRuleset("telegramcidr", c);
  assert.ok(a.domain.includes("geosite:loyalsoldier-reject"));
  assert.ok(b.domain.includes("geosite:sukka-ai"));
  assert.ok(c.ip.includes("geoip:loyalsoldier-telegramcidr"));
}

// Scenario: Netflix 双字段
{
  const out = emptyOut();
  applyDatRuleset("Netflix", out, { hasGeoip: new Set(["xiaolin-netflix"]) });
  assert.ok(out.domain.includes("geosite:xiaolin-netflix"));
  assert.ok(out.ip.includes("geoip:xiaolin-netflix"));
}

// 兜底 CN
{
  const out = emptyOut();
  // 由 mapRule 测；此处测表含 applications:null
  assert.strictEqual(DAT_RULESET_MAP.applications, null);
}

console.log("test_dat_map: ok");
```

- [ ] **步骤 2：运行确认失败**

```bash
node /Users/flame/clash2passwall/tests/test_dat_map.mjs
```

预期：无法解析 `map_dat.js`。

- [ ] **步骤 3：实现 `map_dat.js` 并接入主文件**

```javascript
// /Users/flame/clash2passwall/map_dat.js
export const DAT_RULESET_MAP = {
  reject:       { name: "loyalsoldier-reject", side: "domain" },
  icloud:       { name: "loyalsoldier-icloud", side: "domain" },
  apple:        { name: "loyalsoldier-apple", side: "domain" },
  google:       { name: "loyalsoldier-google", side: "domain" },
  proxy:        { name: "loyalsoldier-proxy", side: "domain" },
  direct:       { name: "loyalsoldier-direct", side: "domain" },
  private:      { name: "loyalsoldier-private", side: "domain" },
  gfw:          { name: "loyalsoldier-gfw", side: "domain" },
  "tld-not-cn": { name: "loyalsoldier-tld-not-cn", side: "domain" },
  telegramcidr: { name: "loyalsoldier-telegramcidr", side: "ip" },
  cncidr:       { name: "loyalsoldier-cncidr", side: "ip" },
  lancidr:      { name: "loyalsoldier-lancidr", side: "ip" },
  YouTube:      { name: "xiaolin-youtube", side: "domain" },
  youtube:      { name: "xiaolin-youtube", side: "domain" },
  Netflix:      { name: "xiaolin-netflix", side: "domain", alsoIp: true },
  netflix:      { name: "xiaolin-netflix", side: "domain", alsoIp: true },
  Spotify:      { name: "xiaolin-spotify", side: "domain" },
  spotify:      { name: "xiaolin-spotify", side: "domain" },
  BilibiliHMT:  { name: "xiaolin-bilibili", side: "domain", alsoIp: true },
  bilibili:     { name: "xiaolin-bilibili", side: "domain", alsoIp: true },
  TikTok:       { name: "xiaolin-tiktok", side: "domain" },
  tiktok:       { name: "xiaolin-tiktok", side: "domain" },
  AI:           { name: "sukka-ai", side: "domain" },
  ai:           { name: "sukka-ai", side: "domain" },
  applications: null,
};

/**
 * @param {string} name RULE-SET key
 * @param {{domain:string[],ip:string[],policy:any}} out
 * @param {{hasGeoip?: Set<string>}} opts
 */
export function applyDatRuleset(name, out, opts = {}) {
  const m = DAT_RULESET_MAP[name];
  if (m === null) return true; // skipped
  if (!m) return false;
  if (m.side === "domain") {
    out.domain.push("geosite:" + m.name);
    if (m.alsoIp && opts.hasGeoip && opts.hasGeoip.has(m.name)) {
      out.ip.push("geoip:" + m.name);
    }
  } else {
    out.ip.push("geoip:" + m.name);
  }
  return true;
}

export function mapBuiltinGeositeGeoip(type, value, out) {
  // GEOSITE,CN / GEOIP,CN / GEOIP,LAN
  if (type === "GEOSITE") {
    out.domain.push("geosite:" + value.toLowerCase());
  } else if (type === "GEOIP") {
    const v = value.toUpperCase() === "LAN" ? "private" : value.toLowerCase();
    out.ip.push("geoip:" + v);
  }
}
```

在 `clash2passwall.js` 中：

```javascript
// 顶部增加
const { applyDatRuleset, mapBuiltinGeositeGeoip } = require("./map_dat.cjs");
// 若用 ESM 不便，把 map_dat.js 改为 map_dat.cjs 的 module.exports 拷贝同一逻辑

// parseArgs 增加 --dat
// MODE: "sing-box" | "xray" | "dat"

// applyRuleset 分支：
// if (MODE === "dat") { applyDatRuleset(name, out, { hasGeoip: globalHasGeoip }); return; }

// mapRule 中 GEOSITE/GEOIP：
// if (MODE === "dat") { mapBuiltinGeositeGeoip(...); break; }
```

将 `map_dat.js` 同步为 `map_dat.cjs`：

```javascript
// map_dat.cjs — CommonJS 导出，内容与上相同，改用 module.exports = { DAT_RULESET_MAP, applyDatRuleset, mapBuiltinGeositeGeoip }
```

测试改为 require cjs，或 `"type"` 调整。推荐 **测试与实现都用 cjs** 以匹配现有 `clash2passwall.js`。

- [ ] **步骤 4：运行测试通过**

```bash
node /Users/flame/clash2passwall/tests/test_dat_map.mjs
# 或 node --test / node tests/test_dat_map.cjs
```

- [ ] **步骤 5：提交（在 clash2passwall 目录；若无 git 则 init 或仅在 clash-rules-srs 文档引用）**

```bash
cd /Users/flame/clash2passwall
git status 2>/dev/null || git init -b main
git add map_dat.cjs clash2passwall.js tests/
git commit -m "feat(T6): add --dat ruleset map for custom geodata tags"
```

若 clash2passwall 无 remote，提交仍本地保留。

---

### 任务 7：端到端转换烟雾（fixture yaml）

**文件**：
- 创建：`/Users/flame/clash2passwall/tests/fixtures/mini_clash.yaml`
- 创建：`/Users/flame/clash2passwall/tests/test_dat_e2e.cjs`

```yaml
# tests/fixtures/mini_clash.yaml
rules:
  - RULE-SET,gfw,Proxy
  - RULE-SET,proxy,Proxy
  - RULE-SET,reject,REJECT
  - RULE-SET,AI,Proxy
  - RULE-SET,telegramcidr,Proxy
  - RULE-SET,Netflix,Proxy
  - GEOSITE,CN,DIRECT
  - GEOIP,CN,DIRECT
  - GEOIP,LAN,DIRECT
  - MATCH,Proxy
proxy-groups:
  - {name: Proxy, type: select, proxies: [DIRECT]}
  - {name: REJECT, type: select, proxies: [REJECT]}
rule-providers: {}
```

- [ ] **步骤 1–4：TDD**

```javascript
// test_dat_e2e.cjs
const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const assert = require("assert");
const out = path.join(__dirname, "out_dat");
fs.rmSync(out, { recursive: true, force: true });
execFileSync("node", [
  path.join(__dirname, "..", "clash2passwall.js"),
  path.join(__dirname, "fixtures", "mini_clash.yaml"),
  "--dat",
  "--out", out,
  "--no-install",
], { stdio: "inherit" });
const conf = fs.readFileSync(path.join(out, fs.readdirSync(out).find(f => f.endsWith(".conf"))), "utf8");
assert.match(conf, /geosite:loyalsoldier-gfw/);
assert.match(conf, /geosite:loyalsoldier-proxy/);
assert.doesNotMatch(conf, /geolocation-!cn/);
assert.match(conf, /geosite:sukka-ai/);
assert.match(conf, /geoip:loyalsoldier-telegramcidr/);
assert.match(conf, /geosite:cn/);
assert.match(conf, /geoip:private/);
console.log("e2e ok");
```

实现：`clash2passwall.js` 完整支持 `--dat` 写出 conf。

- [ ] **步骤 5：提交** `feat(T7): dat mode e2e fixture conversion`

---

## 切片 C：安装脚本与文档

### 任务 8：安装脚本（URL + 覆盖导入 shunt_rules + geoview 提示）

**文件**：
- 修改：`clash2passwall.js` 的 `generateInstall`（dat 模式分支）
- 或创建：`install_shunt_rules_dat.sh.template`

**行为（生成脚本内容必须字面包含）**：

```sh
#!/bin/sh
# PassWall2 geodata + shunt install (dat mode)
OWNER="${OWNER:-YOUR_GITHUB_USER}"
REPO="${REPO:-clash-rules-srs}"
BASE="https://github.com/${OWNER}/${REPO}/releases/latest/download"

uci -q set passwall2.@global_rules[0].geosite_url="${BASE}/geosite.dat"
uci -q set passwall2.@global_rules[0].geoip_url="${BASE}/geoip.dat"

# 覆盖导入分流：删除既有 shunt_rules，保留 nodes
while uci -q delete passwall2.@shunt_rules[0]; do :; done

# 以下由 generateInstall 展开每条 config shunt_rules ...
# ...

uci commit passwall2
echo "NOTE: sing-box kernel requires geoview >= 0.1.10"
echo "Run PassWall2 rule update or restart to download dat."
```

- [ ] **步骤 1：测试生成物字符串**

```javascript
// tests/test_install_script.cjs
const fs = require("fs");
const { execFileSync } = require("child_process");
const path = require("path");
const assert = require("assert");
const out = path.join(__dirname, "out_install");
fs.rmSync(out, { recursive: true, force: true });
execFileSync("node", [
  path.join(__dirname, "..", "clash2passwall.js"),
  path.join(__dirname, "fixtures", "mini_clash.yaml"),
  "--dat", "--out", out,
], { stdio: "inherit" });
const sh = fs.readdirSync(out).find(f => f.endsWith(".sh"));
const body = fs.readFileSync(path.join(out, sh), "utf8");
assert.match(body, /releases\/latest\/download\/geosite\.dat/);
assert.match(body, /releases\/latest\/download\/geoip\.dat/);
assert.match(body, /delete passwall2\.@shunt_rules/);
assert.match(body, /geoview/);
assert.match(body, /0\.1\.10/);
assert.match(body, /geosite:loyalsoldier-gfw/);
console.log("install script ok");
```

- [ ] **步骤 2–4：实现 generateInstall 的 dat 分支直至测试通过**
- [ ] **步骤 5：提交** `feat(T8): dat install script writes URL and replaces shunt_rules`

---

### 任务 9：重写 README.md 与 context.md

**文件**：
- 重写：`/Users/flame/clash-rules-srs/README.md`
- 重写：`/Users/flame/clash-rules-srs/context.md`（对齐 spec 术语「轻量完整增强底」、退役 `.srs`）

README 必含：
1. 产物与 latest URL 模板（替换 USER）
2. PassWall2 规则页填写方法
3. 自定义 tag 与 sources 关系 + 映射表链接
4. 本地：`bootstrap_vendor.sh` + `build.py` + probe
5. clash2passwall `--dat` 与安装脚本用法
6. 非目标：无 `.srs`、无热更新

- [ ] **步骤 1：撰写并自检关键词**

```bash
grep -E 'releases/latest/download|轻量完整增强|geosite.dat|rule-set:remote' README.md context.md
# 应有 latest/download 与轻量完整增强；不应再把 .srs 当主交付
```

- [ ] **步骤 2：提交** `docs(T9): rewrite README and context for geodata-only delivery`

---

### 任务 10：验收（acceptance-qa）

> 本任务由 executing-plans 收尾审查阶段触发 acceptance-qa 按下表执行，不参与逐任务连续执行；报告与证据落盘特性目录 `acceptance/`。

| Scenario / 检查项 | 维度 | 执行方式 | 目标 | 阈值/预期 | 验收证据 |
|-------------------|------|---------|------|----------|---------|
| Releases URL 形态与 sha 派生 | e2e | 验收任务 | 已发布 latest | 四 URL HTTP 200 且 sha 匹配 | curl 日志 |
| Release 资产列表无 srs | integration | 验收任务 | latest Release | 无 `.srs` asset | `gh release view` |
| 安装 URL / 覆盖导入 / geoview 提示 | e2e/手工 | 验收任务 | 生成 install.sh | 含三要素 + gfw 规则 | 文件摘录 |
| 文档声明 .srs 退役与轻量完整增强定义 | docs | 验收任务 | README+context | 术语一致 | grep 记录 |

---

### 任务 11：合并与清理

- [ ] **步骤 1：全量验证**

```bash
cd /Users/flame/clash-rules-srs/.worktrees/plan-geodata-selfhost
.venv/bin/python -m unittest discover -s tests -v
# 若环境允许：完整 build.py + probe
# clash2passwall tests
node /Users/flame/clash2passwall/tests/test_dat_map.mjs 2>/dev/null || node /Users/flame/clash2passwall/tests/test_dat_map.cjs
node /Users/flame/clash2passwall/tests/test_dat_e2e.cjs
node /Users/flame/clash2passwall/tests/test_install_script.cjs
```

- [ ] **步骤 2：合并回来源分支**

```bash
cd /Users/flame/clash-rules-srs
git merge plan/2026-07-30-geodata-selfhost
```

- [ ] **步骤 3：清理 worktree**

```bash
git worktree remove .worktrees/plan-geodata-selfhost
git branch -d plan/2026-07-30-geodata-selfhost
```

- [ ] **步骤 4：sync_commit 锚定**

```bash
SYNC=$(git rev-parse HEAD)
# 编辑 .spec-dev/2026-07-30-geodata-selfhost/spec/geodata-selfhost-design.md
# sync_commit: <SYNC>
git add .spec-dev/2026-07-30-geodata-selfhost/spec/geodata-selfhost-design.md
git commit -m "chore(spec): sync_commit anchor ${SYNC:0:7}"
```

---

## Self-Review（编写时已核对）

| Spec Requirement | 任务 |
|------------------|------|
| 发布可校验 geodata + sha + latest | T3 write_sha256, T5 CI |
| 仅 dat/sha 资产 | T5 files: 列表 |
| 轻量完整增强底 + sources 探针 | T4 bootstrap+probe, T3 merge |
| tag 与 sources 一一对应 / applications | T2 |
| classical 拆分 | T1–T2 emit, T4 probe |
| dlc 带前缀映射 | T1 |
| fail-fast 发布门禁 | T3 main return 1, T5 无 release on fail |
| clash2passwall 映射表 | T6–T7 |
| 安装脚本 | T8 |
| 文档 | T9 |

Scenario → 测试：均在 T1/T2/T3/T6/T7/T8 有对应 assert。无 TBD 占位。

---
