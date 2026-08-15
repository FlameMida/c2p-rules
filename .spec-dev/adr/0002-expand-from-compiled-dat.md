# ADR-0002: Shadowrocket 规则从编译后的 dat 展开

**Status**: Accepted (2026-08-15)

## 背景

`build/data-merged/<tag>` 仍含未展开的 `include:`。`build/ip/<tag>.txt` 对 `merge-base` 只含本仓增量，底包 CIDR 在 `geoip.dat` 里。若只读这两处文本，iPhone 上的名单会小于路由器实际命中的集合。

## 决定

在 `geosite.dat` / `geoip.dat` 编译完成且 tag 探测通过之后，按托管组引用从**最终 dat** 抽出完整域名与 CIDR，再写成 conf 的 `[Rule]`。禁止只读未展开的 data-merged 或增量 `ip/*.txt` 作为发布内容。

## 理由

- PassWall2 消费的就是这两个 dat；从同一产物展开才能「两端同一套名单」。
- `include:` 只在编进 dat 时展开；merge-base 的国家/Telegram 等 IP 底包从不出现在增量文本里。

## 被否方案

- 只解析 data-merged + 增量 CIDR，用 `GEOIP,CN` 补国家 — 实现短，但 telegram/netflix/cn 的 IP 与路由器不一致。
- 只导出 `Contribution`（本仓增量、不含 community）— apple/youtube/cn 残缺。
