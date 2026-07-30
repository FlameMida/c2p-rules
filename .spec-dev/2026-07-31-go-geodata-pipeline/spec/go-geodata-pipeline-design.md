---
# —— spec-dev 漂移守卫锚点（机器可校验，勿删）——
spec_dev:
  version: 1
  feature: go-geodata-pipeline
  status: active
  covers:
    - "cmd/**"
    - "internal/**"
    - "go.mod"
    - "go.sum"
    - "sources.yaml"
    - "custom/**"
    - "config/**"
    - "scripts/**"
    - "tests/**"
    - "tools/clash2passwall/**"
    - "requirements.txt"
    - ".gitignore"
    - ".github/workflows/**"
    - "README.md"
    - "context.md"
  sync_commit: null
---

# 全 Go geodata 构建与 PassWall2 分流安装设计

## 背景与目标

现有实现以 Python 编排 geodata 构建、以 JavaScript 转换 Clash 配置，再生成 PassWall2 安装脚本；同时把 source 身份、输出 tag 和底包碰撞策略绑定在同一个名称上。新设计将第一方构建链统一为根目录单一 Go module，直接生成增强后的 `geosite.dat`、`geoip.dat`、严格 tag manifest 与声明式 PassWall2 分流安装脚本，并允许远程 source 和本地自定义规则显式合并进最终 tag。

**成功标准**：CI 无 Python/Node 运行时即可产出并验证六个 Release 资产；远程 source、本地 `custom/` 与底包按显式契约完成合并；安装脚本能幂等更新自身托管的 PassWall2 分流组、自动更新 dat，并在任何失败后恢复旧配置与旧 dat。

## 非目标

- 不实现 Clash 配置到 PassWall2 的通用转换器，也不保留 sing-box `.srs`、旧 `--xray` 或 `--dat` 转换模式。
- 不 fork 或复制 `domain-list-custom`、`Loyalsoldier/geoip`、`geoview` 的核心算法。
- 不自动选择代理节点或修改 `_shunt` 节点的出站选择；安装脚本只维护分流规则定义和 geodata 更新设置。
- 不把 Google 与 YouTube 做成互斥集合，也不做跨 tag 规则差集。
- 不改写既有 acceptance report；新实现生成独立验收证据。

## 术语表

- **Source ID**：远程规则来源的稳定身份，例如 `loyalsoldier-google`；不等于最终 dat tag。_Avoid_：自定义 tag、输出文件名。
- **输出目标**：`sources.yaml` 中某一 side 的 `{tag, mode}` 声明。_Avoid_：沿用 `sides` 推导 tag。
- **最终 tag**：完整构建后可由 dat 实际探针确认存在的 geosite/geoip list 名。
- **托管分流组**：安装脚本生成、带 `managed_by=clash-rules-srs` 标记的 PassWall2 `shunt_rules` section。_Avoid_：用户分流组、代理出站节点。
- **本地自定义规则**：`custom/geosite/*.yaml` 或 `custom/geoip/*.yaml` 中由用户维护、合入同名最终 tag 的 Clash YAML 规则。

## 影响面

| 范围 | 变化 |
|---|---|
| 构建入口 | Python 脚本替换为根目录 Go module 与 `geodata-build` CLI |
| source 契约 | `sides` 替换为显式 `outputs.<side>.tag/mode` |
| 本地规则 | 新增 `custom/geosite/`、`custom/geoip/` Clash YAML 接口 |
| geodata 工具 | 固定 commit 的上游 Go CLI 迁入 `.cache/upstream/`，二进制放 `.cache/bin/` |
| PassWall2 | 删除 clash2passwall；由 `config/passwall2-groups.yaml` 生成独立事务安装脚本 |
| 发布 | 四个 dat/sha 资产扩展为 dat/sha + installer/sha 六个资产 |
| CI 与文档 | 删除 Python、Node、npm 命令与依赖，改为 Go 测试、构建、探针和安装器验收 |

## 已确认的关键决策

- Source ID 与输出目标分离；输出显式选择 `create` 或 `merge-base`，旧前缀 tag 一次性移除（见 `../../adr/0003-source-output-targets.md`）。
- 根目录采用单一 Go module；临时上游目录由 `vendor/` 改为 `.cache/`，固定上游编译器继续作为 CLI 子进程使用（见 `../../adr/0004-go-orchestrator-pinned-tools.md`）。
- `sources.yaml` 只接受新 `outputs` schema，不兼容旧 `sides`，避免两个真源。
- 本地自定义规则只接受 Clash YAML；文件名对应已存在的最终 tag，不允许通过 `custom/` 隐式创建新 tag。
- PassWall2 分流组由独立 YAML 声明，构建器严格验证全部 tag 后生成 shell；不再解析 Clash 配置。
- 安装脚本只替换自身托管的 section，保留用户规则和节点；自动更新 dat、校验嵌入 SHA，失败恢复配置和 dat。
- 默认分流按逻辑服务拆分，具体服务优先于宽泛规则，YouTube 必须位于 Google 前。

## ADDED Requirements

### Requirement: 本地 Clash YAML 可扩展任一最终 tag

构建器 SHALL 将 `custom/geosite/<tag>.yaml` 与 `custom/geoip/<tag>.yaml` 中的合法 Clash 规则合入同 side 的既有最终 tag，并在目标不存在、side 与规则类型不符或规则非法时于调用最终编译器前失败。

#### Scenario: 扩展远程 source 创建的 BilibiliHMT

- **GIVEN** 远程 source 声明创建 `geosite:BilibiliHMT`，且 `custom/geosite/BilibiliHMT.yaml` 含 `DOMAIN-SUFFIX,example.test`
- **WHEN** 执行完整构建
- **THEN** 最终 `geosite:BilibiliHMT` 能匹配该后缀，且普通 `geosite:bilibili` 未增加该规则

#### Scenario: 拒绝拼错的本地目标

- **GIVEN** `custom/geosite/googel.yaml` 存在，但最终目标集合不存在 `geosite:googel`
- **WHEN** 构建器校验本地规则
- **THEN** 构建非零退出并指出文件路径与未知目标，既有 publish 目录保持不变

### Requirement: 仓库提供带规则说明的空自定义模板

仓库 SHALL 提供默认 `custom/geosite/apple.yaml` 与 `custom/geoip/cn.yaml`，以注释列出对应目录支持的 Clash 匹配规则，且默认内容为语义空集。

#### Scenario: 未编辑模板不改变产物

- **GIVEN** 两个默认模板保持初始内容
- **WHEN** 分别在有模板和无模板 fixture 上构建
- **THEN** 对应最终 tag 的规则语义一致，模板不会注入示例域名或 CIDR

### Requirement: 声明式配置生成有序 PassWall2 分流组

构建器 SHALL 按 `config/passwall2-groups.yaml` 的数组顺序生成具名 `shunt_rules`，在生成前以最终 dat 实际探针验证每个 geosite/geoip 引用，并在任一声明引用不存在时使构建失败。

#### Scenario: 苹果服务包含声明的 tag

- **GIVEN** `苹果服务` 声明 `geosite: [apple, icloud]`
- **WHEN** 构建安装脚本
- **THEN** 解码后的 UCI 片段包含一个 remarks 为 `苹果服务` 的具名 section，且 domain list 同时包含 `geosite:apple` 与 `geosite:icloud`

#### Scenario: 缺失 tag 阻断脚本发布

- **GIVEN** 某分流组声明 `geoip:not-exist`
- **WHEN** 完整构建执行分流引用探针
- **THEN** 构建非零退出，错误同时指出分流组和 `geoip:not-exist`，publish 不出现新安装脚本

### Requirement: 默认分流组按逻辑服务提供

默认配置 SHALL 分别提供广告拦截、苹果服务、Google 服务、代理规则、直连规则、私有网络、GFW、非中国域名、Telegram、中国大陆、YouTube、Netflix、Spotify、哔哩哔哩港澳台、TikTok 与 AI 服务；同一业务的多个 tag MAY 合入同一组，互不相干的具体服务不聚合为单一代理组。

#### Scenario: YouTube 优先于 Google

- **GIVEN** 默认 groups 配置未修改
- **WHEN** 生成托管分流 section
- **THEN** YouTube section 位于 Google 服务 section 前，且两组保持独立

#### Scenario: 中国大陆具备域名与 IP 规则

- **GIVEN** 底包包含 `geosite:cn` 且合并结果包含 `geoip:cn`
- **WHEN** 生成中国大陆分流组
- **THEN** 该 section 同时包含 `geosite:cn` 与 `geoip:cn`

### Requirement: 安装脚本自动更新并验证 geodata

安装脚本 SHALL 先以本次不可变 Release tag 的 URL 事务性写入 UCI，同步调用 `/usr/share/passwall2/rule_update.lua` 更新 geosite/geoip，以构建时嵌入的两个 SHA-256 校验实际 dat，成功后把持久更新 URL 切换为同仓库 `latest`，并在任一步失败时恢复执行前的 PassWall2 配置与两个 dat。

#### Scenario: 更新器成功退出但 dat 哈希错误

- **GIVEN** PassWall2 更新器返回成功但落盘 `geosite.dat` 与脚本嵌入哈希不符
- **WHEN** 安装脚本执行更新后校验
- **THEN** 脚本非零退出，旧配置、旧 geosite.dat 与旧 geoip.dat 均被恢复

#### Scenario: 完整成功后保留可恢复备份

- **GIVEN** UCI 无未提交修改且 Release 六资产互相匹配
- **WHEN** 执行安装脚本
- **THEN** 新 URL、托管分流组和两个新 dat 生效，脚本报告备份位置并零退出

## MODIFIED Requirements

### Requirement: 安装脚本写入 URL 并托管更新分流规则（替换全量覆盖导入）

安装脚本 SHALL 从 `config/passwall2-groups.yaml` 的已验证结果生成稳定具名 section，只删除或替换带 `managed_by=clash-rules-srs` 标记的当前托管 `shunt_rules` 及旧转换器保留的 `c2p_` 命名空间 section，保留用户创建的其他分流组、全部节点和其他 section，让用户分流组保持原顺序且位于重新追加的托管组之前，并在成功后把 geosite/geoip 持久更新 URL 设为同仓库 `latest`；脚本拒绝未提交 UCI 修改、占位仓库名与控制字符，使用临时 UCI 配置验证后再替换真实配置，并继续提示 sing-box 需要 `geoview >= 0.1.10`。

#### Scenario: 重复安装保持用户配置与托管组幂等

- **GIVEN** PassWall2 同时包含用户分流组、节点和一套旧托管组
- **WHEN** 连续执行同一版本安装脚本两次
- **THEN** 用户分流组与节点字节语义不变，每个托管 ID 仅存在一次，托管组顺序与 groups 配置一致

#### Scenario: 首次安装清理旧转换器分流

- **GIVEN** PassWall2 含旧 clash2passwall 生成的 `c2p_` 具名分流、一个非 `c2p_` 用户分流和节点
- **WHEN** 首次执行新安装脚本
- **THEN** 旧 `c2p_` 分流被新托管组替换，用户分流与节点保留，后续执行不再触发旧命名空间迁移

### Requirement: source 身份与输出目标显式解耦（替换 tag 等于 source name）

每个启用 source SHALL 以唯一 Source ID 标识，通过 `outputs.geosite` / `outputs.geoip` 分别声明最终 tag 与 `create|merge-base` 模式，并拒绝旧 `sides` 字段、隐式同名 tag 与双 schema。

#### Scenario: Google 合并标准 tag

- **GIVEN** Source ID 为 `loyalsoldier-google`，输出声明为 `{tag: google, mode: merge-base}`
- **WHEN** 构建
- **THEN** 自定义规则进入唯一的 `geosite:google`，不生成 `geosite:loyalsoldier-google`

#### Scenario: create 与 merge-base 前置条件严格执行

- **GIVEN** create 目标已存在于底包，或 merge-base 目标不存在于底包
- **WHEN** 校验 outputs
- **THEN** 构建在写入目标文件前失败并指出 source、side、tag 和违反的 mode

### Requirement: 产物包含轻量完整增强底与 outputs 最终 tag（替换 sources 自定义 tag）

最终 `geosite.dat` 与 `geoip.dat` SHALL 保留既有 community/官方增强底，并包含每个启用 source 在 `outputs` 中声明的非空最终 tag；期望集合从 outputs 与显式底包必需项生成，不再从 Source ID 推导。

#### Scenario: 对全部 outputs 逐侧探针

- **GIVEN** 当前 `sources.yaml` 的所有启用 source 均成功解析
- **WHEN** 完整构建结束
- **THEN** 每个 outputs 声明在对应 dat 中存在且非空，`geosite:cn`、`geoip:cn` 与 `geoip:private` 仍存在

### Requirement: classical 混合源按逐侧 outputs 拆分（替换同名 tag）

classical source SHALL 将域名 bucket 写入显式 `outputs.geosite.tag`、将 IP-CIDR/IP-CIDR6 bucket 写入显式 `outputs.geoip.tag`，并要求实际非空 side 与 outputs side 精确一致，而不再使用 Source ID 作为两侧同名 tag。

#### Scenario: Netflix 合入标准双侧 tag

- **GIVEN** `xiaolin-netflix` 同时解析出域名与 CIDR，outputs 分别声明 `geosite:netflix` 与 `geoip:netflix`
- **WHEN** 构建完成
- **THEN** 两个标准 `netflix` tag 均非空，且两侧都不存在 `xiaolin-netflix`

### Requirement: 构建失败条件与 fail-fast 发布门禁（允许显式 merge-base）

构建进程 SHALL 在远程源下载或解析失败、官方 GeoIP 底包不可达或无效、`create|merge-base` 前置条件不成立、未声明碰撞、本地 custom 非法、声明 side 为空、分流引用缺失、外部工具失败或最终探针失败时非零退出，并保持既有 publish 不变；只有显式 `merge-base` 的同名底包目标允许继续合并。

#### Scenario: 未声明的 community 碰撞仍失败

- **GIVEN** 一个 create 输出或未授权输出与 community data 同名
- **WHEN** 构建器校验 target registry
- **THEN** 构建在调用 domain-list-custom 前非零退出，错误指出碰撞 source 与 tag，旧 publish 不变

#### Scenario: 上游源失败不发布

- **GIVEN** 任一启用 source 下载返回非成功状态或 payload 无法解析
- **WHEN** 执行完整构建
- **THEN** 构建非零退出，不切换 build/publish，也不创建新 Release artifact

### Requirement: 同 tag 安全并集合并（替换 geosite 全碰撞失败）

构建器 SHALL 在同一 geosite 目标内对规范化后完全相同的规则去重、保留不同匹配类型/属性/keyword/regexp，并让 GeoIP 同名目标通过 IPSet 形成底包、远程 source 与本地 custom 的 CIDR 并集。

#### Scenario: Geosite 精确去重不扩大语义

- **GIVEN** 底包和两个输入分别含相同 `domain:example.com`，另含 `full:example.com` 与带属性的同值规则
- **WHEN** 合并目标
- **THEN** 完全相同的 domain 规则仅保留一次，full 与属性不同规则仍存在

#### Scenario: GeoIP 重叠 CIDR 规范化

- **GIVEN** 底包目标含 `/24`，自定义输入含同一 `/24` 及其内部 `/25`
- **WHEN** 编译最终 geoip.dat
- **THEN** 最终同名 list 能匹配整个 `/24`，且不生成旧前缀 list

### Requirement: Google 与 YouTube 允许重叠并靠优先级分流（替换跨 tag 去重设想）

构建 SHALL 保留 Google 与 YouTube 各自完整的 tag 内并集，不尝试从 Google 删除能匹配 YouTube 的父域、keyword 或 regexp，并让默认 PassWall2 分流顺序中 YouTube 高于 Google。

#### Scenario: Google 父域不被破坏

- **GIVEN** Google 含 `domain:googleapis.com`，YouTube 含 `full:youtubei.googleapis.com`
- **WHEN** 构建并生成默认分流
- **THEN** 两条规则分别保留在各自 tag，且 YouTube section 先于 Google 服务 section

### Requirement: BilibiliHMT 独立于普通 bilibili

构建 SHALL 创建大小写保真的 `geosite:BilibiliHMT` 与 `geoip:BilibiliHMT`，让普通 `Bilibili`/`bilibili` 语义继续指向底包 `geosite:bilibili`，且两者不得互相合并。

#### Scenario: 港澳台规则不污染普通 bilibili

- **GIVEN** BilibiliHMT source 含一条 HMT 专有域名与 CIDR
- **WHEN** 构建完成
- **THEN** 两条规则只出现在 `BilibiliHMT` 对应 side，普通 `geosite:bilibili` 不匹配该专有域名

### Requirement: Release 资产扩展为严格六项

每次 Release SHALL 精确包含 `geosite.dat`、`geoip.dat`、两个对应 `.sha256sum`、`install_passwall2_rules.sh` 与 `install_passwall2_rules.sh.sha256sum`，且 publish job 在公开 Release 前回读并验证集合与三份校验文件。

#### Scenario: 草稿 Release 六资产回读

- **GIVEN** build job 交付一个已验证 artifact
- **WHEN** publish job 上传并回读草稿 Release
- **THEN** 资产名集合精确等于六项，三次 `sha256sum -c` 均成功后才公开并切换 latest

### Requirement: 第一方构建链统一为 Go

仓库 SHALL 通过根目录 Go module 提供 `geodata-build bootstrap|build|verify`，让 CI 与文档不再要求 Python、Node 或 npm，并只从 `.cache/bin/` 或显式测试替身调用受超时控制的固定上游工具。

#### Scenario: 干净环境完成全链路

- **GIVEN** 环境只提供固定 Go 版本、git 与标准 shell 工具，未安装 Python、Node、npm 或全局 geoip/geoview
- **WHEN** 按文档运行 bootstrap、Go 测试与完整构建
- **THEN** 六个 publish 资产全部生成并通过 required/forbidden tag、引用、哈希与资产集合验证

### Requirement: 构建输出以 staging 成功切换

完整构建 SHALL 先在同文件系统 staging 中完成下载、合并、编译、探针、脚本生成和六资产验证，任一步失败时保留上一次成功的 `build/` 与 `publish/`，全部成功时再执行可恢复目录切换。

#### Scenario: 最后一个探针失败不破坏旧发布物

- **GIVEN** publish 中存在一套已成功资产，且新构建在 forbidden tag 探针阶段失败
- **WHEN** 构建退出
- **THEN** publish 六资产的字节与执行前完全一致，staging 被清理

## REMOVED Requirements

### Requirement: clash2passwall dat 模式使用固定 provider→tag 映射

移除原因：系统统一使用本项目 dat 与声明式 PassWall2 分流配置，不再保留 sing-box、xray、dat 三种转换模式及其 JavaScript/Go CLI。

### Requirement: Python 与 Node 构建运行时

移除原因：第一方构建、探针、manifest 引用校验和 installer 生成全部由 Go module 提供。

## 方案设计

### 架构与组件

```text
cmd/geodata-build
  ├─ bootstrap → internal/tools       固定 checkout 与二进制构建
  ├─ build     → internal/app         全流程编排
  └─ verify    → internal/verify      dat、引用、资产、hash 探针

internal/
  sourcecfg    严格 sources.yaml 与 output target registry
  customrules  custom Clash YAML 发现、解析、side 校验
  rules        五类 domain bucket、CIDR 与规范化
  geosite      create/merge-base、文本合并与安全去重
  geoip        base dat 本地化、text inputs 与 geoip config
  manifest     required/forbidden/source provenance
  passwall     groups schema、UCI renderer、installer renderer
  tools        CommandContext、固定路径、超时与有界日志
  workspace    staging、原子文件写入、可恢复目录切换
```

仓库不暴露公共 Go library，全部业务包置于 `internal/`。`domain-list-custom` 因无可导入 API，`geoip` 因 plugin 注册与依赖耦合，均保持固定 commit CLI 边界；`geoview` 继续作为实际 dat 探针。

### 数据流

```text
sources.yaml ─→ 严格校验 ─→ 下载/解析 ─┐
community data ────────────────────────┼→ target registry → geosite 合并树
custom/geosite/*.yaml ────────────────┘                       ↓
                                                   domain-list-custom

官方 geoip.dat ─→ 本地受控下载 ─────────┐
远程 IP buckets ───────────────────────┼→ geoip config → pinned geoip
custom/geoip/*.yaml ───────────────────┘

两个 dat → required/forbidden 实际探针 → manifest
manifest + passwall2-groups.yaml → 引用探针 → installer
dat + installer → sha256sum → 精确六资产 → publish 切换
```

GeoSite 合并顺序固定为底包、按 Source ID 排序的远程输入、按路径排序的本地输入；规范化 key 包含匹配类型、值和完整属性。GeoIP config 保证 base input 第一，后续 inputs 按目标 tag 与来源稳定排序。

### 关键接口

`sources.yaml` 新 schema：

```yaml
sources:
  - id: loyalsoldier-google
    behavior: domain
    format: yaml
    url: https://example/rules.yaml
    outputs:
      geosite:
        tag: google
        mode: merge-base
```

`create` 要求底包不存在目标；`merge-base` 要求底包已存在目标。一个 source 可声明一个或两个 side，实际解析 side 必须与 outputs 精确一致。

完整 source 输出契约：

| Source ID | geosite 输出 | geoip 输出 |
|---|---|---|
| `loyalsoldier-reject` | `reject` / create | — |
| `loyalsoldier-icloud` | `icloud` / merge-base | — |
| `loyalsoldier-apple` | `apple` / merge-base | — |
| `loyalsoldier-google` | `google` / merge-base | — |
| `loyalsoldier-proxy` | `proxy` / create | — |
| `loyalsoldier-direct` | `direct` / create | — |
| `loyalsoldier-private` | `private` / merge-base | — |
| `loyalsoldier-gfw` | `gfw` / create | — |
| `loyalsoldier-tld-not-cn` | `tld-not-cn` / create | — |
| `loyalsoldier-telegramcidr` | — | `telegram` / merge-base |
| `loyalsoldier-cncidr` | — | `cn` / merge-base |
| `loyalsoldier-lancidr` | — | `private` / merge-base |
| `xiaolin-youtube` | `youtube` / merge-base | — |
| `xiaolin-netflix` | `netflix` / merge-base | `netflix` / merge-base |
| `xiaolin-spotify` | `spotify` / merge-base | — |
| `xiaolin-bilibili` | `BilibiliHMT` / create | `BilibiliHMT` / create |
| `xiaolin-tiktok` | `tiktok` / merge-base | — |
| `sukka-ai` | `ai` / create | — |

本地规则 schema：

```yaml
payload:
  - DOMAIN-SUFFIX,example.com
  - DOMAIN,api.example.com
```

geosite 只支持 `DOMAIN-SUFFIX`、`DOMAIN`、`DOMAIN-KEYWORD`、`DOMAIN-REGEX`；geoip 只支持 `IP-CIDR`、`IP-CIDR6`，可带唯一尾随属性 `no-resolve`。

分流组 schema：

```yaml
groups:
  - id: apple_services
    remarks: 苹果服务
    geosite: [apple, icloud]
    geoip: []
```

`id` 必须是稳定、合法、唯一的 UCI section ID 片段；renderer 加仓库命名空间并写入 `managed_by` 标记。数组顺序为托管组优先级；用户既有组保留原顺序并整体优先。

默认托管组精确顺序与引用：

| 顺序 | remarks | geosite | geoip |
|---:|---|---|---|
| 1 | 广告拦截 | `reject` | — |
| 2 | 哔哩哔哩港澳台 | `BilibiliHMT` | `BilibiliHMT` |
| 3 | YouTube | `youtube` | — |
| 4 | Netflix | `netflix` | `netflix` |
| 5 | Spotify | `spotify` | — |
| 6 | TikTok | `tiktok` | — |
| 7 | AI 服务 | `ai` | — |
| 8 | 苹果服务 | `apple`, `icloud` | — |
| 9 | Telegram | — | `telegram` |
| 10 | Google 服务 | `google` | — |
| 11 | GFW | `gfw` | — |
| 12 | 代理规则 | `proxy` | — |
| 13 | 非中国域名 | `tld-not-cn` | — |
| 14 | 私有网络 | `private` | `private` |
| 15 | 中国大陆 | `cn` | `cn` |
| 16 | 直连规则 | `direct` | — |

CLI：

```text
geodata-build bootstrap [--cache-root .cache]
geodata-build build [--sources sources.yaml] [--custom custom]
                     [--groups config/passwall2-groups.yaml]
                     [--community .cache/upstream/domain-list-community/data]
                     [--work-root PATH] [--repo OWNER/REPO]
                     [--release-tag TAG] [--skip-compile]
geodata-build verify --dat FILE --manifest FILE --side geosite|geoip [--forbid]
```

CI 生成正式 installer 时必须提供真实 `OWNER/REPO` 和即将创建的不可变 Release tag；publish job 必须创建同名 tag。正式 Release 禁止占位仓库名或只绑定 `latest` 的初次安装 URL。

### 错误处理

- 三份仓库自有 YAML 均拒绝未知字段、重复 mapping key、多 document、非法 scalar 与控制字符；远程 payload 只严格拥有 `payload` 的类型与规则内容，不因无关 metadata 失败。
- HTTP 对连接、响应头和总请求设 deadline，限制 redirect、禁止 HTTPS 降级并按文本/base dat 分别设置大小上限；不使用 stale cache 降级发布。
- 外部命令不经过 shell，必须使用固定路径、显式 cwd、deadline 与有界 stderr；PATH 中同名程序不得覆盖固定工具。
- installer 不信任 `rule_update.lua` 的退出码，必须以真实 dat SHA 判定成功；配置或 dat 恢复失败时输出明确的人工恢复路径并非零退出。
- 旧前缀 tag、`applications` 及明确废弃 tag 进入 manifest forbidden；底包可能存在但本 source 未声明的跨侧同名 tag 不得被机械列为 forbidden。

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|---|---|---|---|
| 扩展远程 source 创建的 BilibiliHMT | integration | 任务内 TDD | synthetic registry、dat 正负 probe |
| 拒绝拼错的本地目标 | unit/integration | 任务内 TDD | 错误路径与旧 publish 字节断言 |
| 未编辑模板不改变产物 | unit | 任务内 TDD | 有/无模板输出等价 |
| 苹果服务包含声明的 tag | unit | 任务内 TDD | UCI golden |
| 缺失 tag 阻断脚本发布 | integration | 任务内 TDD | fake geoview 与 publish 保持断言 |
| YouTube 优先于 Google | unit/integration | 任务内 TDD | groups 校验与 UCI order golden |
| 中国大陆具备域名与 IP 规则 | integration | 任务内 TDD | 双侧引用 probe |
| 重复安装保持用户配置与托管组幂等 | e2e | 验收任务 (D) | fake-UCI 两次执行前后对比 |
| 首次安装清理旧转换器分流 | e2e | 验收任务 (D) | `c2p_` 迁移 fixture 与节点保留证据 |
| 更新器成功退出但 dat 哈希错误 | e2e | 验收任务 (D) | 故障注入后配置/dat 原字节恢复 |
| 完整成功后保留可恢复备份 | e2e | 验收任务 (D) | UCI、dat hash 与备份路径证据 |
| Google 合并标准 tag | integration | 任务内 TDD | 新 tag 正探针与旧 tag 负探针 |
| create 与 merge-base 前置条件严格执行 | unit | 任务内 TDD | 两类非法 registry table tests |
| 对全部 outputs 逐侧探针 | integration | 任务内 TDD | manifest 集合与 real-tool 非空 probe |
| Netflix 合入标准双侧 tag | integration | 任务内 TDD | 双侧新 tag 正探针与旧 tag 负探针 |
| 未声明的 community 碰撞仍失败 | unit/integration | 任务内 TDD | registry 错误与旧 publish 保持断言 |
| 上游源失败不发布 | integration | 任务内 TDD | HTTP/解析故障注入与目录不切换断言 |
| Geosite 精确去重不扩大语义 | unit/integration | 任务内 TDD | 合并文本与 dat probe |
| GeoIP 重叠 CIDR 规范化 | integration | 任务内 TDD | synthetic base dat + address probe |
| Google 父域不被破坏 | integration | 任务内 TDD | 两 tag 内容 probe + UCI order golden |
| 港澳台规则不污染普通 bilibili | integration | 任务内 TDD | 双侧正负 probe |
| 草稿 Release 六资产回读 | release | 验收任务 (D) | GitHub API 资产清单与三次 sha 校验 |
| 干净环境完成全链路 | e2e | 验收任务 (D) | 无 Python/Node 的 CI 日志与六资产 |
| 最后一个探针失败不破坏旧发布物 | integration | 任务内 TDD | staging 故障注入与目录字节对比 |
| PassWall2 真实设备更新 | operational | 验收任务 (D) | 设备 UCI、dat hash、xray/sing-box 查询证据；无设备时标记 deferred |

## 风险与边缘情况

- community 与官方 geoip 是滚动底包；`merge-base` 目标或 groups 引用消失会让当日构建失败，这是防止静默漂移的预期行为。
- Google 的父域能覆盖部分 YouTube 子域，dat 无负规则可表达完整差集；安全边界是 tag 内去重与分流优先级，不宣称集合互斥。
- `BilibiliHMT` 混合大小写必须在 manifest、geoview、Xray 与 sing-box 路径上实际验收，不在中间 map 中统一转小写。
- 用户分流组整体优先于托管组；若用户规则过宽，可能遮蔽托管服务规则，installer 应在完成摘要中明确提示这一优先级。
- PassWall2 版本可能缺少预期更新器、UCI section 或 asset 路径；installer 在修改前检测并失败，不尝试兼容未知 fork。
- installer 初次下载 dat 使用同一次构建的不可变 Release tag，避免 `latest` 切换竞态；校验成功后才持久化 `latest` URL，后续定时更新仍由 PassWall2 的伴随 sha 校验保护。

## 开放问题

无重大开放问题。具体 Go package 文件拆分、下载大小上限数值和测试 fixture 命名由实施计划在上述行为边界内确定。
