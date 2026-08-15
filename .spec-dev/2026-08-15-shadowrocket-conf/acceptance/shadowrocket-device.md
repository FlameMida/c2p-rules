# Shadowrocket 真机验收记录

- 状态：DEFERRED
- 日期：2026-08-15
- Release tag：未发布
- 设备 / Shadowrocket 版本：未授权、未知

## 延后原因

当前没有已发布且包含八资产的 Release，也没有获得 Shadowrocket 真机操作授权。因此未执行 HTTPS 导入、scheme 导入、16 个策略组可见性、下一版配置自动更新或大 conf 真机编译耗时验证；这些项目不得用本地 Go 测试标记为 PASS。

## 后续验收清单

- 在默认分支首次以 `force_publish=true` 发布八资产，并记录 Release tag。
- 从配置页粘贴官方 latest `shadowrocket.conf` URL，记录导入结果。
- 打开 URL-encoded `shadowrocket://config/add/` scheme，记录导入结果。
- 确认 16 个具名策略组可见、默认出口符合 group id 契约，且配置不含节点定义。
- 发布内容不同的下一版 conf，记录自动更新前后的可观察差异。
- 记录设备型号、系统版本、Shadowrocket 版本及大 conf 编译耗时。
