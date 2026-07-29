# ADR 0002: geodata 完整增强底的构成

## 状态

Accepted — 2026-07-30

## 背景

「完整增强版」可指：(a) 仅自定义 tag；(b) 自定义 + 标准别名；(c) 社区/官方全量 + 自定义；(d) 复刻 v2ray-rules-dat 二次聚合。PassWall2 默认分流依赖 `geosite:cn`/`geoip:cn` 等，且用户希望双核共用自建 dat。

## 决策

采用**轻量完整增强**：

- **geosite 底**：`v2fly/domain-list-community` 的 `data/` 全量，再并入自定义 list 文件，用 `domain-list-custom` 一次打包。
- **geoip 底**：以 `Loyalsoldier/geoip` 已发布的 `geoip.dat` 为 `v2rayGeoIPDat` input，再 add 自定义 CIDR list；**不**在 CI 中自备 MaxMind license 重建国家库。
- **不**复刻 `v2ray-rules-dat` 的 gfwlist 等二次聚合流水线。

## 理由

- 保留标准 tag，避免替换默认 dat 后 China/Private 等规则静默失效。
- 官方 geoip.dat 作底可避免 MaxMind secret 与周更国家库维护成本。
- v2ray-rules-dat 级聚合与「sources 1:1 自建」目标重叠且维护成本高（YAGNI）。

## 后果

- geosite 语义接近 community，而非 Loyalsoldier 增强 geosite 的每一处差异。
- geoip 自定义 tag 叠加在官方增强 geoip 之上；上游变更会进入底包。
- CI 需 checkout community data 并下载上游 geoip.dat。
