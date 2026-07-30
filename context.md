# context.md — geodata 自建链路上下文

## 1. 当前目标

本仓库把 `sources.yaml` 中的 Clash 规则源转换成 PassWall2 可由 xray 与 sing-box 双核共同消费的 `geosite.dat` / `geoip.dat`。交付口径是 geodata-only：GitHub Release 仅有两个 dat 和两个 `.sha256sum`，不发布 `.srs`。

采用“轻量完整增强底”而不是只打自定义 list：

- geosite 底为 `v2fly/domain-list-community/data` 全量，因此 `geosite:cn` 等标准 list 保留。
- geoip 底为 `Loyalsoldier/geoip` 官方 `geoip.dat`，因此国家、private 与其增强 list 保留。
- 每个源的 `name` 原样成为自定义 tag；没有另一份手抄 tag 清单。

相关决策记录在 `.spec-dev/adr/0001-custom-tag-naming.md` 和 `.spec-dev/adr/0002-geodata-base-strategy.md`。

## 2. 为什么从 `.srs` 转向 dat

PassWall2 的 xray 路径会丢弃 `rule-set:remote:`，无法消费远程 `.srs`；内置 `rule_update` 则会下载并校验 `geosite.dat` / `geoip.dat`。使用自建 dat 后，两套内核共享 `geosite:<tag>` / `geoip:<tag>` 引用，更新入口也统一到 PassWall2 现有机制。

旧 `.srs`、orphan `release` 分支和 jsDelivr 路径均已退役。仓库内 `tools/clash2passwall` 保留默认 `.srs` 与 `--xray` 模式作为兼容行为，但它们不属于本仓库发布物。

## 3. 构建数据流

```text
sources.yaml
  -> scripts/build.py
     -> build/data/<tag>          带 domain/full/keyword/regexp 前缀
     -> build/ip/<tag>.txt        CIDR
     -> build/data-merged/        community + 自定义 geosite list
     -> build/geoip-config.json   官方 geoip.dat + 自定义 CIDR
  -> domain-list-custom           publish/geosite.dat
  -> Loyalsoldier/geoip convert   publish/geoip.dat
  -> scripts/probe_tags.py        required 正向 + forbidden 负向 tag
  -> *.sha256sum
  -> GitHub latest Release
```

`scripts/lib/buckets.py` 负责五桶分类，`dlc_emit.py` 负责 dlc 文本前缀，`fetch_emit.py` 负责 YAML/text 解析与文件发射，`merge_and_hash.py` 负责冲突、合并、geoip config 和 sha。

## 4. 映射语义

| Clash 输入 | dlc / geoip 中间输出 |
|---|---|
| `DOMAIN-SUFFIX,x`、`+.x`、`*.x` | `domain:x` |
| `DOMAIN,x`、无通配精确域名 | `full:x` |
| `DOMAIN-KEYWORD,x` | `keyword:x` |
| `DOMAIN-REGEX,x`、一般 glob | `regexp:x` 或锚定 glob 正则 |
| `IP-CIDR` / `IP-CIDR6` | 同名 geoip 输入 CIDR |
| `IP-SUFFIX` | 无损 CIDR 转换不存在，skipped |
| `PROCESS-*` 等不支持类型 | skipped；进程-only 源整体跳过 |

`classical` 源的域名和 IP 会拆到同名的两侧 tag。当前 Netflix 与 BilibiliHMT 两侧都有规则；YouTube、Spotify、TikTok 和 Sukka AI 只有域名侧。

## 5. 发布门禁

以下任一条件失败都会让 workflow 在 Release 步骤前终止：

1. 任一启用源下载或解析失败。
2. domain/ipcidr/classical 的期望侧为空（classical 的 IP 侧可不存在）。
3. 自定义 geosite tag 与 community 文件撞名，或 sources tag 重复/非法。
4. community、domain-list-custom、geoip 工具或官方 geoip 底不可用。
5. required tag 不存在，或 forbidden tag 意外存在。
6. dat 缺失、为空、出现额外 publish 文件或 sha 校验失败。

workflow 的 Release files 清单固定为：

```text
geosite.dat
geoip.dat
geosite.dat.sha256sum
geoip.dat.sha256sum
```

## 6. 仓库内 clash2passwall

`tools/clash2passwall/clash2passwall.js --dat` 使用 `map_dat.cjs` 中的固定 provider→tag 映射，并以 `build/expected_tags.json` 决定引用是否实际存在。关键例子：

- `gfw` → `geosite:loyalsoldier-gfw`
- `proxy` → `geosite:loyalsoldier-proxy`，不是 `geolocation-!cn`
- `AI` → `geosite:sukka-ai`
- `telegramcidr` → `geoip:loyalsoldier-telegramcidr`
- Netflix / BilibiliHMT → geosite 必须存在；只有 manifest 声明对应 geoip tag 时才写 ip_list
- `GEOSITE,CN` / `GEOIP,CN` / `GEOIP,LAN` → `geosite:cn` / `geoip:cn` / `geoip:private`

dat 安装脚本使用具名 `shunt_rules`（原分组名存入 `remarks`），在临时 UCI 配置中设置 URL、删除并重建分流、验证后原子替换；错误时恢复备份，不触碰 nodes，并提示 sing-box 需要 `geoview >= 0.1.10`。仓库 URL 必须由 `--repo`、`REPO_SLUG` 或兼容的 `OWNER`+`REPO` 明确提供，不存在占位默认值。

## 7. 维护与验证

- source 变更只编辑 `sources.yaml`；新增 tag 会自动进入 expected manifest 和探针。
- 上游 Go 工具的最低版本会漂移；workflow 当前使用 Go 1.26.x，因为 2026-07-30 的 geoip/geoview 已要求 Go 1.25+。
- 本地完整验收按 README 的测试、build、双 probe 和 sha 命令执行。
- 线上验收仍需用户另行授权创建/配置远端，并在第一次成功 GitHub Release 后检查四个 `releases/latest/download/` URL 与资产列表。

## 8. 非目标

不复刻 `v2ray-rules-dat` 的二次聚合，不自备 MaxMind license，不修改 openwrt-passwall2，不做 mihomo 资产，也不承诺无断流热更新。keyword/regexp 经旧 geoview 版本可能降级，因此设备侧版本下限为 0.1.10，建议使用当前版本。
