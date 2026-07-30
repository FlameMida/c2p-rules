# ADR 0004: 根 Go 编排器调用固定上游工具

## 状态

Accepted — 2026-07-31

## 决策

第一方构建、探针、manifest 与 installer 生成统一为根目录 Go module，临时上游源码和二进制迁至 `.cache/`；`domain-list-custom`、`geoip`、`geoview` 继续按完整 commit 固定并作为受超时控制的 CLI 调用。直接导入或复制上游实现会接管不稳定的 main/plugin/protobuf 边界，而 CLI 隔离能保留既有语义并限制升级半径。
