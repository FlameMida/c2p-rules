---
# —— spec-dev 漂移守卫锚点（机器可校验，勿删）——
spec_dev:
  version: 1
  feature: geodata-selfhost
  status: draft
  covers:
    - "scripts/**"
    - "sources.yaml"
    - ".github/workflows/**"
    - "README.md"
    - "context.md"
    - "requirements.txt"
    - "/Users/flame/clash2passwall/clash2passwall.js"
    - "/Users/flame/clash2passwall/output*/**"
  sync_commit: null
---

# 自建 geodata 全链路（方案 A）设计

## 背景与目标

`clash-rules-srs` 当前只产出 sing-box `.srs`，且仓库无 git、CI 从未真正发布；PassWall2 的 xray 路径显式丢弃 `rule-set:`，双核无法共用同一套远程规则集。方案 A 将主产物改为 V2Ray/Xray 兼容的 `geosite.dat`/`geoip.dat`，在「Loyalsoldier 全量底」上注入用户 1:1 源 tag，经 GitHub Releases 供 PassWall2 内置 `rule_update` 消费，并打通 `clash2passwall` 与安装脚本。

**成功标准**

1. Releases 提供 `geosite.dat`、`geoip.dat` 及对应 `.sha256sum`，格式可被 PassWall2 `rule_update.lua` 校验。
2. 产物同时包含标准 tag（community/Loyalsoldier 全量底）与自定义 tag（与 `sources.yaml` 的 `name` 一致）。
3. PassWall2 将 `geoip_url`/`geosite_url` 指向该 Releases 后，xray 与 sing-box（经 geoview）均可使用 `geosite:loyalsoldier-gfw` 等引用。
4. `clash2passwall` 能输出对齐自定义 tag 的 shunt 配置，并有安装脚本写入 URL 与分流规则。

## 非目标

- 产出或维护 `.srs` / `rule-set:remote:` 路径（本版仅 `.dat`）。
- 修改 `openwrt-passwall2` 源码或实现无断流热更新。
- 接入 mihomo 内核、连接监看、JS 脚本扩展。
- 完整复刻 `Loyalsoldier/v2ray-rules-dat` 的二次聚合逻辑（gfwlist 等）；「完整增强」定义见术语表与决策节。
- orphan `release` 分支与 jsDelivr purge（本版仅 GitHub Releases）。

## 术语表

- **自定义 tag**：写入 `.dat` 的 list 名，等于 `sources.yaml` 的 `name`（如 `loyalsoldier-gfw`）。_Avoid_：MetaCubeX 近似名（`geolocation-!cn` 等）、Script.js 短键 alone 作为唯一真源。
- **完整增强底**：geosite = `v2fly/domain-list-community` 的 `data/` 全量；geoip = `Loyalsoldier/geoip` 发布的 `geoip.dat` 整包。_Avoid_：理解为 v2ray-rules-dat 全聚合流水线。
- **消费契约**：PassWall2 要求的文件名、sha256 伴随文件、URL 形态与 tag 引用方式。_Avoid_：随意文件名或无 hash 发布。
- **classical 拆分**：将 classical 源中的域名写入 geosite tag、IP-CIDR 写入同名或对应 geoip tag。_Avoid_：单 tag 混装域名与 IP。

## 影响面

| 路径 | 影响 |
|------|------|
| `/Users/flame/clash-rules-srs/**` | 原地重构：build、CI、文档、git 初始化；`.srs` 交付退役 |
| `/Users/flame/clash2passwall/**` | 映射表/`--dat` 模式、安装脚本生成 |
| PassWall2 运行时 UCI | 仅运维改 `geoip_url`/`geosite_url` 与分流规则；**不改**上游包源码 |
| 外部工具 | CI 使用 `domain-list-custom`、`Loyalsoldier/geoip`、可选 geoview 探针 |

## 已确认的关键决策

- **内容范围**：完整增强底 + 自定义 tag —— 避免只含自定义 tag 时默认 `geosite:cn`/`geoip:cn` 空匹配。
- **产物**：仅 `.dat` + `.sha256sum` —— 双核统一走 `geosite:`/`geoip:`，YAGNI 掉 `.srs` 双轨。
- **自定义 tag 命名**：与 `sources.yaml` 的 `name` 对齐 —— 1:1 可追溯（详见 `../../adr/0001-custom-tag-naming.md`）。
- **classical IP**：拆独立 geoip tag（如 `xiaolin-netflix`）—— `.dat` 分文件约束下保真。
- **发布**：仅 GitHub Releases `latest/download` —— 对齐 PassWall2 默认 URL 形态。
- **改造边界**：全链路（规则集 + clash2passwall + 安装脚本）。
- **仓库安置**：原地重构 `clash-rules-srs`（`git init`，远程名可后定）。
- **实施方案**：方案 1 轻量完整增强 —— geosite 用社区 data ∪ 自定义 list；geoip 用官方 dat 作 input 再 add 自定义 CIDR；不自备 MaxMind license（详见 `../../adr/0002-geodata-base-strategy.md`）。

## 行为规范（Requirements）

### Requirement: 发布可被 PassWall2 校验的 geodata 资产

构建流水线 SHALL 产出并发布名为 `geosite.dat` 与 `geoip.dat` 的文件，以及同名 `.sha256sum` 伴随文件；伴随文件每行格式为「64 位十六进制哈希 + 两个空格 + 纯文件名 + 换行」，且不含路径前缀。

#### Scenario: sha256 伴随文件可被标准工具校验

- **GIVEN** 一次成功的发布产物目录 `publish/`
- **WHEN** 在该目录执行 `sha256sum -c geosite.dat.sha256sum` 与 `sha256sum -c geoip.dat.sha256sum`
- **THEN** 两条命令均 exit 0 且报告 OK

#### Scenario: Releases URL 形态

- **GIVEN** 仓库已配置 GitHub Releases 发布
- **WHEN** 访问 `.../releases/latest/download/geosite.dat` 与 `.../geoip.dat` 及对应 `.sha256sum`
- **THEN** HTTP 成功且文件与当次构建产物一致

### Requirement: 产物包含完整增强底与自定义 tag

`geosite.dat` SHALL 包含 domain-list-community 数据目录所提供的标准 list（至少可解析 `cn`），并包含全部有效自定义域名 tag；`geoip.dat` SHALL 以 Loyalsoldier 官方 `geoip.dat` 为底并叠加全部有效自定义 IP tag。

#### Scenario: 标准 cn 仍可用

- **GIVEN** 新构建的 `geosite.dat` / `geoip.dat`
- **WHEN** 查询 list 名 `cn`（geosite 与 geoip）
- **THEN** 两侧均存在非空规则集

#### Scenario: 自定义 gfw tag 存在

- **GIVEN** `sources.yaml` 含 `loyalsoldier-gfw` 且上游源可拉取
- **WHEN** 构建完成
- **THEN** `geosite.dat` 中存在 list `loyalsoldier-gfw` 且条目数大于 0

### Requirement: 自定义 tag 与 sources.yaml 一一对应

每个启用的源的 `name` SHALL 成为写入 dat 的 list 名；`applications` 类进程规则 SHALL 不生成 tag。

#### Scenario: 域名源写入 geosite

- **GIVEN** 源 `name: loyalsoldier-proxy`、`behavior: domain`
- **WHEN** 构建
- **THEN** 仅写入 `geosite.dat` 的 list `loyalsoldier-proxy`，不创建同名 geoip list

#### Scenario: 跳过 applications

- **GIVEN** 源清单注释或配置标明 applications/PROCESS-NAME 不转换
- **WHEN** 构建
- **THEN** 不产生以 applications 为名的 list

### Requirement: classical 混合源拆分域名与 IP

classical 源中的域名类规则 SHALL 写入对应 geosite tag；IP-CIDR/IP-CIDR6 类规则 SHALL 写入同名 geoip tag；若某 classical 源无任何 IP 规则，则 SHALL NOT 创建空的 geoip tag。

#### Scenario: Netflix 同时有域名与 IP

- **GIVEN** xiaolin Netflix 源同时含 DOMAIN* 与 IP-CIDR 行
- **WHEN** 构建
- **THEN** 存在非空 `geosite:xiaolin-netflix` 与非空 `geoip:xiaolin-netflix`

#### Scenario: YouTube 仅域名

- **GIVEN** xiaolin YouTube 源无 IP 行
- **WHEN** 构建
- **THEN** 存在 `geosite:xiaolin-youtube`，且不存在空的 `geoip:xiaolin-youtube` 交付物要求

### Requirement: Clash 规则到 dlc 行的映射保真

域名转换 SHALL 按下列映射写入 dlc 文本：`DOMAIN-SUFFIX`/`+.`/`*.` → `domain:`（或等价裸域名）；`DOMAIN`/精确 → `full:`；`DOMAIN-KEYWORD` → `keyword:`；`DOMAIN-REGEX`/非前缀通配 → `regexp:`。不支持的类型（如 PROCESS-NAME）SHALL 跳过并计入日志。

#### Scenario: 后缀与精确域名

- **GIVEN** payload 含 `+.example.com` 与 `www.example.com`
- **WHEN** 生成 `data/loyalsoldier-*` 文本
- **THEN** 分别出现 `domain:example.com`（或裸 `example.com`）与 `full:www.example.com`

### Requirement: 单源失败不得发布残缺 dat

当任一启用源下载或解析失败时，构建进程 SHALL 以非零退出码结束，且 CI SHALL NOT 将不完整产物发布为 latest Release 资产。

#### Scenario: 上游 404

- **GIVEN** 某一 `sources.yaml` URL 返回失败
- **WHEN** 执行完整构建
- **THEN** 进程失败，且不更新 GitHub Releases 上的 dat 资产

### Requirement: clash2passwall 输出自定义 tag

`clash2passwall` 在 dat 模式（`--dat` 或项目约定的等价开关）下 SHALL 将 Script.js 同源的 RULE-SET 映射为 `geosite:<sources.name>` / `geoip:<sources.name>`，而非 MetaCubeX 近似标准名；对同时含 IP 的 xiaolin 源 SHALL 在 domain_list 与 ip_list 两侧分别输出。

#### Scenario: gfw 映射

- **GIVEN** 输入规则含 `RULE-SET,gfw,Proxy`
- **WHEN** 以 dat 模式转换
- **THEN** 输出含 `geosite:loyalsoldier-gfw`，且不含将该条映射为 `geolocation-!cn` 的结果

#### Scenario: Netflix 双字段

- **GIVEN** 输入含 `RULE-SET,Netflix,...` 且该源在构建中产生了 geoip tag
- **WHEN** dat 模式转换
- **THEN** 对应分流规则 domain_list 含 `geosite:xiaolin-netflix`，ip_list 含 `geoip:xiaolin-netflix`

### Requirement: 安装脚本配置 PassWall2 消费入口

安装脚本 SHALL 将 `global_rules.geoip_url` 与 `geosite_url` 设为仓库 Releases 的 `geoip.dat`/`geosite.dat` URL，并导入与用户规则意图对齐的 shunt_rules（策略：覆盖导入分流规则，不删除用户节点）。脚本 SHALL 提示 sing-box 路径需要 geoview≥0.1.10。

#### Scenario: 写入 URL

- **GIVEN** 目标设备可写 UCI
- **WHEN** 执行安装脚本（配置了正确的 GitHub 用户/仓库）
- **THEN** `uci get passwall2.@global_rules[0].geosite_url` 指向 `.../releases/latest/download/geosite.dat`（geoip 同理）

## 方案设计

### 架构与组件

```
sources.yaml
    → scripts/build.py
        → build/data/<tag>          # dlc 文本（自定义域名）
        → build/ip/<tag>.txt        # CIDR 文本
        → build/data-merged/        # community data ∪ 自定义
        → build/geoip-config.json
    → domain-list-custom --datapath=build/data-merged → publish/geosite.dat
    → geoip convert -c build/geoip-config.json       → publish/geoip.dat
    → sha256sum → publish/*.sha256sum
    → GitHub Releases
         ↘ PassWall2 rule_update
         ↘ clash2passwall --dat + install_shunt_rules.sh
```

| 组件 | 职责 | 依赖 |
|------|------|------|
| `sources.yaml` | 源清单与扩展接口 | 无 |
| `scripts/build.py` | 拉源、五桶、写 dlc/IP、生成 geoip config、编排工具、hash | PyYAML |
| domain-list-custom | 合并 data → geosite.dat | Go、community checkout |
| Loyalsoldier/geoip | 上游 dat + 自定义 CIDR → geoip.dat | Go |
| GHA build.yml | 日更/手动、发 Releases | contents:write |
| clash2passwall | RULE-SET → 自定义 tag | 与 sources 对齐的映射 |
| 安装脚本 | UCI URL + 导入 shunt_rules | OpenWrt shell/UCI |

### 数据流

1. 读取 `sources.yaml` 启用源（跳过 applications）。
2. 下载并按 `format`/`behavior` 解析为五桶：`domain` / `domain_suffix` / `domain_keyword` / `domain_regex` / `ip_cidr`。
3. 域名桶 → dlc 行写入 `build/data/<name>`；IP 桶 → `build/ip/<name>.txt`。
4. 合并 community `data/` 与自定义文件到 `build/data-merged`（自定义文件名不得覆盖 community 已有文件名；启动前 assert）。
5. `domain-list-custom` 生成 `publish/geosite.dat`。
6. `geoip-config.json`：`input` 含官方 `geoip.dat`（`v2rayGeoIPDat`）+ 各 `text` 自定义 list；`output` 为 `v2rayGeoIPDat` → `publish/geoip.dat`。
7. 生成 sha256；探针校验标准 `cn` 与全部自定义 tag 非空。
8. 发布 Releases；下游转换/安装脚本消费 URL 与 tag。

### 关键接口

**sources.yaml 项（保持）**

```yaml
- {name: <tag>, behavior: domain|ipcidr|classical, url: "...", format: yaml|text}  # format 默认 yaml
```

**domain-list-custom（CI）**

```text
go run ./ --datapath=build/data-merged --datname=geosite.dat --outputpath=publish \
  --exportlists= --togfwlist=
```

（`exportlists`/`togfwlist` 置空或最小，避免多余 plaintext 噪音；若工具不允许空值则按实施时实测最小合法值。）

**geoip convert**

- `input`: `v2rayGeoIPDat`（上游 URL 或缓存文件）+ 多个 `text`（`name`=`tag`, `uri`=`build/ip/<tag>.txt`）
- `output`: `v2rayGeoIPDat` → `publish/geoip.dat`

**clash2passwall**

- 开关：`--dat`（名称以实施为准，文档与 README 一致）
- 映射：Script provider 键 → `sources.yaml` name → `geosite:`/`geoip:` 前缀

**PassWall2 UCI**

- `global_rules.geoip_url` / `geosite_url`
- `global_rules.v2ray_location_asset` 默认 `/usr/share/v2ray/`（不强制改）

**自定义 tag 清单（交付时与 sources 同步）**

- geosite（15）：`loyalsoldier-reject|icloud|apple|google|proxy|direct|private|gfw|tld-not-cn`、`xiaolin-youtube|netflix|spotify|bilibili|tiktok`、`sukka-ai`
- geoip（3+动态）：`loyalsoldier-telegramcidr|cncidr|lancidr`；若有 IP 则加 `xiaolin-netflix`、`xiaolin-bilibili` 等

### 错误处理

| 场景 | 行为 |
|------|------|
| 单源下载/解析失败 | fail-fast，非零退出，不发 Release |
| 上游 geoip.dat 不可达 | 构建失败 |
| 自定义文件名与 community 冲突 | 构建前 assert 失败 |
| classical 无 IP | 不写空 geoip tag |
| keyword/regex 在 geoview 路径丢失 | 文档声明；不在本特性修复 geoview |
| 用户未改 URL | 自定义 tag 空匹配；安装脚本负责写 URL |

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|-------------------|------|---------|---------|
| sha256 伴随文件可被标准工具校验 | integration | 任务内 TDD/脚本 | `sha256sum -c` 通过 |
| Releases URL 形态 | e2e | 验收任务 | HTTP 200 + 文件一致 |
| 标准 cn 仍可用 | integration | 任务内探针 | list 非空 |
| 自定义 gfw tag 存在 | integration | 任务内探针 | list 非空 |
| 域名源写入 geosite | unit | 任务内 TDD | 生成文件与桶断言 |
| 跳过 applications | unit | 任务内 TDD | 无对应 list |
| Netflix 同时有域名与 IP | integration | 任务内 TDD+探针 | 双 tag 非空 |
| YouTube 仅域名 | unit | 任务内 TDD | 无空 geoip 要求 |
| 后缀与精确域名映射 | unit | 任务内 TDD | dlc 行内容 |
| 上游 404 | integration | 任务内 TDD | 非零退出且无发布 |
| gfw 映射 | unit | 任务内 TDD | 输出黄金文件 |
| Netflix 双字段 | unit | 任务内 TDD | domain+ip 输出 |
| 写入 URL | e2e/手工 | 验收任务 | UCI 值正确 |
| 文档声明 .srs 退役与完整增强定义 | docs | 验收任务 | README/context 一致 |

## 风险与边缘情况

| 风险 | 缓解 |
|------|------|
| 用户误解「完整增强」= v2ray-rules-dat 全聚合 | README/context/本 spec 术语表写死定义 |
| CI 无 remote / 未建 GitHub 仓 | 文档步骤：建空仓、推送、`GH_TOKEN` 权限 |
| Go 工具版本漂移 | CI 固定 Go 版本与工具 commit/module 版本 |
| 社区 data 体积导致超时 | checkout 缓存；仅失败重试 |
| geoview 丢 keyword/regex | 文档；xray 路径仍保留 |
| 双仓库改动不同步 | 映射表注释互链；安装脚本版本注释 |
| 定时更新仍会全量 restart 断流 | 已知架构限制，本特性不解决 |

## 开放问题

- domain-list-custom 的 `--exportlists`/`--togfwlist` 是否允许完全关闭 plaintext/gfwlist 输出：实施时以 `--help`/试跑为准，取最小噪音配置。
- GitHub Release 采用「持续更新同一 tag（如 `latest` 资产滚动）」还是「每日新 tag」：默认滚动 `release` 或 `latest` 资产更新，实施可二选一并写进 README。
- `clash2passwall` 是否保留旧 MetaCubeX/`--xray` 标准名模式作为并列开关：建议保留旧模式以免破坏既有用户，dat 模式为新路径。
