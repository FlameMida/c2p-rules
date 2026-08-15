# context.md — Go geodata 与 PassWall2 托管分流上下文

## 1. 当前交付

仓库已统一为 Go 1.26 module。`cmd/geodata-build` 提供四个 use-case：

```text
geodata-build bootstrap
geodata-build build
geodata-build verify
geodata-build release-decision
```

数据面统一使用 `geosite.dat` / `geoip.dat`；不保留 Python 构建链、Node 转换器或 `.srs` 交付。Release 固定为两个 dat、安装器、Shadowrocket 胖配置及四份对应 checksum，共八资产。

```text
geoip.dat
geoip.dat.sha256sum
geosite.dat
geosite.dat.sha256sum
install_passwall2_rules.sh
install_passwall2_rules.sh.sha256sum
shadowrocket.conf
shadowrocket.conf.sha256sum
```

## 2. 数据流与信任边界

```text
sources.yaml + custom/geosite + custom/geoip
  -> 严格配置与 Clash 规则解析
  -> 显式 create / merge-base 目标注册
  -> community GeoSite 精确去重合并
  -> 中间 GeoSite DAT 编译与无损解码
  -> proxy/direct 保守残差推导
  -> 最终 GeoSite DAT 第二次编译
  -> base GeoIP + 规范化 CIDR 输入合并
  -> pinned geoip 编译
  -> geoview required + forbidden 探针
  -> config/passwall2-groups.yaml 引用验证
  -> 从最终 dat 展开并渲染 shadowrocket.conf
  -> 八资产 checksum 与事务性发布
```

远程下载只接受 HTTPS，具备连接、TLS、响应头和总请求 deadline、redirect 限制及内容大小上限。外部工具只能从 `.cache/bin` 调用，固定完整 commit、显式参数、超时和有界日志，不经过 shell，也不接受 PATH 中同名替换。

## 3. 输出目标

`sources.yaml` 的 source ID 是追踪标识，`outputs.geosite/geoip.tag` 才是最终 dat tag：

- `create` 要求底包中不存在目标。
- `merge-base` 要求底包中已经存在目标。
- 多个 source 只有在同 tag、同 side、同 mode 时才可共享目标。
- Google 合入 `geosite:google`，YouTube 合入 `geosite:youtube`，Netflix 合入标准双侧 `netflix`。
- 最终 DAT 保留完整 `gfw`、`proxy`、`direct` 公共标签；托管组只消费 `crs-proxy-residual` 与 `crs-direct-residual`。
- 原港澳台专用来源与双侧标签已破坏性移除，不提供兼容入口。

GeoSite 去重键是完整的 `kind + value + attrs`，不会做父子域覆盖推导。GeoIP 只做 CIDR Mask、稳定排序和完全相同前缀去重，不删除父前缀内部的子前缀。

## 4. 自定义文件夹

每个 `custom/geosite/<tag>.yaml` 或 `custom/geoip/<tag>.yaml` 对应一个已由底包或 source 注册的目标。GeoSite 支持 `DOMAIN-SUFFIX`、`DOMAIN`、`DOMAIN-KEYWORD`、`DOMAIN-REGEX`；GeoIP 支持 `IP-CIDR`、`IP-CIDR6` 和可选 `no-resolve`。默认 `apple.yaml`、`cn.yaml` 只含注释，解析结果为空。

未知文件目标、错侧规则、unsupported 规则、控制字符、重复 YAML key、多 document 或非法 scalar 都是构建错误。失败只删除 staging，不改变旧 `build/` 与 `publish/`。

## 5. 分流与安装

`config/passwall2-groups.yaml` 显式列出 14 个托管组：reject、youtube、netflix、spotify、tiktok、ai、apple_services、telegram、google、proxy、tld_not_cn、private、cn、direct。数组顺序是托管优先级；苹果服务为 `[apple, icloud]`；中国大陆为 GeoSite/GeoIP 双 `cn`。完整 GFW 规则并入 proxy 残差，`tld_not_cn` 必须与 proxy 同出口；Shadowrocket 按 group ID 建立单向引用。

renderer 生成稳定 `crs_<id>` section，并设置 `managed_by=clash-rules-srs`。安装器只清理旧 `c2p_` 分流和该标记下的托管 section。它会先在隔离暂存配置中预检所有 Shunt 节点的主字段、FakeDNS 与 proxy-tag 三族：GFW/proxy 冲突或 proxy/tld_not_cn 不等价时 live 配置零修改失败；单侧启用会告警并按非空一侧归一化。成功后删除废弃字段，保留 `default_node`、reject、private、cn、direct；后续失败从完整配置快照恢复节点、规则与 URL。

`reject` 行为不变，`private`、`cn`、`direct` 仍默认直连。`IPOnDemand` 不属于本轮验收矩阵。

Shadowrocket 胖配置不含节点；导入前需先在 App 中添加可供 PROXY 使用的订阅或节点。在配置页粘贴官方 latest URL：

https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf

同一 URL 可交给可信的二维码生成器后扫码，也可打开 URL-encoded scheme：

shadowrocket://config/add/https%3A%2F%2Fgithub.com%2FFlameMida%2Fc2p-rules%2Freleases%2Flatest%2Fdownload%2Fshadowrocket.conf

配置的 `update-url` 固定使用 GitHub 官方 latest URL，不带 `gh-proxy.com`、`ghfast.top` 等镜像前缀。自动更新会覆盖配置正文；App 是否保留策略组选择不属于本仓契约。旧 latest 仍为六文件时，必须在默认分支手动运行 build-geodata Workflow 并设置 `force_publish=true` 完成八资产首次上线。

安装器预检 UCI 无未提交改动、规则更新锁不存在，以及 `uci`、`lua`、`sha256sum`、`base64`、`timeout` 和 updater 均可用。终端实时打印阶段、当前源和等待时间。下载按 `gh-proxy.com → ghfast.top → GitHub 官方` 固定回退，每个源 60 秒总超时（可由 `PASSWALL2_RULE_TIMEOUT` 覆盖）；每次 updater 返回后立即用安装器内置的 Release SHA-256 校验两个 dat。超时、失败或 SHA 不匹配时恢复安装前 dat，清理该次 updater 的精确临时文件及遗留锁，再尝试下一级。

事务顺序固定为：备份 → staging UCI 写入首个镜像的不可变 URL 并验证托管组 → 安装并提交 live 配置 → 调用 updater 更新两个 dat（按三源逐一尝试并即时校验）→ 校验两个 dat SHA-256 → 持久 URL 切到 `latest`（GitHub 官方）。镜像仅加速本次安装，不进入最终配置。所有源失败或 UCI 失败都会逆序回滚配置与 dat；恢复本身失败时保留临时目录并打印各备份路径，完整成功后仍保留权限为 0600 的配置备份供人工恢复。

## 6. 本地维护命令

```bash
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test ./...
go test -tags=integration ./internal/app
go run ./cmd/geodata-build build --repo OWNER/REPO --release-tag local-test
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip --forbid
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --baseline /path/to/latest-assets --baseline-tag latest-tag
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --first-release
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --force
```

source 或 custom 变化后必须重跑普通测试与 integration。分流变化只编辑 `config/passwall2-groups.yaml`；构建会在生成安装器前逐项探针其引用。

## 7. CI 与发布门禁

CI 每日完整构建，不因可能无变化而裁剪上游下载、编译、探针或候选验证。build job 权限为 `contents: read` 且 checkout 不持久化凭据，顺序为 Go test、bootstrap、integration、带真实 repository/tag 的 build、四份 checksum 校验，再通过 `release-decision` 比较两个 dat、规范化安装器与 `shadowrocket.conf` 四个主资产。无有效产物变化时不上传 Artifact，publish job 跳过。

只有 latest API 明确 404 才进入首次发布；API、鉴权、网络、下载、基线完整性或安装器 tag 与发布上下文不一致全部严格失败。默认分支的手动 `force_publish=true` 可跳过旧基线比较，但不能跳过候选构建、八资产验证、Release 回读或默认分支限制。普通 changed 发布在任何远端写入前重新查询并复核同一基线的 ID、tag 和载荷指纹。

publish job 独立使用 `contents: write`，精确比较八资产，创建同名 draft Release，上传后通过 API 回读资产名、target commit 和 tag SHA，再公开并标记 latest。

任何下载、解析、目标前置条件、required/forbidden 探针、分流引用、checksum、资产集合或 Release 回读失败都会阻断发布。

## 8. 验收边界

仓库自动验收覆盖 Go 单元/集成、真实固定工具、安装器 fake UCI 事务、八资产与文档/CI 守卫。真实 OpenWrt/PassWall2 设备上的首次安装、Xray/sing-box 双核命中、更新期间网络体验和物理断电恢复，以及 Shadowrocket 真机导入、自动更新和大 conf 编译耗时，需要已发布 Release 与授权设备；当前状态应记录为 DEFERRED，不能用本地模拟冒充通过。

非目标：`.srs`、mihomo 资产、修改 openwrt-passwall2、自建 MaxMind 数据、无断流热更新，以及未经授权创建远端仓库或 Release。
