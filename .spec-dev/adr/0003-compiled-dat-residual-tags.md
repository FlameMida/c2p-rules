# ADR-0003: 从编译后 DAT 派生运行时残差标签

**Status**: Accepted (2026-08-16)

## 背景

完整公共 GeoSite 标签包含 include、regexp 与 attributes，直接按源文本裁剪无法可靠还原最终匹配语义；只在 PassWall2 或 Shadowrocket 渲染阶段过滤又会形成两套实现。

## 决定

先编译中间 GeoSite DAT，从中无损解码并以保守覆盖证明生成 namespaced 残差标签，再编译最终 DAT；托管组引用残差标签，完整公共 `gfw`、`proxy`、`direct` 标签继续保留。

## 理由

编译后输入包含真实展开语义，一份确定的残差结果可同时降低 PassWall2 加载集合和 Shadowrocket 胖配置，并把无法证明安全的规则默认保留。

## 被否方案

- 按来源文本或仅按 `.cn`/TLD 过滤——无法覆盖 include、keyword、regexp、attributes 与规范化差异。
- 在 PassWall2 和 Shadowrocket 渲染器分别去重——会复制规则语义，并违背 Shadowrocket 从最终 DAT 展开的既有决策。
