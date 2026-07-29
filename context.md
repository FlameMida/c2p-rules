# context.md — 项目知识与决策上下文

本文件记录 clash-rules-srs 的来龙去脉、关键调研结论与设计决策，供后续维护者（人或 AI）快速理解全貌。

## 1. 项目目的

把 Clash Verge Rev 实际在用的 **19 个规则源**（Loyalsoldier 13 + xiaolin-007 5 + Sukka 1）**1:1 自建**为 sing-box `.srs` 订阅，由 GitHub Actions 每日自动拉取原源 → 转换 → 发布，供 OpenWrt **PassWall2**（sing-box 内核）订阅。目标：与原 Clash 规则逐条一致、摆脱对第三方聚合源（MetaCubeX）的依赖、自主可控。

`applications`（进程类规则）在路由器场景无意义，不转换 → 实际产出 **18 个 `.srs`**。

## 2. 背景：三种代理内核的规则格式互不兼容

这是整个项目存在的根本原因。

| 内核 | 原生规则格式 | 是否支持外部规则集订阅 |
|---|---|---|
| **mihomo (Clash.Meta)** | `.mrs`(二进制) / yaml `payload:` / text classical | 是（rule-providers，http+interval）|
| **sing-box** | `.srs`(二进制) / source JSON | 是（`rule_set` remote，自动刷新；local 文件 mtime 热加载）|
| **xray** | `geosite.dat`/`geoip.dat` + 内联域名/IP | **否**（只认 .dat + 内联，无 URL 订阅）|

用户的源（Loyalsoldier/xiaolin-007/Sukka）都是 **mihomo 格式**（yaml payload / text），sing-box/xray **不认**。所以必须转换。

- **sing-box 路径**：转成 `.srs`，PassWall2 用 `rule-set:remote:` 订阅 ← 本项目做的事。
- **xray 路径**：转成 `geosite.dat`/`geoip.dat`（需 v2fly `dlc`，重）或用标准 `.dat` ← 本项目**不自建**（见 §8）。

## 3. PassWall2 分流机制（openwrt-passwall2 仓库）

PassWall2 用 `shunt_rules`（UCI section）定义分流：每条含 `domain_list`（域名文本域）与 `ip_list`（IP 文本域），两套内核各自编译进 routing。

**domain_list 支持的前缀**（`luci-app-passwall2/luasrc/passwall2/util_sing-box.lua:1717-1745`）：
`geosite:` / `regexp:` / `full:`(精确→domain) / `domain:`(后缀→domain_suffix) / `rule-set:remote:` / `rule-set:local:` / `rs:` / 裸文本(→domain_keyword)

**ip_list 支持的前缀**（同文件 `1760-1787`）：
`geoip:`(含 `private`) / `rule-set:remote:` / `rule-set:local:` / 裸 CIDR

**关键代码锚点**：
- `parse_rule_set`（`util_sing-box.lua:1167-1207`）：接受 `local:/remote:` 的 `.srs`/`.json`，**仅认 `.srs`/`.json` 后缀**，其他返回 nil（静默空规则）。
- `rule_update.lua`：PassWall2 自带的定时更新**只更新 `geoip.dat`/`geosite.dat`**，不支持自定义规则集订阅。
- xray 路径（`util_xray.lua:1433,1452`）：**显式丢弃** `rule-set:`/`rs:` 行（xray 不支持外部规则集）。
- PassWall2 `reload()` 实为全量 restart，无 live reload；sing-box remote rule_set 默认 1d 自动刷新（内核内行为，无需重启）。

## 4. 为什么自建而不直接用 MetaCubeX

MetaCubeX/meta-rules-dat 是**唯一同时发布 sing-box `.srs`(sing 分支) 与 xray `.dat` 的聚合源**，内容聚合了 Loyalsoldier/Sukka/v2fly 等上游——所以 `geosite:cn` ≈ Loyalsoldier direct，`geosite:youtube` ≈ xiaolin-007 youtube。

自建的价值：
- **100% 1:1**：用自己指定的源，不经过聚合，内容完全自主（MetaCubeX 偶有聚合差异，如 `proxy`→`geolocation-!cn` 更广）。
- **不依赖第三方**：源、构建、托管全在自己仓库。

工具链选择：**`sing-box rule-set compile`**（官方二进制，成熟可靠）。Python 脚本负责 Clash payload → sing-box source JSON，sing-box 负责压成 `.srs`。**不选** MetaCubeX/meta-rules-converter（21 star、README 仅 524 字节，不成熟）。

## 5. 项目架构与数据流

```
sources.yaml ──► build.py ──► dist/*.srs ──► CI ──► release 分支 ──► jsdelivr @release ──► PassWall2
                  │
                  ├─ urllib 拉源（按 format: yaml|text）
                  ├─ 解析（按 behavior: domain|ipcidr|classical）
                  ├─ 归一化到五桶：domain / domain_suffix / domain_keyword / domain_regex / ip_cidr
                  ├─ 生成 sing-box source JSON（version 2，每类一个 rule）
                  └─ sing-box rule-set compile <json> -o <srs>
```

### 5.1 关键技术点（踩过的坑）

- **每类一个 rule**：sing-box 单条 rule 内多字段是 **AND**，而 Clash 规则是 OR。故 domain_suffix / domain / domain_keyword / domain_regex / ip_cidr 必须分成多个 rule（rule 间 OR，同类数组内 OR）。`build.py:to_source_json()`。
- **format=yaml 但 url 是 `.txt`**：Loyalsoldier/xiaolin-007 的源 url 结尾是 `.txt` 但内容是 yaml `payload:`。按 `format` 字段解析，不看扩展名。
- **classical 解析**：每行是一条 Clash 规则（`TYPE,value[,attr]`），复用 mapRule 的类型映射。`attr` 如 `no-resolve` 忽略；`PROCESS-NAME` 等不支持类型入 skipped。
- **sing-box compile 语法**：`sing-box rule-set compile <source.json> -o <output.srs>`（输出用 `-o`，不是第二个位置参数——曾踩此坑，`accepts 1 arg(s), received 2`）。
- **source JSON version=2**：兼容 sing-box 1.8+；PassWall2 主流 sing-box 1.11+，向上兼容。

### 5.2 CI 发布方式（jsdelivr 限制）

jsdelivr `cdn.jsdelivr.net/gh/USER/REPO@<ref>/<path>` 解析的是**仓库分支/tag 的文件，不含 Release assets**。故 CI 用 `git checkout --orphan release` + 把 `dist/*.srs` 提交 + `git push -f origin release`，让 `.srs` 存在于 `release` 分支，jsdelivr `@release` 才能加速。

## 6. 转换规则（1:1 映射详表）

| Clash 形态 | sing-box .srs 内 |
|---|---|
| `behavior: domain` 的 `+.foo` / `*.foo` | `domain_suffix: ["foo"]` |
| 精确域名 `foo.com` | `domain: ["foo.com"]` |
| 含通配 `*?`（非前缀） | `domain_regex: [glob→regex]` |
| `behavior: ipcidr` 的 `1.2.3.0/24` | `ip_cidr: ["1.2.3.0/24"]` |
| classical/text 的 `DOMAIN,x` | `domain` |
| `DOMAIN-SUFFIX,x` | `domain_suffix` |
| `DOMAIN-KEYWORD,x` | `domain_keyword` |
| `DOMAIN-REGEX,x` | `domain_regex` |
| `IP-CIDR/IP-CIDR6/IP-SUFFIX,x` | `ip_cidr` |
| `PROCESS-NAME` / 逻辑规则(AND/OR/NOT) / `IP-ASN` | 跳过（路由器无意义或不支持）|

## 7. 源清单（sources.yaml，18 个有效源）

- **Loyalsoldier/clash-rules**（yaml payload）：reject / icloud / apple / google / proxy / direct / private / gfw / tld-not-cn（domain）；telegramcidr / cncidr / lancidr（ipcidr）。
- **xiaolin-007/clash**（yaml classical，含 IP-CIDR 混合）：YouTube / Netflix / Spotify / BilibiliHMT / TikTok。
- **Sukka**（text classical）：ai（`https://ruleset.skk.moe/Clash/non_ip/ai.txt`，唯一 text）。
- 自定义接口：`sources.yaml` 末尾追加 `{name, behavior, url, [format]}` 即可，下次 CI 自动纳入。

## 8. xray 部分（不自建）

已与用户确认：xray **不自建** geosite.dat/geoip.dat。原因：用户的源内容是标准 geosite 的子集，自建 `.dat` 与用 MetaCubeX/Loyalsoldier 标准 `.dat` 内容基本等价，但自建需 Go + v2fly `dlc` 工具链，工程量大、收益低。

xray 用户直接用标准 `.dat`（PassWall2 规则管理页选 MetaCubeX/Loyalsoldier 源），引用方式由 **`/Users/flame/clash2passwall/clash2passwall.js --xray`** 模式处理（输出 `geosite:`/`geoip:` 前缀，而非 `.srs`）。

## 9. 关联项目

- **`/Users/flame/clash2passwall/clash2passwall.js`**：Clash 配置 → PassWall2 shunt_rules UCI 配置的转换脚本（sing-box 默认出 `rule-set:remote:.srs`，`--xray` 出 `geosite:`/`geoip:`）。本项目的转换逻辑（mapRule 类型映射）即源自它。配套 `output/install_shunt_rules*.sh`（清空自带 shunt_rules + 导入）。
- **`/Users/flame/openwrt-passwall2`**：PassWall2 源码（lua + LuCI），分流机制与代码锚点见 §3。

## 10. 已知坑与注意事项

- **classical 含 IP-CIDR**（xiaolin-bilibili 等）：其 `.srs` 同时有域名和 IP；PassWall2 里填进 `domain_list` 只匹配域名，IP 不生效——需同时填 `ip_list`。
- **sing-box 版本**：CI 固定 `1.11.4`，避免上游 source format 变动；`.srs` 向后兼容性有限，旧 sing-box 读新 `.srs` 可能不支持，故 source JSON 用 version 2（最兼容）。
- **jsdelivr 国内不稳**：README 给 `gh-proxy.com` 前缀备选。
- **`applications`（PROCESS-NAME）跳过**：路由器无进程概念。

## 11. 验证方法

1. **1:1 核对**：`build.py` 日志每源打印 `raw=<原payload条数> → <转换后条数>`，二者应相等（无丢失）。
2. **.srs 合法性**：`sing-box rule-set compile` 成功退出即合法；`sing-box rule-set decompile <x.srs> -o back.json` 可回读。
3. **CI**：Actions 手动触发，确认 `release` 分支有 18 个 `.srs`，jsdelivr URL HTTP 200。
4. **PassWall2 联通**：贴 `rule-set:remote:<url>` 进 shunt_rules，sing-box 启动成功 = 下载+解析成功，实测分流。

## 12. 本地构建

```bash
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
# 装 sing-box（macOS: brew install sing-box；或下二进制）
sing-box version  # 确认在 PATH
.venv/bin/python scripts/build.py   # 产出 dist/*.srs
```
