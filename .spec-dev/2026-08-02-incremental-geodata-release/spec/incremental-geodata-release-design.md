---
# —— spec-dev 漂移守卫锚点（机器可校验，勿删）——
spec_dev:
  version: 1
  feature: incremental-geodata-release
  status: draft
  covers:
    - "cmd/geodata-build/**"
    - "internal/app/**"
    - "internal/releasecmp/**"
    - ".github/workflows/build.yml"
    - "README.md"
    - "context.md"
  sync_commit: null
---

# Geodata 有效变化增量发布设计

## 背景与目标

当前 GitHub Actions 每天完整构建并无条件创建一个新的 geodata Release。即使全部规则、dat 和安装器逻辑都没有变化，运行级 Release tag 仍会改变安装器字节及其 checksum，造成无意义 Release。

本特性保留每日完整构建和全部发布门禁，只在消费者可观察的有效产物发生变化时创建新 Release；手动运行可显式强制发布。

**成功标准**：内容不变的定时运行成功结束且不创建 tag、Artifact 或 Release；任一 dat、分流组或安装器逻辑变化时仍发布严格六资产；首次运行、强制发布、不可信基线和 GitHub 错误均有确定且可验证的行为。

## 非目标

- 不通过 ETag、Last-Modified 或上游 commit 判断是否发布。
- 不减少上游下载、编译、探针或候选六资产验证步骤。
- 不自动更新 `domain-list-custom`、`geoip` 或 `geoview` 的固定 commit。
- 不增加第七个 Release 资产，也不改变 PassWall2 的下载 URL 契约。
- 不把真实 GitHub Release 验收或 OpenWrt 设备验收伪装成本地已通过。

## 术语表

- **候选发布载荷**：本次完整构建产生并通过严格六资产验证的目录。
- **主资产**：`geosite.dat`、`geoip.dat` 与 `install_passwall2_rules.sh`。
- **派生资产**：三个与主资产一一对应的 `.sha256sum` 文件。
- **发布绑定字段**：安装器中唯一合法的 `RELEASE_TAG='...'` 赋值；它只绑定本次不可变下载地址。
- **规范化主资产**：两个 dat 的原始字节，加上只将发布绑定字段值替换为固定哨兵后的安装器字节。
- **发布基线**：当前 GitHub latest Release 中通过严格六资产与 checksum 验证的载荷。
- **有效产物变化**：候选发布载荷与发布基线的规范化主资产不相等。
- **不可信基线**：latest 存在，但资产集合、checksum 或安装器结构不满足本项目契约。

## 影响面

| 范围 | 变化 |
|---|---|
| Go CLI | 新增 `geodata-build release-decision` |
| 比较逻辑 | 新增严格六资产验证、安装器规范化和流式内容比较 |
| GitHub Actions | 新增手动强制输入、只读变化判定、job output 与条件发布 |
| 发布竞态 | 普通增量发布在写入前复核 latest 身份 |
| 测试 | 增加比较器、CLI 与 workflow 结构契约测试 |
| 文档 | 说明每日检测、无变化跳过及人工修复方式 |

## 已确认的关键决策

- 以规范化三主资产作为发布等价键；只比较两个 dat 会漏掉安装器或分流组变化，metadata 又不能证明最终产物变化。
- 比较器属于现有 Go CLI，不增加 Shell 脚本或第三方依赖，以便用表驱动测试验证严格语义。
- 不可信基线默认严格失败；只有人工 `force_publish=true` 才允许跳过基线并发布已验证候选。
- `force_publish` 只覆盖内容比较，不跳过候选构建、探针、checksum、六资产回读或默认分支限制。
- 发布前复核比较时的 latest 身份；普通运行发现基线被外部替换时失败并等待重跑。

## ADDED Requirements

### Requirement: 仅在有效产物变化时发布

默认 CI SHALL 在候选发布载荷通过全部构建门禁后，将其规范化主资产与可信发布基线比较；只有首次发布或存在有效产物变化时才上传供 publish job 消费的 Artifact、创建 tag 和 Release，并移动 latest。

#### Scenario: 相同内容重复运行

- **GIVEN** 当前 latest 为可信发布基线，且本次候选与其规范化主资产相同
- **WHEN** 定时 CI 完成构建与变化判定
- **THEN** build job 成功，`should_publish=false`，不上传 Artifact，publish job 跳过，远端 tag、Release 与 latest 均不变

#### Scenario: 任一 dat 发生变化

- **GIVEN** 候选六资产有效，且任一候选 dat 与发布基线字节不同
- **WHEN** 执行变化判定
- **THEN** `should_publish=true`，默认分支继续执行原有严格六资产发布流程

#### Scenario: 安装器逻辑发生变化

- **GIVEN** 两个 dat 与发布基线相同，但分流片段、仓库地址、嵌入 SHA 或安装器其他逻辑不同
- **WHEN** 执行变化判定
- **THEN** `should_publish=true`，差异不得被发布绑定字段规范化掩盖

#### Scenario: 尚无 latest Release

- **GIVEN** GitHub latest API 明确返回 404
- **WHEN** 默认分支完成有效候选构建
- **THEN** 变化判定报告 `first-release` 并继续首次发布

### Requirement: 安装器只忽略发布绑定字段

比较器 SHALL 要求候选与基线安装器各自恰有一行符合生成器 tag 语法的 `RELEASE_TAG='...'`，并只将该值替换为固定哨兵后比较其余完整字节。

#### Scenario: 安装器仅 Release tag 不同

- **GIVEN** 候选与基线的两个 dat 相同，两个安装器仅唯一合法的 `RELEASE_TAG` 值不同
- **WHEN** 比较规范化主资产
- **THEN** 判定为无有效产物变化

#### Scenario: 安装器发布绑定字段异常

- **GIVEN** 候选或基线安装器缺失发布绑定字段、包含多个字段或字段语法非法
- **WHEN** 比较器规范化安装器
- **THEN** 命令非零退出，不得宽松删除任意相似文本或继续发布

### Requirement: 手动运行可强制发布已验证候选

`workflow_dispatch` SHALL 提供默认值为 false 的布尔输入 `force_publish`；值为 true 时跳过发布基线下载与内容比较，但仍执行候选的完整构建验证，且只有默认分支能够创建 Release。

#### Scenario: 内容相同但人工强制发布

- **GIVEN** 从默认分支手动触发 workflow 且 `force_publish=true`
- **WHEN** 候选发布载荷通过全部验证
- **THEN** 变化判定报告 `forced` 并执行完整 draft、上传、回读、公开和 latest 切换流程

#### Scenario: 非默认分支强制运行

- **GIVEN** 从非默认分支手动触发 workflow 且 `force_publish=true`
- **WHEN** build job 完成
- **THEN** 候选 MAY 作为短期 Artifact 上传，但 publish job SHALL 跳过且不得创建远端 tag 或 Release

### Requirement: 基线错误严格失败

默认变化判定 SHALL 只把 latest API 的明确 404 解释为首次发布；latest 存在但不可信、下载失败、鉴权失败、网络失败或 API 返回其他状态时必须非零失败且不得创建远端发布对象。

#### Scenario: latest 六资产损坏

- **GIVEN** latest Release 存在，但缺少资产、包含额外资产、checksum 不匹配或安装器结构非法
- **WHEN** 非强制 CI 执行变化判定
- **THEN** build job 非零失败，publish job 不运行，并明确指出不可信基线原因

#### Scenario: latest 查询不是 200 或 404

- **GIVEN** GitHub API 因鉴权、限流、网络或服务错误未返回有效 200/404 结果
- **WHEN** CI 查询发布基线
- **THEN** workflow 非零失败，不得把错误误判为首次发布或内容变化

### Requirement: 普通发布复核 latest 身份

非强制发布 SHALL 在创建候选 draft/tag 前确认当前 latest 仍与变化判定时的 Release ID 相同；首次发布则确认 latest 仍不存在，身份不一致时本轮失败。

#### Scenario: 比较后 latest 被外部替换

- **GIVEN** 候选判定需要发布，但变化判定后人工或其他流程改变了 latest
- **WHEN** publish job 准备创建候选 Release
- **THEN** workflow 在远端写入前非零失败，等待下一次基于新基线重新构建比较

## MODIFIED Requirements

### Requirement: Release 资产保持严格六项（修改为只在实际变化时创建）

每次实际创建的 Release SHALL 精确包含 `geosite.dat`、`geoip.dat`、两个对应 `.sha256sum`、`install_passwall2_rules.sh` 与 `install_passwall2_rules.sh.sha256sum`；无有效产物变化时不创建 Release，实际发布时仍须在公开前回读并验证资产集合、目标 commit、tag SHA 与三份 checksum。

#### Scenario: 变化后草稿 Release 六资产回读

- **GIVEN** `should_publish=true` 且默认分支上传了一个有效候选 Artifact
- **WHEN** publish job 创建并回读草稿 Release
- **THEN** 资产名精确等于六项，目标 commit、tag SHA 和三份 checksum 全部通过后才公开并切换 latest

### Requirement: 第一方构建链保持全 Go（增加发布判定命令）

仓库 SHALL 通过根目录 Go module 提供 `geodata-build bootstrap|build|verify|release-decision`，让 CI 与文档不要求 Python、Node、npm 或新增的仓库 Shell 脚本，并只从 `.cache/bin/` 或显式测试替身调用受超时控制的固定上游工具。

#### Scenario: 干净环境完成全链路

- **GIVEN** 环境只提供固定 Go 版本、git、GitHub CLI 与标准系统工具，未安装 Python、Node、npm 或全局 geoip/geoview
- **WHEN** 按文档运行 bootstrap、Go 测试、完整构建与六资产验证
- **THEN** 六个候选资产全部生成并通过 required/forbidden tag、引用、checksum 与资产集合验证

#### Scenario: 干净 runner 完成无变化判定

- **GIVEN** GitHub-hosted Ubuntu 只提供项目现有 Go、git、GitHub CLI 与标准系统工具
- **WHEN** 完成 bootstrap、构建、基线下载和 `release-decision`
- **THEN** 命令能够验证六资产并输出稳定的 `should_publish` 与 `reason`，无需 Python、Node 或 npm

## 方案设计

### 架构与组件

```text
cmd/geodata-build
  └─ internal/app command dispatch
       └─ release-decision
            └─ internal/releasecmp
                 ├─ verify candidate/baseline exact six assets
                 ├─ normalize one RELEASE_TAG assignment
                 └─ compare two dat streams + normalized installer

GitHub Actions build job (contents:read)
  ├─ build and verify candidate
  ├─ resolve/download latest or classify explicit 404
  ├─ invoke release-decision
  ├─ expose should_publish/reason/baseline_release_id
  └─ upload Artifact only when should_publish=true

GitHub Actions publish job (contents:write)
  ├─ require default branch + should_publish=true
  ├─ recheck baseline identity unless forced
  └─ preserve existing draft/readback/publish transaction
```

`internal/releasecmp` 只处理本地、可信边界明确的目录和字节，不负责 GitHub 网络访问。HTTP 状态分类、Release 下载和 job output 编排保留在 workflow；这样比较逻辑可独立单元测试，GitHub 权限边界保持 build 只读、publish 才可写。

### 数据流

```text
rolling upstream + repository config
  → complete candidate build
  → candidate exact-six/checksum validation
  → force?
      yes → Decision{publish:true, reason:forced}
      no  → GitHub latest status
              404 → Decision{publish:true, reason:first-release}
              200 → download baseline → strict validation → canonical compare
  → job outputs
      unchanged → success/no Artifact/no publish
      changed   → Artifact → baseline identity recheck → existing Release transaction
```

上游文件只发生注释、排序或重复项变化，但最终规范化主资产相同时，不产生 Release。固定工具 pin 变化也只在最终主资产变化时触发发布。

### 关键接口

```go
package releasecmp

type Input struct {
    CandidateDir string
    BaselineDir  string
    FirstRelease bool
    Force        bool
}

type Decision struct {
    ShouldPublish bool
    Reason        string // changed, unchanged, first-release, forced
}

func Decide(input Input) (Decision, error)
func NormalizeInstaller(data []byte) ([]byte, error)
```

CLI：

```text
geodata-build release-decision --candidate DIR --baseline DIR
geodata-build release-decision --candidate DIR --first-release
geodata-build release-decision --candidate DIR --force
```

三种模式必须互斥。成功时 stdout 只输出可写入 `$GITHUB_OUTPUT` 的稳定键值：

```text
should_publish=true|false
reason=changed|unchanged|first-release|forced
```

### GitHub Actions 条件

- `workflow_dispatch.inputs.force_publish` 为 boolean，默认 false；schedule 缺失该输入时按 false。
- build job 将 decision step 的 `should_publish`、`reason`、`baseline_release_id` 暴露为 job outputs。
- Artifact upload 显式判断 `steps.release_decision.outputs.should_publish == 'true'`。
- publish job 同时要求 build 成功、`needs.build.outputs.should_publish == 'true'` 与默认分支。
- 普通 changed/first-release 发布复核基线 ID 或“仍无 latest”；forced 发布不依赖旧基线，但不放松候选与新 Release 验证。
- 全 workflow 继续使用 `geodata-release` concurrency group 且不取消正在发布的运行。

### 错误处理

- 候选资产无效在任何模式下都是硬失败。
- baseline 参数模式下，基线资产集合、checksum 或安装器异常都是硬失败。
- 只有 latest API 明确 404 才使用 `--first-release`；其他非 200 状态与网络失败直接失败。
- `--force` 允许人工修复不可信 latest，但不能用于非默认分支发布，也不能跳过候选或发布回读验证。
- 比较器使用流式摘要比较 dat，避免为十余 MiB 文件额外保留完整内存副本。
- 发布阶段失败继续沿用现有 trap，删除未完成的候选 draft 和 tag。

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|---|---|---|---|
| 相同内容重复运行 | unit/integration | 任务内 TDD | 相同 dat、仅 tag 不同 fixture 返回 unchanged；workflow 条件阻止 Artifact/publish |
| 任一 dat 发生变化 | unit | 任务内 TDD | 两侧 dat 变化 table test 均返回 changed |
| 安装器逻辑发生变化 | unit | 任务内 TDD | fragment、SHA、命令或其他字节变化返回 changed |
| 尚无 latest Release | workflow contract | 任务内 TDD | 仅明确 404 映射 first-release |
| 安装器仅 Release tag 不同 | unit | 任务内 TDD | 规范化结果相同 |
| 安装器发布绑定字段异常 | unit | 任务内 TDD | 缺失、重复、非法 tag table test 非零失败 |
| 内容相同但人工强制发布 | unit/workflow contract | 任务内 TDD | force 验证候选并输出 forced/true |
| 非默认分支强制运行 | workflow contract | 任务内 TDD | publish 条件仍要求默认分支 |
| latest 六资产损坏 | unit/integration | 任务内 TDD | 缺失、额外、坏 checksum、坏 installer 均失败 |
| latest 查询不是 200 或 404 | workflow contract | 任务内 TDD | 非法状态与命令错误不能进入 first-release |
| 比较后 latest 被外部替换 | workflow contract | 任务内 TDD | publish 前身份不一致阻断远端写入 |
| 变化后草稿 Release 六资产回读 | release | 验收任务 (D) | GitHub API 资产、target/tag SHA 与下载 checksum；无远端授权时 DEFERRED |
| 干净环境完成全链路 | e2e | 验收任务 (D) | 无 Python/Node 的 clean archive 构建日志与严格六资产 |
| 干净 runner 完成无变化判定 | e2e | 验收任务 (D) | GitHub-hosted Ubuntu 日志；无远端 runner 时 DEFERRED |

workflow contract test 应按 YAML job/step 归属验证 input、output、`if` 与步骤顺序，不能只在全文件搜索孤立字符串。真实 GitHub 的 404、权限、Artifact 条件和 Release 写入仍需 GitHub-hosted 运行验收，本地测试不得冒充。

## 风险与边缘情况

- GitHub latest 可被人工或其他 workflow 改变；普通发布用 Release ID 二次核对降低比较到写入之间的竞态。
- `RELEASE_TAG` 模板格式是规范化协议的一部分；格式变更必须同步比较器测试，异常时严格失败。
- GitHub API 限流、鉴权或短暂网络问题会使当日运行失败而不是多发 Release；下一次 schedule 可重试。
- Release Artifact 下载必须保留全部六项；比较不依赖可执行权限，只依赖主资产字节和 checksum。
- 上游变化但最终语义/字节不变不会发布，这是“有效产物变化”定义的预期结果。
- 非默认分支可以验证候选和变化判定，但永远不能取得 publish 写权限路径。

## 开放问题

无重大开放问题。workflow 内 HTTP 状态读取与 YAML 结构测试的具体辅助函数由实施计划在上述行为边界内确定。
