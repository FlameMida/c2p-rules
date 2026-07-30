# Acceptance Report: Go geodata pipeline 与 PassWall2 托管分流

> Time: 2026-07-31 CST | Triggered by: executing-plans wrap-up and review remediation | Tier: standard
> Spec: `.spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md` | Evidence dir: `.spec-dev/2026-07-31-go-geodata-pipeline/acceptance/`

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|-----------|-----------|-----:|-----:|-----:|-----------:|-------|
| unit | D | 16 packages | 0 | 0 | 0 | clean Git archive 中 `go test -count=1 ./...` 通过 |
| integration | D | 2 suites | 0 | 0 | 0 | 固定真实工具 synthetic build 与 5 条 ProductionDependencies 失败回滚链路通过 |
| e2e | D | 8 scenarios | 0 | 0 | 1 | fake-UCI 覆盖幂等、三阶段 commit、savedir、live delta、hash 与恢复失败；Ubuntu Actions 日志未验证 |
| release | D | 1 static | 0 | 0 | 1 | 六资产 workflow 契约通过；真实 GitHub draft/API 回读未执行 |
| operational | D | 0 | 0 | 0 | 1 | 无获授权 OpenWrt/PassWall2 可丢弃设备 |

## Requirement Coverage

| Matrix row (Scenario / check item) | Dimension | Status | Evidence |
|------------------------------------|-----------|--------|----------|
| 重复安装保持用户配置与托管组幂等 | e2e | pass | `TestInstallerIsIdempotentAndPreservesUserRulesAndNodes` 两次执行后配置逐字节一致；用户 node/rule 保留，托管组仅一份 |
| 首次安装清理旧转换器分流 | e2e | pass | 同一测试断言 `c2p_Proxy` 与旧 `crs_old` 消失，非托管用户 section 保留 |
| 更新器成功退出但 dat 哈希错误 | e2e | pass | `TestInstallerRollsBackWhenUpdaterReturnsSuccessWithWrongHash` 断言配置与两个 dat 恢复原字节 |
| 完整成功后保留可恢复备份 | e2e | pass | 真实 16 组 fixture 断言双 latest URL、两个 dat、原 node、0600 配置备份；恢复失败测试断言配置/site/ip 人工恢复材料保留 |
| staging UCI 与 live delta 隔离 | e2e | pass | fake-UCI 强制 staging 提供独立 `-P` savedir，并要求恢复配置前先 `uci revert passwall2` |
| 并发 build/publish 成套切换 | integration/race | pass | barrier 控制两个独立 Transaction 交错，最终 build/publish 同代；`go test -race` 通过，Linux 交叉编译通过 |
| 草稿 Release 六资产回读 | release | unverified | 静态契约强制 upload → API 名称/target/tag SHA → draft 重新下载 → 精确六项 → 三次 checksum → publish；无已授权远端 |
| 干净环境完成全链路 | e2e | unverified (CI) | clean archive、空 cache、隔离 PATH 中 Python/Python3/Node/npm 不可见，fresh bootstrap 到六资产全链路 exit 0；但没有 GitHub-hosted Ubuntu Actions 日志，故不升级 CI 行 |
| PassWall2 真实设备更新 | operational | unverified | 无可丢弃设备；fake-UCI 不能替代真实 libuci、Xray/sing-box 命中或物理中断恢复 |

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

收尾审查确认的 7 项实现/契约问题已全部修复：manifest 不再从任意 Source ID 推导 forbidden；bootstrap 强制 reset/clean；custom YAML 严格校验；公开 CLI 使用 `--work-root`；PassWall2 staging savedir 与 live delta 隔离；恢复失败保留人工恢复材料；workspace 使用 root 级跨进程锁保证双目录同代切换。

用户选择的完整清理也已完成：ProductionDependencies 增加 404/custom/collision/group/final-probe 五条真实失败链路；组引用先以组名感知错误验证并缓存 tag probe；五份 atomic write 合为 `fileutil.AtomicWrite`；DomainKind 强类型化；GeoIP 模板只解析一次；严格 YAML 统一到 `yamlutil`；README/context 的事务顺序与脚本一致。

两个对抗复核结论保留：pinned geoview 对 tag 查询本身大小写不敏感，原 mixed-case fallback 候选不构成可触发缺陷；其余已确认项没有跳过。

## Evidence Index

- 持久化确定性证据：`acceptance/local-evidence.md`
- Source commit: `58b3586032dc4376194af8ec886b73dea8178529`
- Clean runtime: Git archive + empty cache + isolated PATH；Python/Python3/Node/npm 均不可见
- Unit/vet/integration/build/probe/checksum：全部 exit 0，命令与关键输出见 evidence 文件
- Exact assets: 六项，大小、mode 与三份 SHA-256 见 evidence 文件
- Review-fix tests: `internal/manifest/manifest_test.go`、`internal/tools/bootstrap_test.go`、`internal/rules/custom_test.go`、`internal/app/build_integration_test.go`、`internal/passwall/installer_test.go`、`internal/workspace/transaction_test.go`
- External boundaries: GitHub draft/API、Ubuntu Actions、OpenWrt/libuci/Xray/sing-box 均保持 DEFERRED

## Diagnosis Details

最终执行项目全部通过；诊断阶段无剩余失败。

## coverage_note

- visual、a11y、perf-web：纯 Go CLI/数据构建项目，无页面，按矩阵裁剪。
- perf-api：无常驻 API 服务且 spec 无吞吐/延迟预算，裁剪。
- release 与 operational 的未验证项已逐项列为 DEFERRED；未用静态 workflow 或 fake-UCI 冒充真实 GitHub/设备通过。
- clean archive 证明第一方链路无需 Python/Node，但不是 GitHub-hosted Ubuntu runner，因此 CI 日志仍单独 DEFERRED。
- 本地证据已持久化为命令、退出状态、关键输出、资产大小/mode/hash；临时目录仅用于执行，不作为唯一证据载体。
