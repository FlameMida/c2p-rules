# Acceptance Report: shadowrocket-conf

> Time: 2026-08-15 | Triggered by: executing-plans wrap-up | Tier: standard
> Spec: `.spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md` | Evidence dir: `.spec-dev/2026-08-15-shadowrocket-conf/acceptance/`
> Spec status: active | Reconciliation status: final

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|-----------|-----------|------|------|------|------------|-------|
| unit | D | 140 | 0 | 0 | 0 | 受影响四个 package，无缓存复跑 |
| integration | D | 65 | 0 | 0 | 0 | 真实固定工具，无缓存复跑 |
| repository regression | D | 314 | 0 | 0 | 0 | `go test -count=1 ./...` |
| e2e (Shadowrocket device) | D | 0 | 0 | 0 | 1 | 无已发布八资产 Release 与授权真机，DEFERRED |

## Requirement Coverage

| Matrix row (Scenario / check item) | Dimension | Status | Evidence |
|------------------------------------|-----------|--------|----------|
| 完整构建得到八文件且 checksum 匹配 | integration | pass | `Test完整构建得到八文件且Checksum匹配` |
| 缺少 conf 时资产校验失败 | unit | pass | `Test缺少Conf时资产校验失败` |
| skip-compile 不产出 conf | integration | pass | `TestSkipCompile不产出Conf` |
| include 被展开进 google 组 | unit/integration | pass | `TestInclude被展开进Google组` 与完整构建测试 |
| merge-base GeoIP 含底包 | unit/integration | pass | `TestMergeBaseGeoIP含底包` 与完整构建双栈断言 |
| 展开失败则不发布 | unit | pass | `Test展开失败则不发布` |
| 四种可映射类型与 CIDR | unit | pass | `Test四种可映射类型与CIDR` golden |
| regexp 被跳过 | unit | pass | `TestRegexp被跳过` |
| 组名与顺序对齐 YAML | unit/integration | pass | 逐组精确规则行存在性 + marker 存在后顺序比较 |
| 广告与国内默认 | unit | pass | `Test广告与国内默认` |
| 空组失败 | unit | pass | `Test空组失败` |
| 重复或保留策略组名失败 | unit | pass | `Test策略组名称拒绝重复与保留名` |
| latest update-url | unit | pass | `TestLatestUpdateURL` |
| 兜底顺序 | unit | pass | `Test兜底顺序` |
| 只改 conf 判定 changed | unit | pass | `Test只改Conf判定Changed` |
| 只换 tag 不改 conf 判定 unchanged | unit | pass | `Test只换Tag不改Conf判定Unchanged` |
| 六文件基线不得当成 changed | unit/CLI | pass | releasecmp 与 CLI 两层测试 |
| 文档含新资产与导入入口 | unit | pass | `Test文档含新资产与导入入口` |
| workflow 上传八文件 | unit | pass | `TestWorkflow上传与校验八资产` 与草稿回读契约 |
| 真机导入与自动更新 | e2e | unverified | `shadowrocket-device.md`，DEFERRED |

## Requirement Reconciliation

7 个现行 Requirement 全部 DELIVERED。真机验收是 spec 明确的 DEFERRED 验收边界，不改变 Requirement 实现状态。

| Requirement / behavior | Verdict | Evidence / Reason |
|------------------------|---------|-------------------|
| 重复或保留策略组名失败 | ADDED-IN-FLIGHT | 用户批准收尾审查修复；active spec 已新增 Scenario，Render 与单测已同步 |

## Key Findings

审查确认的 3 个中严重性问题与 2 个直接相关测试缺口均已修复，受影响维度复审两轮后零发现：

1. Render 现在拒绝重复 remarks 和 `PROXY` / `DIRECT` / `REJECT` 大小写变体，active spec 已补契约。
2. protobuf 错误 wire type、截断、非法 enum/IP/prefix、重复 tag、缺 geoip tag与未知字段消费已有回归测试。
3. 计划已如实记录八资产清单从 T4 前移 T3，以及 T4 实际只有 releasecmp 指纹测试出现红灯。
4. 16 组规则存在性改为逐组精确行断言，顺序 helper 先验证 marker 存在；实际构建另断言 conf 不含 `ReleaseTag`。

经用户批准暂缓的低优先级建议：抽取中立 repo 校验、合并重复文档断言、生产规模大 dat 内存/取消性能优化；均不影响现行 Requirement 交付。

## Diagnosis Details

确定性测试没有失败，Tier X 诊断跳过。审查发现来自独立静态审查与对抗复核，不等同于本次测试失败。

## Evidence Index

- 受影响测试：`go test -count=1 ./internal/shadowrocket ./internal/app ./internal/verify ./internal/releasecmp`，140 pass / 0 fail。
- integration：`go test -tags=integration ./internal/app`，65 pass / 0 fail。
- 全仓回归：`go test -count=1 ./...`，314 pass / 0 fail。
- 真机记录：`.spec-dev/2026-08-15-shadowrocket-conf/acceptance/shadowrocket-device.md`。
- 审查原始报告：本次执行临时目录中的 A/B/C 与 completeness 报告；合并前按最终处置汇总到本报告。

## coverage_note

已执行所有与代码变更相交的 unit / integration 矩阵行和无缓存全仓回归。visual、a11y、perf-web、perf-api 与本次 Go CLI / 配置资产形态不适用而裁剪；未配置覆盖率门槛，未临时增加阈值。Shadowrocket 真机 e2e、自动更新和大 conf 真机编译耗时因没有已发布八资产 Release 与授权设备而标记 unverified / DEFERRED。
