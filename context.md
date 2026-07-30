# context.md — Go geodata 与 PassWall2 托管分流上下文

## 1. 当前交付

仓库已统一为 Go 1.26 module。`cmd/geodata-build` 提供三个 use-case：

```text
geodata-build bootstrap
geodata-build build
geodata-build verify
```

数据面统一使用 `geosite.dat` / `geoip.dat`；不保留 Python 构建链、Node 转换器或 `.srs` 交付。Release 固定为两个 dat、两个 dat checksum、`install_passwall2_rules.sh` 和 `install_passwall2_rules.sh.sha256sum` 六资产。

## 2. 数据流与信任边界

```text
sources.yaml + custom/geosite + custom/geoip
  -> 严格配置与 Clash 规则解析
  -> 显式 create / merge-base 目标注册
  -> community GeoSite 精确去重合并
  -> base GeoIP + 规范化 CIDR 输入合并
  -> pinned domain-list-custom / geoip
  -> geoview required + forbidden 探针
  -> config/passwall2-groups.yaml 引用验证
  -> 六资产 checksum 与事务性发布
```

远程下载只接受 HTTPS，具备连接、TLS、响应头和总请求 deadline、redirect 限制及内容大小上限。外部工具只能从 `.cache/bin` 调用，固定完整 commit、显式参数、超时和有界日志，不经过 shell，也不接受 PATH 中同名替换。

## 3. 输出目标

`sources.yaml` 的 source ID 是追踪标识，`outputs.geosite/geoip.tag` 才是最终 dat tag：

- `create` 要求底包中不存在目标。
- `merge-base` 要求底包中已经存在目标。
- 多个 source 只有在同 tag、同 side、同 mode 时才可共享目标。
- Google 合入 `geosite:google`，YouTube 合入 `geosite:youtube`，Netflix 合入标准双侧 `netflix`。
- `BilibiliHMT` 使用独立 create 双侧 tag，不与底包 `bilibili` 合并。

GeoSite 去重键是完整的 `kind + value + attrs`，不会做父子域覆盖推导。GeoIP 只做 CIDR Mask、稳定排序和完全相同前缀去重，不删除父前缀内部的子前缀。

## 4. 自定义文件夹

每个 `custom/geosite/<tag>.yaml` 或 `custom/geoip/<tag>.yaml` 对应一个已由底包或 source 注册的目标。GeoSite 支持 `DOMAIN-SUFFIX`、`DOMAIN`、`DOMAIN-KEYWORD`、`DOMAIN-REGEX`；GeoIP 支持 `IP-CIDR`、`IP-CIDR6` 和可选 `no-resolve`。默认 `apple.yaml`、`cn.yaml` 只含注释，解析结果为空。

未知文件目标、错侧规则、unsupported 规则、控制字符、重复 YAML key、多 document 或非法 scalar 都是构建错误。失败只删除 staging，不改变旧 `build/` 与 `publish/`。

## 5. 分流与安装

`config/passwall2-groups.yaml` 显式列出 16 个托管组及双侧引用。数组顺序是托管优先级，YouTube 位于 Google 之前；苹果服务为 `[apple, icloud]`；中国大陆为 GeoSite/GeoIP 双 `cn`；BilibiliHMT 独立双侧。

renderer 生成稳定 `crs_<id>` section，并设置 `managed_by=clash-rules-srs`。安装器只清理旧 `c2p_` 分流和该标记下的托管 section，保留用户规则、节点及其原顺序。

事务顺序固定为：备份 → staging UCI 写入不可变 URL 并验证托管组 → 安装并提交 live 配置 → 调用 updater 更新两个 dat → 校验两个 dat SHA-256 → 持久 URL 切到 `latest`。任何 dat、updater 或 UCI 失败都会逆序回滚配置与 dat；恢复本身失败时保留临时目录并打印各备份路径，完整成功后仍保留权限为 0600 的配置备份供人工恢复。

## 6. 本地维护命令

```bash
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test ./...
go test -tags=integration ./internal/app
go run ./cmd/geodata-build build --repo OWNER/REPO --release-tag local-test
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip --forbid
```

source 或 custom 变化后必须重跑普通测试与 integration。分流变化只编辑 `config/passwall2-groups.yaml`；构建会在生成安装器前逐项探针其引用。

## 7. CI 与发布门禁

build job 权限为 `contents: read` 且 checkout 不持久化凭据，顺序为 Go test、bootstrap、integration、带真实 repository/tag 的 build、三份 checksum 校验、六资产 artifact。publish job 独立使用 `contents: write`，精确比较六个文件，创建同名 draft Release，上传后通过 API 回读资产名、target commit 和 tag SHA，再公开并标记 latest。

任何下载、解析、目标前置条件、required/forbidden 探针、分流引用、checksum、资产集合或 Release 回读失败都会阻断发布。

## 8. 验收边界

仓库自动验收覆盖 Go 单元/集成、真实固定工具、安装器 fake UCI 事务、六资产与文档/CI 守卫。真实 OpenWrt/PassWall2 设备上的首次安装、Xray/sing-box 双核命中、更新期间网络体验和物理断电恢复需要授权设备，状态应记录为 DEFERRED，不能用本地模拟冒充通过。

非目标：`.srs`、mihomo 资产、修改 openwrt-passwall2、自建 MaxMind 数据、无断流热更新，以及未经授权创建远端仓库或 Release。
