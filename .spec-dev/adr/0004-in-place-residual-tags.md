# ADR-0004: 用 proxy/direct 原位承载编译后残差

**Status**: Accepted (2026-08-16)

**Supersedes**: [ADR-0003](0003-compiled-dat-residual-tags.md)

## 背景

ADR-0003 通过新增 namespaced 残差标签降低运行时加载集合，但最终 DAT 同时保留完整公共标签和残差副本，导致发布的 `geosite.dat` 明显增大。当前发布物只服务本仓托管的 PassWall2 与 Shadowrocket 配置，不再要求旧 GeoSite 标签兼容。

## 决定

继续从中间 GeoSite DAT 无损解码并执行保守覆盖证明，但将结果原位写回最终标签：

- `gfw ∪ proxy` 经前序覆盖裁剪后，与完整 `tld-not-cn` 精确去重合并，覆写 `proxy`；
- 完整 `direct` 经最终前序活动标签覆盖裁剪后，覆写 `direct`；
- 最终 DAT 删除中间标签 `gfw`、`tld-not-cn`，并禁止旧 `crs-proxy-residual`、`crs-direct-residual` 标签；
- PassWall2 与 Shadowrocket 的 proxy/direct 组只消费同名最终标签。

## 理由

原位覆写保留了一份共享的编译后残差实现，同时避免完整标签与残差标签在最终 DAT 中重复存储。把 `tld-not-cn` 实体并入 `proxy` 后，运行消费者也只需展开一个代理标签。

## 代价

这是显式破坏性变更：直接消费最终 `gfw`、`tld-not-cn` 或两个旧 `crs-*` 标签的外部配置必须迁移到 `proxy`；最终 `proxy`、`direct` 也不再具有上游完整标签语义。

## 被否方案

- 保留完整标签并新增残差标签——运行时规则较小，但最终 DAT 因重复内容增大。
- 只给托管组引用新标签、保留 `tld-not-cn` companion tag——仍保留多份数据与额外消费者引用。
- 在 PassWall2 和 Shadowrocket 渲染阶段分别裁剪——复制匹配语义并形成两套实现。
