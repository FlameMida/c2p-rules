# Acceptance Report: Go geodata pipeline 与 PassWall2 托管分流

> Time: 2026-07-31 01:42 CST | Triggered by: executing-plans wrap-up | Tier: standard
> Spec: `.spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md` | Evidence dir: `.spec-dev/2026-07-31-go-geodata-pipeline/acceptance/`

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|-----------|-----------|-----:|-----:|-----:|-----------:|-------|
| unit | D | 13 packages | 0 | 0 | 0 | 干净 Git 归档中 `go test -count=1 ./...` 通过，3.9s |
| integration | D | 2 | 0 | 0 | 0 | 固定真实工具 synthetic integration 与 18 个真实远程 source 正式 build 均通过 |
| e2e | D | 5 | 0 | 0 | 0 | fake-UCI 幂等、旧分流清理、错误 hash/三阶段 commit 回滚、真实 16 组成功安装与备份 |
| release | D | 1 static | 0 | 0 | 1 | 六资产 workflow 契约通过；真实 GitHub draft/API 回读未执行 |
| operational | D | 0 | 0 | 0 | 1 | 无获授权 OpenWrt/PassWall2 可丢弃设备 |

## Requirement Coverage

| Matrix row (Scenario / check item) | Dimension | Status | Evidence |
|------------------------------------|-----------|--------|----------|
| 重复安装保持用户配置与托管组幂等 | e2e | pass | `TestInstallerIsIdempotentAndPreservesUserRulesAndNodes` 两次执行后配置逐字节一致，并断言用户 node/rule 的完整 section 片段保留，托管组仅一份 |
| 首次安装清理旧转换器分流 | e2e | pass | 同一测试断言 `c2p_Proxy` 与旧 `crs_old` 消失，非托管用户 section 保留 |
| 更新器成功退出但 dat 哈希错误 | e2e | pass | `TestInstallerRollsBackWhenUpdaterReturnsSuccessWithWrongHash` 断言配置与两个 dat 恢复原字节；单侧旧 dat 缺失路径也通过 |
| 完整成功后保留可恢复备份 | e2e | pass | `TestInstallerSuccessInstallsAllRepositoryGroupsAndValidData` 使用仓库真实 16 组，断言双 latest URL、两个 dat 字节、16 个 section、原 node、备份原内容和 0600 权限；三次 UCI commit 故障均回滚 |
| 草稿 Release 六资产回读 | release | unverified | 静态契约已强制 upload → API 名称/target/tag SHA → draft 重新下载 → 精确六项 → 三次 checksum → publish 顺序；无已授权远端，未产生真实 GitHub API JSON/job log |
| 干净环境完成全链路 | e2e | pass (local) | Git `4110c738e1c99dc20230afe67ef6dbb4a066950b` 归档到全新临时目录；fresh bootstrap、测试、vet、integration、正式 build、四次 tag probe、三份 checksum 全部 exit 0 |
| PassWall2 真实设备更新 | operational | unverified | 无可丢弃设备，按计划边界记录为 DEFERRED |

## Requirement Reconciliation

实现 Requirement 均已 DELIVERED；以下仅为需要外部系统或设备的验收差量：

| Requirement | Verdict | Evidence / Reason |
|-------------|---------|-------------------|
| GitHub draft Release 六资产 API 回读 | DEFERRED | workflow 已实现并有静态契约守卫，但当前任务未获授权创建远端 Release，不能伪造 API/job log |
| Ubuntu Actions runner 完整日志 | DEFERRED | 本地干净归档全链路通过；尚无已配置远端触发 GitHub Actions |
| PassWall2 真实设备更新与双内核命中 | DEFERRED | 无获授权测试设备；fake-UCI 不能替代 operational 证据 |

## Key Findings

无 P0/P1/P2 验收失败。三项 DEFERRED 均属于预先声明的外部交付边界，不是本地实现失败。

独立证据审计最初指出三项本地覆盖缺口，已在报告提交前补齐：真实 16 组成功安装与双 URL/dat/备份断言；第三次切换 latest 的 UCI commit 故障注入；draft Release 重新下载后精确集合与三次 checksum 的 workflow 契约。补强测试与全仓回归均通过。审计保留的合理限制是：fake-UCI 不能替代真实 libuci，静态 workflow 不能替代真实 GitHub API，因此相应外部项仍为 DEFERRED。

真实工具验收期间发现并已在 T13 修复两项 pinned 工具行为：缺失 GeoIP tag 的 `geoview` 非零退出判别，以及 `domain-list-custom --togfwlist=` 仍生成空 `gfwlist.txt`。修复后的 integration 和正式全量构建均通过。

## Diagnosis Details

全部已执行项目通过，诊断阶段跳过。

## Evidence Index

- Focused e2e：初始 `go test -count=1 ./internal/passwall -run 'TestInstaller(...)' -v` exit 0；审计补强后 `TestInstallerSuccessInstallsAllRepositoryGroupsAndValidData` 与 commit-1/2/3 故障矩阵均 exit 0。
- Repository contracts：`go test -count=1 ./internal/app -run 'Test(Workflow...|Repository...|Docs...)' -v`，exit 0。
- Clean archive root：`/var/folders/r5/ww8n2hdd7pz9s1jb52q6mqbm0000gn/T/crs-clean-acceptance.XXXXXX.vVHADD6o9V`。
- Clean unit regression：`go test -count=1 ./...`，exit 0；`go vet ./...`，exit 0。
- Clean real-tool integration：`go test -count=1 -tags=integration ./internal/app`，exit 0，1.444s。
- Clean production build：`go run ./cmd/geodata-build build --repo example/clash-rules-srs --release-tag acceptance-20260731`，18 个真实 source 与真实 base dat，exit 0。
- Final tag probes：GeoSite/GeoIP required 与 forbidden 四条 `geodata-build verify` 命令均 exit 0。
- Final checksums：`geoip.dat: OK`、`geosite.dat: OK`、`install_passwall2_rules.sh: OK`。
- Final exact assets：六项，大小分别为 17,483,046 / 76 / 9,117,274 / 78 / 7,620 / 93 bytes，总计 26,608,187 bytes。
- Test sources：`internal/passwall/installer_test.go`、`internal/app/build_integration_test.go`、`internal/app/workflow_contract_test.go`、`internal/app/runtime_contract_test.go`。
- 独立 pass 审计：`acceptance_audit` 只读复跑 focused tests、integration、全仓测试和 vet；指出的本地缺口均已转成确定性断言，真实 libuci/GitHub 边界保留为 DEFERRED。
- Tier A contract JSON：不适用；本项目无 UI/浏览器/设备会话，本轮仅执行 Tier D。

## coverage_note

- visual、a11y、perf-web：纯 Go CLI/数据构建项目，无页面，按矩阵取舍裁剪。
- perf-api：无长驻 API 服务且 spec 无吞吐/延迟预算，裁剪；未安装 k6 不影响本矩阵。
- AI 浏览器验收：无浏览器目标，裁剪。
- release 与 operational 的未验证项已逐项列为 DEFERRED；未以本地静态检查或 fake-UCI 冒充真实 GitHub/设备通过。
- fake-UCI 能证明脚本状态机与文件回滚，但不能证明真实 libuci 实现；真实设备行因此未升级为 pass。
