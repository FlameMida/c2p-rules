# Acceptance Report: Geodata 有效变化增量发布

> Time: 2026-08-03 CST | Triggered by: executing-plans wrap-up | Tier: standard  
> Spec: `.spec-dev/2026-08-02-incremental-geodata-release/spec/incremental-geodata-release-design.md`  
> Verified commit: `06f6d8b1b6177f4541a90349b1655396e0a142ec`

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|---|---|---:|---:|---:|---:|---|
| unit / CLI | D | 10 scenarios | 0 | 0 | 0 | releasecmp、CLI 参数/输出、严格六资产与 tag 上下文绑定 |
| workflow contract | D | 4 scenarios | 0 | 0 | 0 | 404、默认分支、错误传播、发布前 ID/tag/fingerprint 复核 |
| e2e clean archive | D | 0 | 0 | 0 | 2 | 本地 bootstrap/test/integration 通过；完整 build 网络超时；GitHub-hosted 连续运行无授权 |
| release | D | 0 | 0 | 0 | 1 | 静态发布事务契约通过；真实 draft/API 写入无授权 |

17 个 Scenario 的最终状态为 **14 pass / 0 fail / 0 warn / 3 unverified**。

## Requirement Coverage

| Scenario / 检查项 | 维度 | 状态 | 证据 |
|---|---|---|---|
| 相同内容重复运行 | unit/integration | pass | `Test相同内容重复运行`、`TestRenderedInstallerMatchesComparisonProtocol`、`Test相同内容重复运行Workflow` |
| 任一 dat 发生变化 | unit | pass | `Test任一Dat发生变化` |
| 安装器逻辑发生变化 | unit | pass | `Test安装器逻辑发生变化` |
| 尚无 latest Release | workflow contract | pass | `Test尚无LatestRelease` 与 `Test尚无LatestReleaseWorkflow`；build 只在 `case "$latest_status"` 的 404 分支进入首次发布，publish 前再次要求仍为 404 |
| 安装器仅 Release tag 不同 | unit | pass | `Test安装器仅ReleaseTag不同` 与真实 `passwall.RenderInstaller` 跨包测试 |
| 安装器发布绑定字段异常 | unit | pass | `Test安装器发布绑定字段异常`、`TestReleaseTagMustMatchPayloadContext` 覆盖 compare/first/force candidate 与 compare baseline 错绑 |
| 内容相同但人工强制发布 | unit/workflow contract | pass | `Test内容相同但人工强制发布`、`TestFirstAndForcePrintNoneBaselineFingerprint`、workflow force 契约 |
| 非默认分支强制运行 | workflow contract | pass | publish `if` 完整等值断言；`&&` 改为 `||` 的变异会失败 |
| latest 六资产损坏 | unit/integration | pass | `TestLatest六资产损坏` 与三模式候选损坏矩阵 |
| latest 查询不是 200 或 404 | workflow contract | pass | `TestLatest查询不是200或404` |
| latest 查询成功但资产下载失败 | workflow contract | pass | 下载严格位于比较之前，未吞掉 `gh release download` 错误 |
| 发布判定模式缺失或冲突 | unit/CLI | pass | usage 组合矩阵、candidate/candidate-tag/baseline-tag 成对要求、exit 2 与空 stdout |
| 首次或强制模式下候选损坏 | unit/CLI | pass | `Test所有模式下候选损坏`、`Test所有模式下候选损坏CLI`；Compare/FirstRelease/Force 均 exit 1 |
| 比较后 latest 被外部替换 | workflow contract | pass | ID/tag 从同一 API JSON 的 `jq` 读取，fingerprint 从重算决策读取，三者均 source-before-mismatch-block 且位于写入前 |
| 变化后草稿 Release 六资产回读 | release | unverified (DEFERRED) | 精确六资产、target/tag SHA、下载 checksum、Go tag 回读与公开顺序静态契约通过；无可写远端授权 |
| 干净环境完成全链路 | e2e | unverified | clean archive bootstrap、全量测试、真实工具 integration 通过；完整 build 读取 `loyalsoldier-reject` 时网络超时，未生成六资产 |
| 干净 runner 完成无变化判定 | e2e | unverified (DEFERRED) | workflow 结构契约通过；无可触发并读取证据的 GitHub-hosted runner |

## Requirement Reconciliation

实现对账：**8 DELIVERED / 0 DEFERRED / 0 DROPPED / 0 ADDED-IN-FLIGHT**。外部验收证据差量不回退已测试的实现状态：

| Requirement / external evidence | Verdict | Evidence / Reason |
|---|---|---|
| 真实 GitHub draft Release 六资产/API/target/tag/checksum 回读 | DEFERRED | 当前 checkout 无 remote，且未获指定仓库 Release 写入授权；workflow 与静态契约已交付 |
| GitHub-hosted Ubuntu 连续两次无变化运行 | DEFERRED | 无可触发并读取 Actions、Artifact、tag/Release/latest 状态的 runner 权限 |
| clean archive 完整 build 与六资产 | UNVERIFIED | bootstrap、测试、integration 已通过；外部 GitHub raw 读取超时，不能记 pass，也不归因于产品实现 |

## Review Remediation

原三维审查的 9 项确认发现全部修复；受影响维度复审又确认 6 个独立测试证明力/追踪缺口，也已全部修复。后续对 first-release 分支归属、来源先于比较、同一次 `$readback` 调用的三项结构性复核均已枯竭，无残留 finding。

关键修复包括：候选/基线安装器 tag 与发布上下文精确绑定；真实 renderer 跨包协议；裸拼接碰撞 fixture；三模式候选错误矩阵；CLI 稳定输出；404 唯一分支；默认分支完整布尔门禁；ID/tag/fingerprint 来源与失败块；精确六资产上传、回读及公开前严格 Go 校验。

## Evidence Index

- 本地确定性证据：`acceptance/local-evidence.md`
- 审查报告：`acceptance/reviews/round1-a.json`、`round1-b.json`、`round1-c.json`、`round2-a.json`、`round2-b.json`、`round2-c.json`、`completeness.json`
- 主要测试：`internal/releasecmp/releasecmp_test.go`、`internal/app/commands_test.go`、`internal/app/workflow_contract_test.go`、`internal/cli/run_test.go`
- 验证命令：`go test -count=1 ./...`、`go vet ./...`、关键包 race test、clean archive bootstrap/test/integration

## coverage_note

- visual、a11y、perf-web：纯 Go CLI / 数据构建与 GitHub workflow，无页面，按矩阵裁剪。
- perf-api：无常驻 API 服务，spec 无吞吐或延迟预算，按矩阵裁剪。
- 真实 GitHub Release 与 GitHub-hosted runner 均显式 DEFERRED；未用静态 workflow 冒充远端通过。
- clean archive 完整 build 因外部网络超时保持 unverified；未把部分通过升级为全链路通过。
- `actionlint` 在本机不可用；workflow 由 YAML 解析测试、精确 job/step 契约和 Go 全量测试覆盖，未声称获得 actionlint 证据。

