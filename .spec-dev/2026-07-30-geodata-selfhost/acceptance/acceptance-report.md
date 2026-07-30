# Acceptance Report: geodata-selfhost

> Time: 2026-07-30 | Triggered by: executing-plans wrap-up | Tier: deep
> Spec: `.spec-dev/2026-07-30-geodata-selfhost/spec/geodata-selfhost-design.md`
> Verified implementation HEAD: `64d7d6a`
> Status: local delivery accepted; external publication and real-device consumption deferred

## Overview

| Dimension | Execution | Pass | Fail | Warn | Unverified | Notes |
|-----------|-----------|------|------|------|------------|-------|
| unit / integration | D | 50 | 0 | 0 | 0 | Python `unittest` 50/50 |
| converter / installer CLI | D | 7 | 0 | 0 | 0 | Node 7/7；包含 builtin 与 js-yaml、fake-UCI 成功/回滚/冲突回退 |
| supply chain / static | D | 6 | 0 | 0 | 0 | `npm ci`、audit、Python/Node/shell syntax、`git diff --check` |
| full geodata build | D | 6 | 0 | 0 | 0 | 在线拉源、双 dat、正负探针、manifest 交叉检查、双 checksum |
| external Release / device | — | 0 | 0 | 0 | 2 | 未获授权建立远端/发布；无可丢弃 OpenWrt/双核设备 |
| visual / a11y / perf | — | — | — | — | — | 纯 CLI、构建与固件配置特性，不适用 |

## Requirement Coverage

| Scenario / check item | Dimension | Status | Evidence |
|-----------------------|-----------|--------|----------|
| sha256 伴随文件可被标准工具校验 | integration | pass | `sha256sum -c` 两项均 OK；文件名为纯 basename、双空格格式 |
| Releases latest URL 与 sha 派生 | external e2e | deferred | workflow 与 URL 形态已实现；公开仓库、remote、push、首次 Release 未获授权 |
| Release 资产列表无 srs | integration | pass / deferred | workflow 仅允许四项并在 draft 中精确回读；真实 Release 资产回读随首次发布延期 |
| 标准 cn 仍可用 | integration | pass | geosite、geoip 的 `cn` 均由 geoview 转为非空输出 |
| 对 sources 逐项正向探针 | integration | pass | geosite：`cn` + 15 个自定义 tag；geoip：`cn`、`private` + 5 个自定义 tag |
| forbidden 负向探针 | integration | pass | applications、IP-only/domain-only 交叉侧均确认不存在 |
| 域名源写入 geosite | unit / integration | pass | emit 单测 + 最终 geosite 探针 |
| 跳过 applications / process-only | unit / integration | pass | 完整 `PROCESS-*` 家族；仅显式 `sides: []` 可跳过，非空声明会 fail-fast |
| Netflix 同时有域名与 IP | integration | pass | 最终两侧 `xiaolin-netflix` 均非空 |
| YouTube 仅域名 | integration | pass | geosite 非空，geoip forbidden probe 证实不存在 |
| domain-behavior 后缀与精确 | unit | pass | `test_buckets_dlc` |
| classical 关键词 | unit | pass | `test_buckets_dlc` |
| 上游 404 / 缺失源 fail-fast | integration | pass | CLI 子进程非零、编译器 sentinel 未触发、没有完整发布集 |
| 自定义与 community 撞名 | integration | pass | CLI 在编译前非零，编译器 sentinel 未触发 |
| gfw / proxy / reject / AI / telegramcidr 映射 | CLI e2e | pass | manifest 驱动映射与交叉引用校验通过 |
| Netflix 双字段 | CLI e2e | pass | 仅在 manifest 声明相应侧时输出，两侧引用均存在 |
| 写入 URL | installer | pass / deferred | fake-UCI 验证明确 repo URL；真实设备执行随设备验收延期 |
| 覆盖导入且保留节点 | installer | pass / deferred | fake-UCI 保留节点、删除旧 shunt、处理全局 ID 冲突；真实 libuci 延期 |
| UCI 分组名可读且稳定 | installer | pass | section 为稳定具名 `c2p_*`，原 Clash 分组保存在 `remarks`；不再生成匿名 `cfg...` |
| geoview 提示 | CLI e2e | pass | 安装脚本输出 `geoview >= 0.1.10` |
| CI 最小权限与提交可追溯 | static | pass | read-only build / write-only publish；actions 与工具固定 SHA；Release tag 两次回读等于 `GITHUB_SHA` |

## Requirement Reconciliation

结论：**4 DELIVERED / 2 DEFERRED / 0 DROPPED**；其中 2 项为实施期间新增并已交付（ADDED-IN-FLIGHT）。

| Scope | Classification | Result / reason |
|-------|----------------|-----------------|
| 本地 geodata 构建、manifest、required/forbidden 探针与 checksum | DELIVERED | 完整在线构建和确定性验证通过 |
| GitHub Actions build/publish 实现与供应链约束 | DELIVERED | 已实现最小权限、固定 revision、draft 精确回读、`GITHUB_SHA` 绑定 |
| `tools/clash2passwall` 并仓、全面加固与分组名修复 | ADDED-IN-FLIGHT / DELIVERED | subtree 已并入；YAML、映射、稳定 UCI ID、设备 ID 冲突回退与控制字符均有回归测试 |
| 事务安装器与 fake-UCI 故障注入 | ADDED-IN-FLIGHT / DELIVERED | 成功路径、stage commit、base64、final commit 回滚及节点保留均通过 |
| 公开仓库创建、remote、push、首次线上 Release 与 latest URL | DEFERRED | 属外部写入/发布，用户未授权；解除条件为明确授权目标 owner/repo 与首次发布 |
| 真实 OpenWrt/libuci、PassWall2 `rule_update`、xray 与 sing-box/geoview 消费 | DEFERRED | 当前无可丢弃设备环境；解除条件为具备 PassWall2 与 geoview >= 0.1.10 的测试设备 |

## Review Disposition

独立审查此前确认的高/中发现已全部修复：严格 YAML 类型、完整 process-only、mandatory `sides`、manifest 驱动映射、C0/C1 控制字符、base64 载荷、显式 repo、稳定具名 UCI section、跨类型 ID 冲突回退、事务回滚、最小权限、固定供应链、精确四资产、干净 checkout 解释器和 Release SHA 绑定。最终 completeness 与 tests/CI 复审均为 0 findings；最终安全/功能替补复审见证据索引。

## Deterministic Evidence

- Python：`.venv/bin/python -m unittest discover -s tests -v` → 50/50。
- 干净 checkout：无 `.venv` 的 `git archive HEAD` 中，fail-fast 3/3 通过。
- Node：`npm ci --ignore-scripts`；`npm test` → 7/7；`npm audit` → 0 vulnerabilities。
- Static：`git diff --check`、`bash -n scripts/bootstrap_vendor.sh`、Python compileall、全部 Node `--check` 通过。
- 完整构建：`bash scripts/bootstrap_vendor.sh` 后运行 `PATH="$PWD/vendor/bin:$PATH" .venv/bin/python scripts/build.py`，精确产出四项。
- converter：仓内 mini Clash fixture 以 `--dat --tag-manifest build/expected_tags.json` 转换；`verify_manifest_refs.cjs` 通过。
- probes：geosite required/forbidden、geoip required/forbidden 全通过。
- checksums：`geosite.dat: OK`、`geoip.dat: OK`。

本次在线产物快照：

| Asset | Bytes | SHA-256 |
|-------|------:|---------|
| `geosite.dat` | 9,119,435 | `46d83ddc302a7b5e36e13670cb2c667177e352466a250acd0de19eee21975412` |
| `geoip.dat` | 17,561,369 | `a1d38476cfd3b5d55c7f8d892639959471bfe0c8e0909e345b5a0610a8cf1db1` |

## Diagnosis Note

验收中第一次单独运行 probe 时未继承 `vendor/bin` PATH，所有 probe 因找不到正确 geoview 而失败。按根因预测补入与 CI 相同的 `PATH="$PWD/vendor/bin:$PATH"` 后，required 与 forbidden 四组 probe 全部通过；因此分类为验收命令环境错误，不是 dat 内容失败。

## Evidence Index

- Final reviews: `acceptance/reviews/final-completeness.json`、`final-tests-ci.json`、`final-security-functionality.json`
- Historical review and disposition inputs: `acceptance/reviews/{functional,security,quality,contracts,test-coverage,completeness}.json`
- Tests: `tests/`、`tools/clash2passwall/tests/`
- Local build evidence: `build/expected_tags.json`、`publish/{geosite,geoip}.dat[.sha256sum]`（git ignored、可复建）

## coverage_note

已执行与本次变更面相关的全部确定性 unit、integration、CLI e2e、installer harness、supply-chain contract 与在线构建检查。浏览器 visual、a11y、web/API 性能不适用于该 CLI/构建特性。未把 fake-UCI 当作真实设备证据：公开 GitHub Release 与 OpenWrt/PassWall2 双核消费均显式 DEFERRED。finding_critic 两次因连接中断未产出最终文件，按验收工作流由另一名独立审查者替补，不采信主线程自评。
