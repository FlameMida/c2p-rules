# ADR-0001: Shadowrocket 以单份胖 conf 作为发布资产

**Status**: Accepted (2026-08-15)

## 背景

仓库已发布 `geosite.dat` / `geoip.dat` 供 PassWall2 使用。要把同一套分流用到 iOS Shadowrocket。App 没有 `GEOSITE` 语法，也不能加载这两个 dat。

## 决定

发布一份内联全部规则的 `shadowrocket.conf`（外加 `.sha256sum`），作为精确发布集合的第 7、8 号资产。不把现有 dat 当作 iOS 输入，第一版不拆远程 `.list`，不发模块。

## 理由

- Shadowrocket 的公开合约是 Surge 风格文本；dat 是 Xray/sing-box 生态。
- 单文件与现有「精确集合 + checksum + 回读」门禁最合拍（只 +2，不必把集合撑到数十个 list）。
- 社区已有数 MB 单 conf 先例；App 远程配置自动更新即可跟上日更。
- 具名策略组写在同一文件的 `[Proxy Group]`，不依赖拆文件。

## 被否方案

- 直接把 dat URL 填进 GeoLite2 或写 `GEOSITE,<tag>` — 公开合约不存在。
- 瘦 conf + 16 个 RULE-SET list 进 Release — 精确集合会膨胀到约 40 文件。
- 模块 `.module` — 策略写死在行内，一键导入要装多次。
