# clash-rules-srs

本仓库用单一 Go CLI 把 Clash 规则与标准 GeoSite/GeoIP 底包合成 PassWall2 可由 Xray、sing-box 共同消费的 `geosite.dat` / `geoip.dat`，并生成自动配置分流组的安装脚本。生产构建不需要 Python、Node 或 clash2passwall。

## 产物与语义

每次 Release 精确包含八资产：

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

`sources.yaml` 为远程源的唯一真源。每个 output 显式指定最终 tag 和模式：

- `merge-base`：与底包同名 tag 做精确规则去重后合并，例如 Google、YouTube、Netflix。
- `create`：只允许底包中不存在该 tag 时新建，例如 `ai`。
- source ID 只用于追踪，不再作为 dat tag，也不需要 PassWall2 前缀。

GeoSite 只对完全相同的 `kind + value + attrs` 去重，不把父域和子域互相消除。GeoIP 对 CIDR 做掩码规范化、排序和精确去重，保留 `/24` 内有明确声明的 `/25`。

完整构建会先编译一份仅位于 staging/build 的中间 GeoSite DAT，再从编译后规则推导 `crs-proxy-residual` 与 `crs-direct-residual`，写回合并目录后进行第二次正式编译。最终 DAT 继续保留规则指纹不变的完整 `gfw`、`proxy`、`tld-not-cn`、`direct` 公共标签；托管分流使用两个残差标签，其中代理组还同组引用完整 `tld-not-cn`。

## 自定义规则

自定义目录分为 `custom/geosite` 和 `custom/geoip`。文件名就是要扩展的现有最终 tag；例如 `custom/geosite/apple.yaml` 扩展 `geosite:apple`，`custom/geoip/cn.yaml` 扩展 `geoip:cn`。拼错或不存在的目标会让构建失败，不会悄悄创建新组。

GeoSite YAML 支持：

```yaml
payload:
  - DOMAIN-SUFFIX,example.com
  - DOMAIN,api.example.com
  - DOMAIN-KEYWORD,example
  - DOMAIN-REGEX,^.+\.example\.com$
```

GeoIP YAML 支持 `IP-CIDR`、`IP-CIDR6`，以及唯一可选尾随属性 `no-resolve`：

```yaml
payload:
  - IP-CIDR,192.0.2.0/24,no-resolve
  - IP-CIDR6,2001:db8::/32
```

仓库自带的模板只有注释，默认是语义空集，不会改变 dat。两个 generated-only 残差标签不接受 custom 文件；构建会在编译前拒绝同名 source、community 或 custom 输入。

## 默认 PassWall2 分流

`config/passwall2-groups.yaml` 的数组顺序就是托管分流优先级，共 13 组：广告拦截、YouTube、Netflix、Spotify、TikTok、AI 服务、苹果服务、Telegram、Google 服务、代理规则、私有网络、中国大陆、直连规则。

其中苹果服务配置 `geosite:apple` 与 `geosite:icloud`；中国大陆同时配置 `geosite:cn` 与 `geoip:cn`。原 GFW 规则已并入“代理规则”，代理组依次引用 `crs-proxy-residual` 与完整 `tld-not-cn`，“直连规则”只引用 `crs-direct-residual`。完整 `tld-not-cn` 来源、custom 目标和 DAT 公共标签继续保留，但不再有独立 PassWall2/Shadowrocket 策略组；两类规则统一使用“代理规则”出口。原港澳台专用来源与双侧标签已破坏性删除，不保留兼容入口。

生成的具名 UCI section 使用 `crs_` 命名空间并带 `managed_by=clash-rules-srs` 标记。`reject` 的规则与默认出口不变；`private`、`cn`、`direct` 仍默认直连。`IPOnDemand` 不在本轮验收范围，路由器建议继续使用 `IPIfNonMatch`。

## 路由器侧提醒事项

以下设置来自真实 PassWall2 设备上的最终持久化配置，只记录实际修改并保留生效的项目，不包含排障期间已回滚的临时设置：

| PassWall2 界面位置 | 应用值 | 提醒 |
| --- | --- | --- |
| 基本设置 → 主要 → 客户端代理 | 开启 | 关闭时，未单独配置访问控制的局域网设备不会进入透明代理。 |
| 基本设置 → 分流规则 → Domain Strategy | `IPIfNonMatch` | 先匹配 GeoSite 域名规则；仅在域名规则未命中时解析 IP 并再次匹配，避免 `IPOnDemand` 在域名分流前提前解析。 |
| 基本设置 → DNS → 远程 DNS 协议 | `DoH` | 避免受污染的明文远程 DNS 结果进入分流链。 |
| 基本设置 → DNS → 远程 DNS DoH | `https://dns.google/dns-query,8.8.8.8` | 逗号后的地址是 DoH 域名的 bootstrap IP，不能省略。 |
| 节点订阅 → 编辑对应订阅（当前为“良心云”）→ allowInsecure | 开启 | 设置在订阅层，避免自动更新更换节点 ID 后丢失；当前默认 Hysteria2 节点必须同时保留 `tls_pinSHA256`。若后续订阅不再提供证书指纹，应关闭此项并修复服务端证书。 |

订阅自动更新后应复查当前默认节点仍有 `tls_allowInsecure=1` 和非空的 `tls_pinSHA256`，并确认 Hysteria2、Xray 进程均已启动。仅看到 Xray 监听端口不代表代理链路完整可用。

## 本地构建与验证

需要 Go 1.26.x、Git 和网络访问：

```bash
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test ./...
go test -tags=integration ./internal/app
go run ./cmd/geodata-build build --repo OWNER/REPO --release-tag local-test
```

`geodata-build bootstrap` 把滚动的 domain-list-community 和三个固定 commit 的 Go 工具放入 `.cache/`。`geodata-build build` 事务性生成 `build/`、`publish/`；任一步失败都保留上一次完整目录。只准备和检查输入可加 `--skip-compile`，此模式不切换正式产物。

也可单独执行正向或负向 tag 验证：

```bash
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite --forbid
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip --forbid
```

本地已有候选八资产时，可使用同一 Go CLI 复现发布判定：

```bash
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --baseline /path/to/latest-assets --baseline-tag latest-tag
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --first-release
go run ./cmd/geodata-build release-decision --candidate publish --candidate-tag candidate-tag --force
```

三个模式必须恰选一种，并且都会先严格验证候选八资产及安装器 `RELEASE_TAG` 与 `--candidate-tag` 的绑定；baseline 模式还要求安装器 tag 等于 `--baseline-tag`。成功输出固定为 `should_publish`、`reason`、`baseline_fingerprint` 三行；参数形状错误返回 2，资产、tag 绑定或完整性错误返回 1。

## 导入 Shadowrocket

Shadowrocket 配置使用同一套最终 GeoSite/GeoIP 数据展开出的胖配置，不含节点；导入前需先在 App 中添加可供 PROXY 使用的订阅或节点。必须支持的入口是在 Shadowrocket 配置页粘贴：

https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf

也可以把同一 HTTPS URL 交给可信的本地或在线二维码生成器，再用 Shadowrocket 扫码；仓库不发布二维码图片。作为增强入口，可打开 URL-encoded scheme：

shadowrocket://config/add/https%3A%2F%2Fgithub.com%2FFlameMida%2Fc2p-rules%2Freleases%2Flatest%2Fdownload%2Fshadowrocket.conf

配置内的 `update-url` 始终是上述 GitHub 官方 latest URL，不写 `gh-proxy.com`、`ghfast.top` 等镜像前缀。自动更新会覆盖配置正文；App 是否保留组内当前选择不属于本仓契约。

发布集合从旧版切换为八资产后，旧 latest 无法通过严格 baseline 比较。首次上线必须在默认分支手动运行 build-geodata Workflow，并设置 `force_publish=true`；此后普通构建会把 `shadowrocket.conf` 原始字节纳入变化判定。

## 安装到 PassWall2

下载同一不可变 Release tag 中的脚本和校验文件，先校验再执行：

```sh
sha256sum -c install_passwall2_rules.sh.sha256sum
sh install_passwall2_rules.sh
```

脚本会先检查 UCI 无未提交改动且 PassWall2 没有正在运行的规则更新实例，并在终端实时打印阶段、当前下载源和等待时间。下载顺序固定为 `gh-proxy.com → ghfast.top → GitHub 官方`，默认每个源 60 秒总超时；每次 updater 返回后都用脚本内置的 Release SHA-256 校验两个 dat，超时、失败或哈希不匹配会恢复安装前的 dat，再切换下一个源。可用 `PASSWALL2_RULE_TIMEOUT` 覆盖逐源超时秒数。

事务顺序固定为：备份 → staging UCI 写入首个镜像的不可变 Release URL → 预检所有 Shunt 节点中历史 GFW/proxy/tld-not-cn 的主出口、FakeDNS 与 proxy-tag → 验证托管组 → 安装并提交 live 配置 → 调用 updater 更新两个 dat（按三源逐一尝试并即时校验）→ 校验两个 dat SHA-256 → 持久 URL 切到 `latest`（GitHub 官方）。GFW 与 proxy 先按既有规则归一：仅一侧非空时明确警告并采用非空侧；归一结果必须与历史 tld-not-cn 三个配置族逐项等价，否则在 live 修改前失败。已经只含规范 proxy 字段的节点可直接重复安装。成功安装会清理废弃 GFW、tld-not-cn 与港澳台节点字段，同时保留 `default_node`、reject 及三组直连字段。镜像 URL 只用于本次安装加速，不会持久写入最终配置。重复执行保持幂等；只清理旧 `c2p_` 与本项目 `managed_by=clash-rules-srs` 的 section。配置提交、所有更新源或哈希验证任一步失败都会把节点、托管规则、URL 和两个 dat 一并回滚；自动恢复不完整时保留临时恢复目录并逐项打印配置与 dat 的人工恢复路径。

设备必须已有 PassWall2、`rule_update.lua`、`uci`、`lua`、`sha256sum`、`base64` 与 `timeout`（PassWall2 上游软件包依赖的 `coreutils-timeout`）。真实路由器上的首次安装、双内核实际分流和断网恢复仍属于设备侧验收，仓库 CI 不假装覆盖这一边界。

## CI 与发布边界

`.github/workflows/build.yml` 每日完整构建：只读 build job 仍运行 Go 单元测试、固定工具 bootstrap、真实工具集成测试、完整构建及四份 checksum 校验。随后把候选的两个 dat、规范化安装器与 `shadowrocket.conf` 四个主资产和可信 latest 比较；没有有效产物变化（内容无变化）时成功结束，不上传 Artifact，也不创建 tag 或 Release。

只有 GitHub latest API 明确 404 才按首次发布处理；查询、鉴权、下载失败或不可信八资产会严格失败。默认分支可手动设置 `force_publish=true` 修复不可信基线，但仍须完成候选构建、checksum、八资产验证和发布回读，非默认分支永不发布。普通 changed 发布在远端写入前重新下载基线并复核 Release ID、tag 和载荷指纹。

独立 publish job 才获得 `contents: write`，创建与构建注入相同的不可变 tag，上传并通过 GitHub API 回读八资产和 target commit，之后才公开并切换 `latest`。

本项目不发布 `.srs`，不修改 PassWall2 源码，也不承诺无断流热更新。仓库/Release 的首次线上创建需要仓库所有者另行授权；示例中的 `OWNER/REPO` 不是可直接使用的公开地址。
