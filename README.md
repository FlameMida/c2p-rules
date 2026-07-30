# clash-rules-srs

本仓库用单一 Go CLI 把 Clash 规则与标准 GeoSite/GeoIP 底包合成 PassWall2 可由 Xray、sing-box 共同消费的 `geosite.dat` / `geoip.dat`，并生成自动配置分流组的安装脚本。生产构建不需要 Python、Node 或 clash2passwall。

## 产物与语义

每次 Release 精确包含六个文件：

```text
geoip.dat
geoip.dat.sha256sum
geosite.dat
geosite.dat.sha256sum
install_passwall2_rules.sh
install_passwall2_rules.sh.sha256sum
```

`sources.yaml` 为远程源的唯一真源。每个 output 显式指定最终 tag 和模式：

- `merge-base`：与底包同名 tag 做精确规则去重后合并，例如 Google、YouTube、Netflix。
- `create`：只允许底包中不存在该 tag 时新建，例如 `BilibiliHMT`；它不会与普通 `bilibili` 合并。
- source ID 只用于追踪，不再作为 dat tag，也不需要 PassWall2 前缀。

GeoSite 只对完全相同的 `kind + value + attrs` 去重，不把父域和子域互相消除。GeoIP 对 CIDR 做掩码规范化、排序和精确去重，保留 `/24` 内有明确声明的 `/25`。

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

仓库自带的两份模板只有注释，默认是语义空集，不会改变 dat。

## 默认 PassWall2 分流

`config/passwall2-groups.yaml` 的数组顺序就是托管分流优先级：广告拦截、哔哩哔哩港澳台、YouTube、Netflix、Spotify、TikTok、AI 服务、苹果服务、Telegram、Google 服务、GFW、代理规则、非中国域名、私有网络、中国大陆、直连规则。

其中苹果服务配置 `geosite:apple` 与 `geosite:icloud`；中国大陆同时配置 `geosite:cn` 与 `geoip:cn`；BilibiliHMT 同时配置独立的 GeoSite/GeoIP tag。生成的具名 UCI section 使用 `crs_` 命名空间并带 `managed_by=clash-rules-srs` 标记，用户已有分流组和节点不会被纳入托管清理范围。

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

## 安装到 PassWall2

下载同一不可变 Release tag 中的脚本和校验文件，先校验再执行：

```sh
sha256sum -c install_passwall2_rules.sh.sha256sum
sh install_passwall2_rules.sh
```

脚本会先检查 UCI 无未提交改动。事务顺序固定为：备份 → staging UCI 写入不可变 URL 并验证托管组 → 安装并提交 live 配置 → 调用 updater 更新两个 dat → 校验两个 dat SHA-256 → 持久 URL 切到 `latest`。重复执行保持幂等；只清理旧 `c2p_` 与本项目 `managed_by=clash-rules-srs` 的 section。配置提交、更新器或哈希验证任一步失败都会回滚配置和两个 dat；自动恢复不完整时保留临时恢复目录并逐项打印配置与 dat 的人工恢复路径。

设备必须已有 PassWall2、`rule_update.lua`、`uci`、`lua`、`sha256sum` 与 `base64`。真实路由器上的首次安装、双内核实际分流和断网恢复仍属于设备侧验收，仓库 CI 不假装覆盖这一边界。

## CI 与发布边界

`.github/workflows/build.yml` 的只读 build job 运行 Go 单元测试、固定工具 bootstrap、真实工具集成测试、完整构建及三份 checksum 校验。独立 publish job 才获得 `contents: write`，创建与构建注入相同的不可变 tag，上传并通过 GitHub API 回读六资产和 target commit，之后才公开并切换 `latest`。

本项目不发布 `.srs`，不修改 PassWall2 源码，也不承诺无断流热更新。仓库/Release 的首次线上创建需要仓库所有者另行授权；示例中的 `OWNER/REPO` 不是可直接使用的公开地址。
