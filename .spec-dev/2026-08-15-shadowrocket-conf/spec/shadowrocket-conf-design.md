---
spec_dev:
  version: 1
  feature: shadowrocket-conf
  status: active
  covers:
    - "internal/shadowrocket/**"
    - "internal/verify/assets.go"
    - "internal/releasecmp/releasecmp.go"
    - "internal/app/build.go"
    - "internal/app/deps.go"
    - "internal/app/commands.go"
    - ".github/workflows/build.yml"
    - "README.md"
    - "context.md"
  sync_commit: null
  supersedes: []
  superseded_by: null
---

# Shadowrocket 胖配置 设计

## 背景与目标

本仓库已把 Clash 规则与 GeoSite/GeoIP 底包合成 PassWall2 可消费的 `geosite.dat` / `geoip.dat`。iOS Shadowrocket 没有 `GEOSITE` 语法，也不能加载这两个 dat。需要把同一套托管分流转成一份可远程导入、可自动更新的 Surge 风格配置。

**成功标准 / Success criteria**：一次完整构建在 `publish/` 中额外产出通过校验的 `shadowrocket.conf` 与其 SHA-256 文件；conf 内联 16 个托管组的完整规则与具名策略组；文档给出 HTTPS / 二维码 / scheme 三种导入方式；首次上线后仅 conf 内容变化也会触发发布。

## 非目标

- 拆分远程 `.list`、GitHub Pages、`.module` 模块分发
- 把 `geosite.dat` / `geoip.dat` 当作 Shadowrocket 输入
- 写入任何节点、地区筛选子组（如「香港节点」）
- 修改 PassWall2 安装器、UCI 托管组或两个 dat 的语义
- 让 App 校验 SHA-256
- 真机导入、自动更新与大 conf 编译耗时（DEFERRED，与现有路由器设备验收同级）

## 术语表

- **胖 conf / fat conf**：单份内联全部 `[Rule]` 的 Shadowrocket 配置文件，不通过 `RULE-SET` 引用外部规则集。_Avoid_：瘦 conf、规则集包
- **托管组 / managed group**：`config/passwall2-groups.yaml` 中的一组分流，字段为 `id`、`remarks`、`geosite`、`geoip`。_Avoid_：策略组（仅当特指 conf 里的 `[Proxy Group]` 条目时使用）
- **策略组 / policy group**：conf `[Proxy Group]` 中的具名 select 组，组名等于对应托管组的 `remarks`。_Avoid_：节点组、订阅组
- **最终 dat / compiled dat**：本次构建写入 `publish/` 的 `geosite.dat` 与 `geoip.dat`。_Avoid_：data-merged、增量 ip 文本（二者不是发布内容的规则源）

## 影响面

- 新增 `internal/shadowrocket` 渲染包
- `internal/app` 构建流水线增加 finish 阶段
- `internal/verify` 精确资产清单 6→8
- `internal/releasecmp` 指纹增加第四主资产
- `.github/workflows/build.yml` 与契约测试 / 文档中的「六资产」表述
- 不改 `internal/passwall` 安装语义；`ValidateReleaseTag` 若被复用应避免 iOS 包反向依赖 UCI 包（实施时可抽中立校验，属实现细节）

## 已确认的关键决策

- 交付形态：单份胖 conf + checksum，作为 Release 第 7、8 号资产 —— 与精确集合门禁合拍，且 App 无 dat 入口（详见 `../../adr/0001-shadowrocket-fat-conf.md`）
- 规则源：从编译后的最终 dat 按托管组 tag 展开 —— 含 include 与 GeoIP 底包，与路由器一致（详见 `../../adr/0002-expand-from-compiled-dat.md`）
- 策略：16 个具名 select 组；广告默认 REJECT 可改；中国大陆 / 私有网络 / 直连规则默认 DIRECT 可改；其余默认 PROXY 可改 DIRECT
- 不写入节点；`update-url` 指向 `releases/latest/download/shadowrocket.conf`；conf 不嵌入 `RELEASE_TAG`
- `regexp` 跳过并计数，不导致构建失败
- 导入：HTTPS 粘贴为必须，二维码与 `shadowrocket://config/add/{url}` 为文档增强
- 首次上线必须在默认分支 `force_publish=true` 一次

## 取代与共存

无相交 active spec。

## 行为规范（Requirements）

### Requirement: 构建产出八资产闭集

一次成功的 `geodata-build build`（非 `--skip-compile`）SHALL 使 `publish/` 恰好包含下列八个普通文件：`geoip.dat`、`geoip.dat.sha256sum`、`geosite.dat`、`geosite.dat.sha256sum`、`install_passwall2_rules.sh`、`install_passwall2_rules.sh.sha256sum`、`shadowrocket.conf`、`shadowrocket.conf.sha256sum`。`shadowrocket.conf.sha256sum` SHALL 为 GNU `sha256sum` 文本格式一行：64 位小写十六进制、两个空格、basename、换行，且重算 `shadowrocket.conf` 后与该摘要相等。

#### Scenario: 完整构建得到八文件且 checksum 匹配

- **GIVEN** 合法的 sources、groups、community 与 cache
- **WHEN** 执行完整 `geodata-build build`
- **THEN** `publish/` 文件名集合等于上述八个，且 `sha256sum -c publish/shadowrocket.conf.sha256sum` 成功

#### Scenario: 缺少 conf 时资产校验失败

- **GIVEN** 构建已写出原六资产但未写 `shadowrocket.conf`
- **WHEN** 运行发布资产校验
- **THEN** 校验失败，错误含 `missing` 与 `shadowrocket.conf`

#### Scenario: skip-compile 不产出 conf

- **GIVEN** 调用带 `--skip-compile`
- **WHEN** 构建返回成功
- **THEN** `publish/` 不被切换为含 `shadowrocket.conf` 的正式产物（与现有 skip-compile 不提交正式 publish 的行为一致）

### Requirement: 从最终 dat 展开托管组规则

`shadowrocket.conf` 的 `[Rule]` SHALL 按 `passwall2-groups.yaml` 数组顺序，为每个托管组写入该组全部 `geosite` 与 `geoip` tag 在**最终 dat** 中的规则。展开结果 SHALL 包含 community `include:` 展开后的域名，以及 `merge-base` GeoIP tag 的底包 CIDR。SHALL NOT 把未展开的 `data-merged` 或仅含增量的 `build/ip/<tag>.txt` 当作发布规则源。

#### Scenario: include 被展开进 google 组

- **GIVEN** 最终 `geosite.dat` 中 `google` 经由 `include:youtube` 纳入了 youtube 域名
- **WHEN** 生成 conf
- **THEN** `[Rule]` 里策略为「Google 服务」的行包含至少一条来自该 include 的域名规则

#### Scenario: merge-base GeoIP 含底包

- **GIVEN** 最终 `geoip.dat` 的 `cn` 含底包 CIDR 且大于本仓增量文件
- **WHEN** 生成 conf
- **THEN** 策略为「中国大陆」的 `IP-CIDR`/`IP-CIDR6` 行数不少于最终 dat 中 `cn` 的前缀数（按导出映射后）

#### Scenario: 展开失败则不发布

- **GIVEN** 某托管组引用的 tag 无法从最终 dat 读出规则
- **WHEN** 执行 render 阶段
- **THEN** 构建失败且 `publish/` 不提交残缺 conf

### Requirement: 规则类型映射与 regexp 跳过

导出时 SHALL 将最终 dat 中的域名规则映射为：`domain`（及隐式域名）→ `DOMAIN-SUFFIX`；`full` → `DOMAIN`；`keyword` → `DOMAIN-KEYWORD`。CIDR SHALL 写成 `IP-CIDR` 或 `IP-CIDR6` 并带 `no-resolve`。`regexp` SHALL 被跳过且不使构建失败。属性（如 `@cn`）SHALL 不写入 conf；同一 `kind+value` 只保留一行。

#### Scenario: 四种可映射类型与 CIDR

- **GIVEN** 某 tag 含 suffix、full、keyword 各一条及一条 IPv4 CIDR
- **WHEN** 生成该组规则
- **THEN** conf 中对应出现 `DOMAIN-SUFFIX`、`DOMAIN`、`DOMAIN-KEYWORD`、`IP-CIDR,...,no-resolve`，策略列为该组 remarks

#### Scenario: regexp 被跳过

- **GIVEN** 某 tag 含至少一条 `regexp`
- **WHEN** 生成 conf
- **THEN** `[Rule]` 中不出现该正则原文，构建成功

### Requirement: 十六个具名策略组与默认出口

conf SHALL 含 `[Proxy Group]` 段，为每个托管组生成一个 `select` 策略组，组名等于该组 `remarks`。所有策略组名 SHALL 唯一，且 SHALL NOT 与渲染器使用的内建策略名 `PROXY`、`DIRECT`、`REJECT`（不区分大小写）冲突。默认出口按 **group id** 决定，不按 remarks 文案匹配：`reject` 默认 `REJECT`；`cn`、`private`、`direct` 默认 `DIRECT`；其余 id 默认 `PROXY`。每组行 SHALL 含对应候选（广告另含 `PROXY` 与 `DIRECT`；国内三类另含 `PROXY`；其余另含 `DIRECT`），并写 `policy-select-name=<默认出口>`，不得只靠「列表第一项即默认」。`[Rule]` 中该组规则的策略列 SHALL 为该组 remarks。conf SHALL NOT 包含 `[Proxy]` 节点定义。

#### Scenario: 组名与顺序对齐 YAML

- **GIVEN** 仓库默认 `passwall2-groups.yaml`
- **WHEN** 生成 conf
- **THEN** `[Proxy Group]` 出现全部 16 个 remarks，且「YouTube」出现在「Google 服务」之前；`[Rule]` 中 YouTube 策略的规则也出现在 Google 服务规则之前

#### Scenario: 广告与国内默认

- **GIVEN** 默认托管组
- **WHEN** 读取 `[Proxy Group]`
- **THEN** id `reject` 对应行含 `policy-select-name=REJECT`；id `cn` 对应行含 `policy-select-name=DIRECT`；id `youtube` 对应行含 `policy-select-name=PROXY`

#### Scenario: 空组失败

- **GIVEN** 某托管组展开后零条规则
- **WHEN** 渲染 conf
- **THEN** 构建失败，错误含该组 remarks 或 id

#### Scenario: 重复或保留策略组名失败

- **GIVEN** 两个不同 group id 使用相同 remarks，或任一 remarks 等于 `PROXY`、`DIRECT`、`REJECT` 的大小写变体
- **WHEN** 渲染 conf
- **THEN** 构建失败，错误标明冲突的策略组名；不得发布含歧义或自引用策略的配置

### Requirement: 配置头与兜底规则

conf SHALL 含 `[General]`，其中 `update-url` 等于 `https://github.com/<构建 --repo>/releases/latest/download/shadowrocket.conf`。`[Rule]` 在全部托管组规则之后 SHALL 有且仅有一条 `GEOIP,CN,<中国大陆组 remarks>` 与一条 `FINAL,PROXY`。conf SHALL NOT 嵌入本次 `RELEASE_TAG`。

#### Scenario: latest update-url

- **GIVEN** `--repo FlameMida/c2p-rules`
- **WHEN** 生成 conf
- **THEN** 存在一行 `update-url = https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf`，且全文不含本次 `--release-tag` 的值

#### Scenario: 兜底顺序

- **GIVEN** 成功生成的 conf
- **WHEN** 读取 `[Rule]` 末尾
- **THEN** 倒数第二条为 `GEOIP,CN,中国大陆`，最后一条为 `FINAL,PROXY`

### Requirement: 仅 conf 变化也发布

发布指纹 SHALL 在现有 `geosite.dat`、`geoip.dat`、规范化安装器之外，纳入 `shadowrocket.conf` 的原始字节。`shadowrocket.conf.sha256sum` SHALL NOT 进入指纹。

#### Scenario: 只改 conf 判定 changed

- **GIVEN** 候选与基线的两个 dat 和规范化安装器相同，但 `shadowrocket.conf` 不同，且两侧均为完整八资产
- **WHEN** 运行 `release-decision --baseline`
- **THEN** 输出含 `should_publish=true` 与 `reason=changed`

#### Scenario: 只换 tag 不改 conf 判定 unchanged

- **GIVEN** 候选与基线仅安装器 `RELEASE_TAG` 不同，dat 与 conf 相同
- **WHEN** 运行 `release-decision --baseline`
- **THEN** 输出含 `should_publish=false` 与 `reason=unchanged`

#### Scenario: 六文件基线不得当成 changed

- **GIVEN** 候选为完整八资产，基线目录只有旧六资产
- **WHEN** 运行 `release-decision --baseline`
- **THEN** 命令失败（非零退出），标准错误含 `asset set mismatch` 与 `missing`，且不输出 `reason=changed`

### Requirement: 文档说明导入与首次强制发布

README 与 context.md SHALL 列出八个发布文件，并说明：用配置页粘贴 latest 的 `shadowrocket.conf` URL 导入；可用同一 URL 生成二维码；可用 `shadowrocket://config/add/` 接该 HTTPS 作为增强入口；镜像前缀不得写入 `update-url`；将发布集合从六文件切到八文件时必须在默认分支执行一次 `force_publish=true`。

#### Scenario: 文档含新资产与导入入口

- **GIVEN** 更新后的 README.md 与 context.md
- **WHEN** 运行文档契约测试
- **THEN** 两份文档均含 `shadowrocket.conf.sha256sum`、`force_publish`、`shadowrocket://config/add/`，且不得再把 Release 描述成「精确六个文件」或「三主资产」而不提 conf

## 方案设计

### 架构与组件

- **`internal/shadowrocket`**：只负责把「已展开的每组规则 + groups」写成 conf 字节。不下载、不调 geoview、不写盘。
- **展开器**（可与渲染同包或 `internal/shadowrocket/expand`）：输入最终 dat 路径与 groups，输出每组的域名/CIDR 列表。实现可调用已有 `geoview` 或等价只读展开，但必须满足「include 已展开、merge-base IP 含底包」。
- **`app.Dependencies.RenderShadowrocket`**：finish 阶段，位于 `RenderInstaller` 之后、`VerifySixAssets` 之前。读 `state.Groups`、`publish/*.dat`，调用展开器与 `Render`，`AtomicWrite` 0644，再 `verify.WriteSHA256`。
- **`verify.ReleaseAssets`**：闭集权威清单扩为八名。
- **`releasecmp.fingerprint`**：第四帧哈希 conf 原文。

不把 iOS 逻辑写入 `internal/passwall`。

### 数据流

```
prepare（现有）→ compile dat → ValidateGroups → ProbeTags
  → RenderInstaller（现有六资产中的脚本）
  → 从最终 dat 按组展开 → Render conf → WriteSHA256
  → verify.Assets(八文件) → Commit
```

映射后的规则行：`TYPE,value,<remarks>[,no-resolve]`。

### 关键接口

```go
type DomainLine struct {
    Kind  model.DomainKind // domain | full | keyword；不含 regexp
    Value string
}

type GroupRules struct {
    Group   model.Group
    Domains []DomainLine
    CIDRs   []netip.Prefix
}

func Expand(ctx context.Context, siteDat, ipDat string, groups []model.Group) ([]GroupRules, error)
func Render(groups []GroupRules, repo string) ([]byte, error)
```

`repo` 仅用于拼 `update-url`，须通过与安装器相同的仓库名规则（拒绝 `owner/repo` 占位）。`[General]` SHALL 含 `update-url`；MAY 另含与分流无关的稳妥键（如 `bypass-system`），golden 测试不得把「只有 update-url 一行」当成闭集。

### 错误处理

- 缺 tag / 展开失败 / 展开后空组 / 非法字符：阶段返回 error，事务 Abort，不切换正式 `publish/`
- regexp：跳过，可在日志打印计数；不失败
- 资产集合不匹配：现有 `verify.Assets` 失败
- 旧 latest 仍为六文件时 Compare 失败：必须 `force_publish`，不自动降级为「忽略新资产」

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|-------------------|------|---------|---------|
| 完整构建得到八文件且 checksum 匹配 | integration | 任务内 TDD | 集成测试通过 |
| 缺少 conf 时资产校验失败 | unit | 任务内 TDD | `verify.Assets` 测试 |
| skip-compile 不产出 conf | integration | 任务内 TDD | 与现有 skip-compile 断言并列 |
| include 被展开进 google 组 | unit 或 integration | 任务内 TDD | 夹具 dat 或最小 community 树 |
| merge-base GeoIP 含底包 | unit 或 integration | 任务内 TDD | cn 行数 > 增量文件 |
| 展开失败则不发布 | unit | 任务内 TDD | 阶段 error |
| 四种可映射类型与 CIDR | unit | 任务内 TDD | Render golden |
| regexp 被跳过 | unit | 任务内 TDD | 无该正则行 |
| 组名与顺序对齐 YAML | unit | 任务内 TDD | YouTube 在 Google 前 |
| 广告与国内默认 | unit | 任务内 TDD | select 首项 |
| 空组失败 | unit | 任务内 TDD | error 含组名 |
| 重复或保留策略组名失败 | unit | 任务内 TDD | Render 拒绝重复与内建策略名 |
| latest update-url | unit | 任务内 TDD | 无 tag 字面量 |
| 兜底顺序 | unit | 任务内 TDD | 末两行 |
| 只改 conf 判定 changed | unit | 任务内 TDD | releasecmp |
| 只换 tag 不改 conf 判定 unchanged | unit | 任务内 TDD | releasecmp |
| 六文件基线不得当成 changed | unit | 任务内 TDD | releasecmp |
| 文档含新资产与导入入口 | unit | 任务内 TDD | docs_contract |
| workflow 上传八文件 | unit | 任务内 TDD | workflow_contract |
| 真机导入与自动更新 | e2e | 验收任务 (D) | DEFERRED，记录于验收报告 |

## 风险与边缘情况

- 胖 conf 可能达数 MB：不设行数上限；集成测试只要求能写出。
- 自动更新会覆盖用户改过的 conf 正文：文档说明；组内当前节点选择通常由 App 侧保留，但不作为本仓契约。
- `shadowrocket://config/add/` 为社区手册语法：HTTPS 粘贴为必须入口。
- 用户尚未添加订阅时，select 组仍可用 PROXY/DIRECT/REJECT。
- 组间不去重：同一域名可按 YAML 顺序先中 YouTube 再中 Google。
- 二维码图片不进入八资产闭集，避免发布集合再膨胀。

## 开放问题

- 从最终 dat 抽出文本的具体 CLI（`geoview` 旗标 vs 自研解包）由实施任务用最小原型选定，以本 spec 的展开完整性 Scenario 为准。
- README 二维码是外链生成说明还是仓库内静态图，实施时选一种，不改变八资产集合。
