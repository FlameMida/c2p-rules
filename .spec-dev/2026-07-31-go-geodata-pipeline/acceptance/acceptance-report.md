# Acceptance Report: Go geodata pipeline 与 PassWall2 托管分流

> Time: 2026-07-31 CST | Triggered by: executing-plans wrap-up and review remediation | Tier: standard
> Spec: `.spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md` | Evidence dir: `.spec-dev/2026-07-31-go-geodata-pipeline/acceptance/`

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|-----------|-----------|-----:|-----:|-----:|-----------:|-------|
| unit | D | 3 pure + 4 mixed scenarios | 0 | 0 | 0 | 24 行 active Scenario 中的 unit 与 unit/integration 场景均有自动化测试 |
| integration | D | 11 pure + 4 mixed scenarios | 0 | 0 | 0 | 固定真实工具 synthetic build、dat probe 与失败回滚链路通过 |
| e2e | D | 4 scenarios | 0 | 0 | 1 | fake-UCI 覆盖幂等、迁移、hash 回滚和成功备份；GitHub-hosted Ubuntu 日志未验证 |
| release | D | 0 | 0 | 0 | 1 | 六资产 workflow 静态契约通过；真实 GitHub draft/API 回读未执行 |
| operational | D | 0 | 0 | 0 | 1 | 无获授权 OpenWrt/PassWall2 可丢弃设备 |

## Requirement Coverage

| Matrix row (Scenario / check item) | Dimension | Status | Evidence |
|------------------------------------|-----------|--------|----------|
| 扩展远程 source 创建的 BilibiliHMT | integration | pass | `TestCustomCanExtendCreatedBilibiliHMT` 将自定义 GeoSite/IP 写入已声明目标；`TestSyntheticFullBuildMergesAndSeparatesApprovedTags` 用真实固定工具正探针 `BilibiliHMT` 双侧，且普通 `bilibili` 负探针 |
| 拒绝拼错的本地目标 | unit/integration | pass | `TestCustomRejectsUnknownTargetBeforeEmission` 与 `TestBuildUnknownCustomTargetDoesNotSwitchPublish` 断言 `geosite:googel` 报错且旧 build/publish 字节不变；ProductionDependencies strict-custom 故障链复验同一发布边界 |
| 未编辑模板不改变产物 | unit | pass | `TestDefaultTemplatesAreSemanticNoOps` 与 `TestCustomEmptyTemplateIsSemanticNoOp` 断言默认模板解析后无 Domain/CIDR contribution |
| 苹果服务包含声明的 tag | unit | pass | `TestRenderAppleAndChinaWithStableOrder` 的 UCI golden 断言苹果服务包含 `geosite:apple`、`geosite:icloud` |
| 缺失 tag 阻断脚本发布 | integration | pass | `TestBuildMissingGroupTagDoesNotPublishInstaller` 断言错误包含组名与 `geoip:not-exist`，旧 build/publish 保持；ProductionDependencies group-ref 故障链复验真实 probe 路径 |
| YouTube 优先于 Google | unit/integration | pass | `TestYouTubePrecedesGoogle`、`TestRepositoryDefaultGroupsMatchApprovedOrder` 与 synthetic installer fragment 均断言 YouTube 位于 Google 服务之前 |
| 中国大陆具备域名与 IP 规则 | integration | pass | `TestRenderAppleAndChinaWithStableOrder` 断言 `geosite:cn`、`geoip:cn`；`TestInstallerSuccessInstallsAllRepositoryGroupsAndValidData` 使用仓库 16 组完整 fixture 安装 |
| 更新器成功退出但 dat 哈希错误 | e2e | pass | `TestInstallerRollsBackWhenUpdaterReturnsSuccessWithWrongHash` 断言配置、geosite.dat、geoip.dat 恢复原字节 |
| 完整成功后保留可恢复备份 | e2e | pass | `TestInstallerSuccessInstallsAllRepositoryGroupsAndValidData` 断言配置备份存在、权限 0600、包含原 node，且双 dat 为校验后的新字节 |
| 重复安装保持用户配置与托管组幂等 | e2e | pass | `TestInstallerIsIdempotentAndPreservesUserRulesAndNodes` 两次执行后配置逐字节一致；用户 node/rule 保留，托管组仅一份 |
| 首次安装清理旧转换器分流 | e2e | pass | 同一测试断言 `c2p_Proxy` 与旧 `crs_old` 消失，非托管用户 section 保留 |
| Google 合并标准 tag | integration | pass | `TestRepositorySourcesMatchApprovedTargets` 固定 `geosite:google:merge-base`；synthetic build 对 `google` 新 tag 正探针、`loyalsoldier-google` 旧 tag 负探针 |
| create 与 merge-base 前置条件严格执行 | unit | pass | `TestCreateAndMergeBasePreconditions` table test 覆盖 create 目标已存在与 merge-base 目标不存在两类非法 registry |
| 对全部 outputs 逐侧探针 | integration | pass | `TestBuildEmitsAndProbesEveryOutput` 对 manifest 的每个 output 逐侧记录 probe；synthetic build 再以真实 geoview 校验 required/forbidden 集合 |
| Netflix 合入标准双侧 tag | integration | pass | `TestRepositorySourcesMatchApprovedTargets` 固定双侧 `netflix:merge-base`；synthetic build 对 GeoSite/GeoIP `netflix` 正探针并确认旧 source tag 不进入最终契约 |
| 未声明的 community 碰撞仍失败 | unit/integration | pass | `TestBuildCreateCollisionDoesNotSwitchPublish` 与 ProductionDependencies create-collision 故障链断言 registry 报错且旧 build/publish 字节不变 |
| 上游源失败不发布 | integration | pass | `TestBuildSourceFailureDoesNotSwitchPublish` 与 ProductionDependencies HTTP 404 故障链断言目录不切换 |
| Geosite 精确去重不扩大语义 | unit/integration | pass | `TestMergeDeduplicatesExactRulesButPreservesKindAndAttrs` 只删除完全相同项，保留同值不同 kind/attrs 与 community directive；synthetic dat probe 通过 |
| GeoIP 重叠 CIDR 规范化 | integration | pass | `TestCIDRsAreMaskedSortedAndDeduplicated` 保留 `/18` 与显式 `/19`、仅精确去重；synthetic dat 对重叠地址正探针 |
| Google 父域不被破坏 | integration | pass | `TestMergeKeepsGoogleYouTubeAndBilibiliTargetsIndependent` 保留 Google 父域与 YouTube 子域在各自 tag；synthetic 双 tag probe 与 UCI order golden 通过 |
| 港澳台规则不污染普通 bilibili | integration | pass | 同一 merge test 与 synthetic build 对 `BilibiliHMT` 双侧正探针、普通 `bilibili` 负探针 |
| 草稿 Release 六资产回读 | release | unverified | 静态契约强制 upload → API 名称/target/tag SHA → draft 重新下载 → 精确六项 → 三次 checksum → publish；无已授权远端 |
| 干净环境完成全链路 | e2e | unverified (CI) | clean archive、空 cache、隔离 PATH 中 Python/Python3/Node/npm 不可见，fresh bootstrap 到六资产全链路 exit 0；但没有 GitHub-hosted Ubuntu Actions 日志，故不升级 CI 行 |
| 最后一个探针失败不破坏旧发布物 | integration | pass | `TestBuildForbiddenProbeFailurePreservesOldOutputs` 与 ProductionDependencies final-forbidden-probe 故障链在最后门禁注入失败，断言旧 build/publish 字节不变 |
| PassWall2 真实设备更新（operational check） | operational | unverified | 无可丢弃设备；fake-UCI 不能替代真实 libuci、Xray/sing-box 命中或物理中断恢复 |

补充的审查修复检查（不替代上述 24 行 active Scenario）：

| Remediation check | Status | Evidence |
|-------------------|--------|----------|
| staging UCI 与 live delta 隔离 | pass | `TestInstallerUsesIsolatedSavedirForStagingUCI` 的 faithful fake-UCI 建模 `-P` NOCOMMIT，确认模板改用可提交的 `uci -t`；`TestInstallerClearsLiveUCIDeltaBeforeRestoringConfig` 检查恢复顺序 |
| custom YAML 仅允许 `payload` 本身为 null | pass | `TestDecodeStrictAllowsOnlyExplicitNullPolicy` 及 custom parser table tests 拒绝根 null、null item、mixed null item，只接受 `payload: null` |
| 并发 build/publish 成套切换 | pass | `TestConcurrentTransactionsNeverPublishMixedGenerations` 用 barrier 控制两个 Transaction 交错；workspace race test 通过 |
| 已发布后 lock release 失败不误报 | pass | `TestCommitIgnoresLockReleaseFailureAfterPublishing` 注入 release 错误，断言 Commit 成功且 build/publish 新代际均已生效 |

## Requirement Reconciliation

实现对账：16 个 active Requirements 均为 DELIVERED；2 个 REMOVED Requirements 按批准设计为 DROPPED；0 个 ADDED-IN-FLIGHT。以下三项仅为外部验收证据差量，不回退本地实现状态：

| Requirement / external evidence | Verdict | Evidence / Reason |
|---------------------------------|---------|-------------------|
| GitHub draft Release 六资产 API 回读 | DEFERRED | workflow 与静态契约已实现，但当前任务未获授权创建远端 Release |
| GitHub-hosted Ubuntu Actions 完整日志 | DEFERRED | 本地无 Python/Node 的 clean archive 全链路已通过；未配置或触发远端 runner |
| PassWall2 真实设备与双内核命中 | DEFERRED | 无获授权测试设备；真实 libuci、Xray/sing-box 与断电恢复不可由 fake-UCI 冒充 |
| clash2passwall dat 模式固定映射 | DROPPED | 属 REMOVED Requirement；Node/转换器路径已删除并有 runtime contract 守卫 |
| Python 与 Node 构建运行时 | DROPPED | 属 REMOVED Requirement；第一方构建、CI 和文档已统一为 Go |

## Key Findings

首轮收尾审查确认的 7 项实现/契约问题已全部修复：manifest 不再从任意 Source ID 推导 forbidden；bootstrap 强制 reset/clean；custom YAML 严格校验；公开 CLI 使用 `--work-root`；PassWall2 staging savedir 与 live delta 隔离；恢复失败保留人工恢复材料；workspace 使用 root 级跨进程锁保证双目录同代切换。

第二轮复审确认的 4 项也已修复：staging UCI 从会设置 NOCOMMIT 的 `-P` 改为真正隔离且可提交的 `-t`；null 策略收窄为仅允许 `payload` 字段本身为空；本报告逐条对账全部 24 个 active Scenario；workspace 已完成切换后忽略 lock release 错误，避免非零退出诱发重复发布。

用户选择的完整清理也已完成：ProductionDependencies 增加 404/custom/collision/group/final-probe 五条真实失败链路；组引用先以组名感知错误验证并缓存 tag probe；五份 atomic write 合为 `fileutil.AtomicWrite`；DomainKind 强类型化；GeoIP 模板只解析一次；严格 YAML 统一到 `yamlutil`；README/context 的事务顺序与脚本一致。

两个对抗复核结论保留：pinned geoview 对 tag 查询本身大小写不敏感，原 mixed-case fallback 候选不构成可触发缺陷；其余已确认项没有跳过。

## Evidence Index

- 持久化确定性证据：`acceptance/local-evidence.md`
- Verified code commit: `da5a5680c19eebfe3e910b9fb969030c2a122d3e`
- Clean runtime: Git archive + empty cache + isolated PATH；Python/Python3/Node/npm 均不可见
- Unit/vet/integration/build/probe/checksum：全部 exit 0，命令与关键输出见 evidence 文件
- Exact assets: 六项，大小、mode 与三份 SHA-256 见 evidence 文件
- Review-fix tests: `internal/manifest/manifest_test.go`、`internal/tools/bootstrap_test.go`、`internal/rules/custom_test.go`、`internal/yamlutil/strict_test.go`、`internal/app/build_integration_test.go`、`internal/passwall/installer_test.go`、`internal/workspace/transaction_test.go`、`internal/workspace/transaction_internal_test.go`
- External boundaries: GitHub draft/API、Ubuntu Actions、OpenWrt/libuci/Xray/sing-box 均保持 DEFERRED

## Diagnosis Details

最终执行项目全部通过；诊断阶段无剩余失败。

## coverage_note

- visual、a11y、perf-web：纯 Go CLI/数据构建项目，无页面，按矩阵裁剪。
- perf-api：无常驻 API 服务且 spec 无吞吐/延迟预算，裁剪。
- release 与 operational 的未验证项已逐项列为 DEFERRED；未用静态 workflow 或 fake-UCI 冒充真实 GitHub/设备通过。
- clean archive 证明第一方链路无需 Python/Node，但不是 GitHub-hosted Ubuntu runner，因此 CI 日志仍单独 DEFERRED。
- 本地证据已持久化为命令、退出状态、关键输出、资产大小/mode/hash；临时目录仅用于执行，不作为唯一证据载体。
