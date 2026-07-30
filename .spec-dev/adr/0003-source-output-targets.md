# ADR 0003: Source 身份与输出目标分离

## 状态

Accepted — 2026-07-31；Supersedes ADR 0001

## 决策

`sources.yaml` 使用唯一 Source ID 表示来源身份，并为每个 side 显式声明 `{tag, mode: create|merge-base}`；旧 `sides` schema 与旧前缀 tag 不再兼容。这样才能在保留来源审计信息的同时安全地区分“创建新 tag”和“并入底包同名 tag”，未声明的碰撞仍失败。
