# ADR 0001: 自定义 tag 与 sources.yaml name 对齐

## 状态

Superseded by [ADR 0003](0003-source-output-targets.md) — 2026-07-31

## 背景

1:1 自建源写入 `geosite.dat`/`geoip.dat` 时需要稳定的 list 名。候选包括：与 `sources.yaml` 的 `name` 一致、与 Script.js provider 短键一致、或加 `pw2-` 前缀隔离。

## 决策

自定义 tag **必须**等于 `sources.yaml` 的 `name`（如 `loyalsoldier-gfw`、`xiaolin-netflix`）。

## 理由

- 构建清单、产物探针、clash2passwall 映射、文档可共用同一字符串，减少三套命名。
- `loyalsoldier-*` / `xiaolin-*` 前缀已与 community 标准文件名隔离，无需再加 `pw2-`。
- 短键（`gfw`）易与官方/社区同名 list 混淆。

## 后果

- PassWall2 分流写法较长：`geosite:loyalsoldier-gfw`。
- 改名必须同时改 sources、构建产物期望与转换映射。
