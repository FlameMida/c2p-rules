---
# —— spec-dev 漂移守卫锚点（机器可校验，勿删）——
spec_dev:
  version: 1
  feature: geodata-selfhost
  status: active
  covers:
    - "scripts/**"
    - "sources.yaml"
    - ".github/workflows/**"
    - "README.md"
    - "context.md"
    - "requirements.txt"
    - "tools/clash2passwall/**"
  sync_commit: null
---

# 自建 geodata 全链路（方案 A）设计

## 背景与目标

`clash-rules-srs` 当前只产出 sing-box `.srs`，且仓库此前无 git、CI 从未真正发布；PassWall2 的 xray 路径显式丢弃 `rule-set:`，双核无法共用同一套远程规则集。方案 A 将主产物改为 V2Ray/Xray 兼容的 `geosite.dat`/`geoip.dat`：在**轻量完整增强底**（geosite = domain-list-community 全量；geoip = Loyalsoldier 官方 `geoip.dat`）上注入用户 1:1 源 tag，经 GitHub Releases 供 PassWall2 内置 `rule_update` 消费，并打通 `clash2passwall` 与安装脚本。

**成功标准**

1. Releases 提供 `geosite.dat`、`geoip.dat` 及对应 `.sha256sum`，格式可被 PassWall2 `rule_update.lua` 校验；sha 资源 URL 可由 dat URL 将最终路径段 `X` 换为 `X.sha256sum` 得到。
2. 产物同时包含：geosite 侧 community 标准 list（至少 `cn`）+ 全部 `sources.yaml` 域名侧自定义 tag；geoip 侧官方底包 list + 全部 IP 侧自定义 tag。
3. PassWall2 将 `geoip_url`/`geosite_url` 指向该 Releases 的 `.../releases/latest/download/{geoip,geosite}.dat` 后，xray 与 sing-box（经 geoview）均可使用 `geosite:loyalsoldier-gfw` 等引用。
4. 仓库内 `clash2passwall --dat` 按固定映射表与本次构建 manifest 输出确实存在的 tag；安装脚本写入明确配置的 URL，并事务性覆盖导入具名分流规则（不删节点）。

## 非目标

- 产出或维护 `.srs` / `rule-set:remote:` 作为发布产物（本版仅 `.dat` + `.sha256sum`）。
- 修改 `openwrt-passwall2` 源码或实现无断流热更新。
- 接入 mihomo 内核、连接监看、JS 脚本扩展。
- 完整复刻 `Loyalsoldier/v2ray-rules-dat` 的二次聚合逻辑（gfwlist 等）。
- orphan `release` 分支与 jsDelivr purge（本版仅 GitHub Releases）。
- 强制用户删除旧的 MetaCubeX/`--xray` 标准名模式：旧模式可保留为并列开关，但**不是**本特性成功标准。

## 术语表

- **自定义 tag**：写入 `.dat` 的 list 名，等于 `sources.yaml` 条目的 `name`（如 `loyalsoldier-gfw`）。_Avoid_：MetaCubeX 近似名、仅用 Script.js 短键作 dat list 名。
- **轻量完整增强底**：geosite = `v2fly/domain-list-community` 的 `data/` 全量；geoip = `Loyalsoldier/geoip` 已发布的 `geoip.dat` 整包。_Avoid_：「Loyalsoldier 全量底」混称（易被理解成 geosite 也用 Loyalsoldier 增强包）、v2ray-rules-dat 全聚合流水线。
- **启用源**：`sources.yaml` 中 `sources` 数组的每一个条目；本仓库无 enable 字段，数组内即启用。
- **applications 跳过**：`name` 为 `applications` 的源，或解析后仅含完整 `PROCESS-*` 家族规则的源；不生成任何 dat list。
- **消费契约**：PassWall2 要求的文件名、sha256 伴随文件与 URL 派生、tag 引用方式。
- **classical 拆分**：classical 源中域名 → geosite tag；IP-CIDR/IP-CIDR6 → **同名** geoip tag；`IP-SUFFIX` 无法无损转为 CIDR，明确跳过。
- **交付切片**：A 构建发布契约；B 转换映射；C 安装脚本（同一 spec，实施 plan 可分批）。

## 影响面

| 路径 | 影响 |
|------|------|
| `/Users/flame/clash-rules-srs/**` | 原地重构：build、CI、文档；`.srs` 不再作为发布产物 |
| `tools/clash2passwall/**` | 已并入主仓的 `--dat` 映射、具名分流与事务安装脚本 |
| PassWall2 运行时 UCI | 仅运维/脚本改 URL 与分流；**不改**上游包源码 |
| 外部工具 | CI：`domain-list-custom`、`Loyalsoldier/geoip`；list 存在性探针工具不绑定特定实现 |

## 已确认的关键决策

- **内容范围**：轻量完整增强底 + 自定义 tag（见 `../../adr/0002-geodata-base-strategy.md`）。
- **产物**：仅 `.dat` + `.sha256sum`。
- **自定义 tag 命名**：与 `sources.yaml` 的 `name` 对齐（见 `../../adr/0001-custom-tag-naming.md`）。
- **classical IP**：拆同名 geoip tag。
- **发布**：必须提供 `.../releases/latest/download/{geoip,geosite}.dat[.sha256sum]`；可选额外日更 tag，**不得替代** latest 资产。
- **改造边界**：全链路（切片 A+B+C）。
- **仓库安置**：原地重构 `clash-rules-srs`。
- **实施方案**：方案 1；geoip 不自备 MaxMind license。
- **旧转换模式**：`--dat` 为新路径；旧 MetaCubeX / 标准名模式可保留为兼容开关，不作为本特性验收阻断项。

## 行为规范（Requirements）

### Requirement: 发布可被 PassWall2 校验的 geodata 资产

构建流水线 SHALL 产出并发布名为 `geosite.dat` 与 `geoip.dat` 的文件，以及同名 `.sha256sum` 伴随文件。伴随文件每行格式为「64 位十六进制哈希 + 两个空格 + 纯文件名 + 换行」，不含路径前缀。sha 资源的公开 URL SHALL 可由对应 dat URL 将最终路径段 `X` 替换为 `X.sha256sum` 得到。发布通道 SHALL 保证资产可通过 `.../releases/latest/download/` 获取。

#### Scenario: sha256 伴随文件可被标准工具校验

- **GIVEN** 一次成功的发布产物目录 `publish/`
- **WHEN** 在该目录执行 `sha256sum -c geosite.dat.sha256sum` 与 `sha256sum -c geoip.dat.sha256sum`
- **THEN** 两条命令均 exit 0 且报告 OK

#### Scenario: Releases latest URL 与 sha 派生

- **GIVEN** 仓库已完成一次成功发布
- **WHEN** 分别请求 `.../releases/latest/download/geosite.dat`、`.../geosite.dat.sha256sum`、`.../geoip.dat`、`.../geoip.dat.sha256sum`
- **THEN** 均 HTTP 成功，且 dat 与 sha 互相匹配

### Requirement: 发布产物仅限 dat 与 sha256sum

CI 发布到 GitHub Releases 的资产集合 SHALL 仅包含 `geosite.dat`、`geoip.dat`、`geosite.dat.sha256sum`、`geoip.dat.sha256sum`（允许附带构建日志 artifact，但**不得**将 `.srs` 作为 Release 资产发布）。

构建 SHALL 在只读 token job 中运行，checkout 不持久化凭据，可执行上游工具固定到完整提交；独立写权限 job 只能消费已验证 artifact。新 Release SHALL 先保持 draft，上传后经 API 回读确认资产集合精确等于四项，再公开并切换 latest。

#### Scenario: Release 资产列表无 srs

- **GIVEN** 一次成功的 latest Release
- **WHEN** 列举该 Release 的 asset 文件名
- **THEN** 不存在以 `.srs` 结尾的 asset

### Requirement: 产物包含轻量完整增强底与 sources 自定义 tag

`geosite.dat` SHALL 包含 domain-list-community `data/` 所提供的标准 list（至少 list `cn` 非空），并包含每一个启用源在域名侧应产生的自定义 tag，且每个此类 tag 非空。`geoip.dat` SHALL 以 Loyalsoldier 官方 `geoip.dat` 为底（至少 list `cn` 非空），并叠加每一个启用源在 IP 侧应产生的自定义 tag，且每个此类 tag 非空。自定义 tag 集合的唯一真源是 `sources.yaml`，不是手抄清单。

#### Scenario: 标准 cn 仍可用

- **GIVEN** 新构建的 `geosite.dat` 与 `geoip.dat`
- **WHEN** 查询 list 名 `cn`
- **THEN** 两个文件中 `cn` 均存在且规则条数大于 0

#### Scenario: 对 sources 逐项探针

- **GIVEN** 当前 `sources.yaml` 全部启用源均拉取成功
- **WHEN** 构建完成并对每个源按 behavior 推导期望 tag
- **THEN** 域名侧期望 tag 均在 `geosite.dat` 中非空；IP 侧期望 tag 均在 `geoip.dat` 中非空

### Requirement: 自定义 tag 与 sources.yaml 一一对应

对每个启用源，其 `name` SHALL 作为写入 dat 的 list 名。applications 跳过规则（见术语表）SHALL 不生成任何 list。若某启用源在域名侧解析后规则条数为 0，或在 IP 侧（ipcidr 或 classical 含 IP）解析后条数为 0，构建 SHALL 失败（classical 无 IP 从而不建 geoip tag 的情况除外）。

#### Scenario: 域名源写入 geosite

- **GIVEN** 源 `name: loyalsoldier-proxy`、`behavior: domain` 且解析后至少 1 条域名规则
- **WHEN** 构建
- **THEN** `geosite.dat` 含非空 list `loyalsoldier-proxy`，且 `geoip.dat` 不含 list `loyalsoldier-proxy`

#### Scenario: 跳过 applications

- **GIVEN** 源 `name: applications` 或解析结果仅含 PROCESS-NAME 类规则
- **WHEN** 构建
- **THEN** `geosite.dat` 与 `geoip.dat` 均不存在 list `applications`

### Requirement: classical 混合源拆分域名与 IP

classical 源中的域名类规则 SHALL 写入 `geosite.dat` 的 list `<name>`；IP-CIDR/IP-CIDR6 类规则 SHALL 写入 `geoip.dat` 的 list `<name>`。若该源无任何 IP 规则，则 `geoip.dat` SHALL NOT 包含 list `<name>`。

#### Scenario: Netflix 同时有域名与 IP

- **GIVEN** `name: xiaolin-netflix` 的 classical 源同时含域名规则与 IP-CIDR 行
- **WHEN** 构建
- **THEN** `geosite.dat` 与 `geoip.dat` 中 list `xiaolin-netflix` 均存在且条数大于 0

#### Scenario: YouTube 仅域名

- **GIVEN** `name: xiaolin-youtube` 的 classical 源无 IP 行
- **WHEN** 构建
- **THEN** `geosite.dat` 含非空 list `xiaolin-youtube`，且 `geoip.dat` 中不存在 list `xiaolin-youtube`

### Requirement: Clash 规则到 dlc 行的映射保真

写入 dlc 文本时 SHALL **统一使用带前缀**形式（禁止裸域名作为规范输出）：

| 输入 | 输出行 |
|------|--------|
| `DOMAIN-SUFFIX,x` / payload `+.x` / `*.x` | `domain:x` |
| `DOMAIN,x` / 无通配的精确域名 | `full:x` |
| `DOMAIN-KEYWORD,x` | `keyword:x` |
| `DOMAIN-REGEX,x` / 含非前缀 `*`/`?` 的通配 | `regexp:<转换后正则>` |

不支持的类型（如 PROCESS-NAME）SHALL 跳过并计入构建日志的 skipped 计数。

#### Scenario: domain-behavior 后缀与精确

- **GIVEN** domain payload 含 `+.example.com` 与 `www.example.com`
- **WHEN** 生成对应 `build/data/<name>` 文件
- **THEN** 文件中含行 `domain:example.com` 与 `full:www.example.com`，且不含无前缀的裸 `example.com` 作为该后缀项的规范行

#### Scenario: classical 关键词

- **GIVEN** classical 行 `DOMAIN-KEYWORD,openai`
- **WHEN** 生成 dlc 文本
- **THEN** 含行 `keyword:openai`

### Requirement: 构建失败条件与 fail-fast 发布门禁

下列任一情况发生时，构建进程 SHALL 以非零退出码结束，且 CI SHALL NOT 更新 latest Release 上的 dat/sha 资产：

1. 任一启用源下载或解析失败；
2. 上游官方 `geoip.dat` 底包不可达或无效；
3. 自定义 list 文件名与 community `data/` 已有文件名冲突；
4. 域名侧或（应存在的）IP 侧自定义 tag 为空。

#### Scenario: 上游源 404

- **GIVEN** 某一启用源 URL 返回失败
- **WHEN** 执行完整构建
- **THEN** 进程非零退出，且不更新 GitHub Releases latest 的 dat 资产

#### Scenario: 自定义与 community 撞名

- **GIVEN** 自定义将写入的文件名与 community data 中某文件同名
- **WHEN** 构建
- **THEN** 在调用 domain-list-custom 之前失败并非零退出

### Requirement: clash2passwall dat 模式使用固定 provider→tag 映射

在 `--dat` 模式下，`clash2passwall` SHALL 使用下表将 Clash Verge Script 的 rule-provider **键**（及 rules 中的 RULE-SET 名）映射到 `sources.yaml` 的 `name`，再写成 `geosite:<name>` 或 `geoip:<name>`。SHALL NOT 将下表左侧键映射为 MetaCubeX/标准近似名（如 `geolocation-!cn`、`category-ads-all`、`gfw` 作为 dat list 名）。

| Script / RULE-SET 键 | sources name | 输出前缀 |
|----------------------|--------------|----------|
| reject | loyalsoldier-reject | geosite |
| icloud | loyalsoldier-icloud | geosite |
| apple | loyalsoldier-apple | geosite |
| google | loyalsoldier-google | geosite |
| proxy | loyalsoldier-proxy | geosite |
| direct | loyalsoldier-direct | geosite |
| private | loyalsoldier-private | geosite |
| gfw | loyalsoldier-gfw | geosite |
| tld-not-cn | loyalsoldier-tld-not-cn | geosite |
| telegramcidr | loyalsoldier-telegramcidr | geoip |
| cncidr | loyalsoldier-cncidr | geoip |
| lancidr | loyalsoldier-lancidr | geoip |
| YouTube | xiaolin-youtube | geosite |
| Netflix | xiaolin-netflix | geosite；若存在 IP tag 则另写 geoip |
| Spotify | xiaolin-spotify | geosite |
| BilibiliHMT | xiaolin-bilibili | geosite；若存在 IP tag 则另写 geoip |
| TikTok | xiaolin-tiktok | geosite |
| AI | sukka-ai | geosite |
| applications | （跳过） | — |

内置兜底 `GEOSITE,CN` / `GEOIP,CN` / `GEOIP,LAN` 在 dat 模式下 SHALL 分别映射为 `geosite:cn` / `geoip:cn` / `geoip:private`（依赖轻量完整增强底，不映射为自定义 tag）。

`--dat` SHALL 强制读取构建产生的机器可读 tag manifest。映射表只表达 provider→tag 名关系；任何 tag 是否存在（包括 Netflix/Bilibili 的 IP 侧）均以 manifest 的 `required.geosite` / `required.geoip` 为准。输出中的每一个 geosite/geoip 引用 SHALL 能在该 manifest 对应侧找到。

#### Scenario: gfw 映射

- **GIVEN** 输入规则含 `RULE-SET,gfw,Proxy`
- **WHEN** `--dat` 转换
- **THEN** 输出含 `geosite:loyalsoldier-gfw`，且该 RULE-SET 的映射结果不是 `geosite:gfw`

#### Scenario: proxy 不映射为 geolocation-!cn

- **GIVEN** 输入含 `RULE-SET,proxy,...`
- **WHEN** `--dat` 转换
- **THEN** 输出含 `geosite:loyalsoldier-proxy`，且不含将该条映射为 `geosite:geolocation-!cn` 的结果

#### Scenario: reject / AI / telegramcidr

- **GIVEN** 输入分别含 `RULE-SET,reject`、`RULE-SET,AI`、`RULE-SET,telegramcidr`
- **WHEN** `--dat` 转换
- **THEN** 分别得到 `geosite:loyalsoldier-reject`、`geosite:sukka-ai`、`geoip:loyalsoldier-telegramcidr`

#### Scenario: Netflix 双字段

- **GIVEN** 输入含 `RULE-SET,Netflix,...` 且构建产物中存在 `geoip` list `xiaolin-netflix`
- **WHEN** `--dat` 转换
- **THEN** 对应分流规则 domain_list 含 `geosite:xiaolin-netflix`，ip_list 含 `geoip:xiaolin-netflix`

### Requirement: 安装脚本写入 URL 并覆盖导入分流规则

安装脚本 SHALL：

1. 将 `passwall2` 的 `global_rules.geoip_url` / `geosite_url` 设为  
   `https://github.com/<owner>/<repo>/releases/latest/download/geoip.dat` 与 `.../geosite.dat`（`<owner>/<repo>` 可配置，默认与本仓库发布一致）；
2. 以 `clash2passwall --dat` 的输出（或脚本生成的等价 UCI 片段）为输入，**删除既有 `shunt_rules` section 后重新导入**（覆盖导入），且 SHALL NOT 删除 `nodes` / 用户节点 section；
3. 在标准输出或注释中提示：使用 sing-box 内核时需要 `geoview >= 0.1.10`。

在公开仓库尚未建立时，脚本 SHALL 拒绝未配置或占位 owner/repo，而不是生成无效默认 URL。生成的 `shunt_rules` SHALL 为稳定具名 section，原 Clash 分组名保存在 `remarks`；不得依赖 UCI 为匿名 section 生成 `cfg...` ID。所有 YAML 派生字段 SHALL 拒绝 CR/LF/NUL/控制字符，安装载荷不得直接插入固定 heredoc。

安装 SHALL 在临时 UCI 配置中完成 URL、删除、导入与解析验证，随后原子替换真实配置；临时提交、载荷解码或最终提交失败时 SHALL 恢复备份。真实配置存在未提交 UCI 变更时 SHALL 拒绝覆盖。

#### Scenario: 写入 URL

- **GIVEN** 目标设备可写 UCI，且脚本参数指定 owner/repo
- **WHEN** 执行安装脚本
- **THEN** `geosite_url` / `geoip_url` 分别为上述 latest/download 形态

#### Scenario: 覆盖导入含 gfw 规则且保留节点

- **GIVEN** 设备上已有用户节点 section，以及旧的 shunt_rules
- **WHEN** 执行安装脚本完成导入
- **THEN** 至少存在一条 shunt 规则的 domain_list 含 `geosite:loyalsoldier-gfw`；且用户节点 section 仍存在

#### Scenario: geoview 提示

- **GIVEN** 执行安装脚本
- **WHEN** 查看脚本输出
- **THEN** 输出文本含 `geoview` 与 `0.1.10`（或「≥0.1.10」语义等价提示）

## 方案设计

### 架构与组件

```
sources.yaml
    → scripts/build.py
        → build/data/<tag>          # dlc 文本（自定义域名，带前缀行）
        → build/ip/<tag>.txt        # CIDR 文本
        → build/data-merged/        # community data ∪ 自定义（冲突则失败）
        → build/geoip-config.json
    → domain-list-custom --datapath=build/data-merged → publish/geosite.dat
    → geoip convert -c build/geoip-config.json       → publish/geoip.dat
    → sha256sum → publish/*.sha256sum
    → 探针：cn + 每个 sources 期望 tag 非空
    → GitHub Releases (latest/download)
         ↘ PassWall2 rule_update
         ↘ clash2passwall --dat + install_shunt_rules.sh
```

| 组件 | 职责 | 依赖 |
|------|------|------|
| `sources.yaml` | 启用源唯一清单 | 无 |
| `scripts/build.py` | 拉源、分桶、写 dlc/IP、合并 data、geoip config、编排、hash、探针 | PyYAML |
| domain-list-custom | community∪自定义 → geosite.dat | Go、community checkout |
| Loyalsoldier/geoip | 官方 dat 底 + 自定义 CIDR → geoip.dat | Go |
| GHA build.yml | 日更/手动；只发 4 个 release 资产 | contents:write |
| clash2passwall `--dat` | 上表映射 | sources 名 |
| 安装脚本 | URL + 覆盖导入 shunt_rules | OpenWrt UCI |

### 数据流

1. 读取 `sources.yaml` 全部启用源；跳过 applications。
2. 下载并解析为五桶；写 `domain:`/`full:`/`keyword:`/`regexp:` 行与 IP txt。
3. 合并 community `data/` 与自定义；冲突 assert。
4. 打包 geosite.dat；geoip convert 叠加自定义 IP。
5. sha256；探针失败则整体失败。
6. 更新 latest Release 四资产。
7. 下游 `--dat` 与安装脚本消费。

### 关键接口

**sources.yaml 项**

```yaml
- {name: <tag>, behavior: domain|ipcidr|classical, url: "...", format: yaml|text}
```

**domain-list-custom**

```text
go run ./ --datapath=build/data-merged --datname=geosite.dat --outputpath=publish
```

（plaintext/gfwlist 导出取实施时最小噪音合法参数。）

**geoip convert**

- input：`v2rayGeoIPDat`（官方 geoip.dat）+ 各 `text`（`name`=`tag`）
- output：`v2rayGeoIPDat` → `publish/geoip.dat`

**PassWall2**

- `global_rules.geoip_url` / `geosite_url`
- 文件名写死落盘 `v2ray_location_asset` 下的 `geoip.dat`/`geosite.dat`

**自定义 tag 真源**：始终以当前 `sources.yaml` 为准；文档中的 15/3 数字仅为说明快照。

### 错误处理

见 Requirement「构建失败条件」；另：keyword/regex 经 geoview 可能丢失 — 文档声明，不在本特性修复 geoview。

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|-------------------|------|---------|---------|
| sha256 校验 | integration | 任务内 | `sha256sum -c` OK |
| latest URL 与 sha 派生 | e2e | 验收 | HTTP 200 + 匹配 |
| Release 无 srs | integration | 验收 | asset 列表 |
| 标准 cn | integration | 探针 | 非空 |
| sources 逐项探针 | integration | 探针 | 全通过 |
| 域名源 / applications | unit | TDD | 断言 |
| Netflix / YouTube 拆分 | integration | TDD+探针 | 双 tag / 无 geoip list |
| dlc 前缀行 | unit | TDD | 黄金文件 |
| 404 / 撞名 fail-fast | integration | TDD | 非零退出 |
| gfw / proxy / reject / AI / telegramcidr 映射 | unit | TDD | 黄金文件 |
| Netflix 双字段 | unit | TDD | domain+ip |
| 安装 URL / 覆盖导入 / geoview 提示 | e2e/手工 | 验收 | UCI + 输出 |

## 风险与边缘情况

| 风险 | 缓解 |
|------|------|
| 误读「Loyalsoldier 全量」成 geosite 也用增强包 | 术语统一为「轻量完整增强底」 |
| sources 与文档清单漂移 | tag 真源仅 sources + 探针 |
| CI 无 remote | 文档建仓步骤 |
| Go 工具漂移 | 固定版本 |
| geoview 丢 keyword | 文档；xray 保留 |
| 覆盖导入误删节点 | 脚本只动 shunt_rules + 测试断言 |
| 定时更新 restart 断流 | 非本特性范围 |

## 开放问题

- domain-list-custom 关闭多余 plaintext/gfwlist 导出的精确 flag 组合：实施试跑决定。
- list 存在性探针具体 CLI（geoview / 自研解析 / 其他）：只要满足「可证明 list 存在且非空」即可。
- GitHub 仓库 `<owner>/<repo>` 最终远程名：本地路径保持 `clash-rules-srs`，远程可改名后回填安装脚本默认值。
