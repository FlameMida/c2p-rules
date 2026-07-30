# clash-rules-srs

把 `sources.yaml` 中的 Clash 规则源注入一套可供 PassWall2 双核使用的自建 geodata：

- `geosite.dat`：`v2fly/domain-list-community` 全量标准 list + 每个域名侧自定义 tag。
- `geoip.dat`：`Loyalsoldier/geoip` 官方增强包 + 每个 IP 侧自定义 tag。
- 两个同名 `.sha256sum`：可直接由 PassWall2 `rule_update` 校验。

这套组合称为“轻量完整增强底”。Release 只发布上述两个 dat 和两个校验文件；本项目不发布 `.srs`，也不再维护 `rule-set:remote:`/jsDelivr 交付路径。

## Release URL

把 `OWNER` 换成 GitHub 仓库所有者：

```text
https://github.com/OWNER/clash-rules-srs/releases/latest/download/geosite.dat
https://github.com/OWNER/clash-rules-srs/releases/latest/download/geosite.dat.sha256sum
https://github.com/OWNER/clash-rules-srs/releases/latest/download/geoip.dat
https://github.com/OWNER/clash-rules-srs/releases/latest/download/geoip.dat.sha256sum
```

sha URL 就是对应 dat URL 的最终文件名后追加 `.sha256sum`。校验文件格式为 `64hex`、两个空格、纯文件名和换行。

## PassWall2 配置

在 PassWall2 规则管理中将：

- `geosite_url` 设为 `https://github.com/OWNER/clash-rules-srs/releases/latest/download/geosite.dat`
- `geoip_url` 设为 `https://github.com/OWNER/clash-rules-srs/releases/latest/download/geoip.dat`

更新规则后，xray 与 sing-box 都可在 `shunt_rules` 中使用同一套引用，例如：

```text
domain_list: geosite:loyalsoldier-gfw
ip_list:     geoip:loyalsoldier-telegramcidr
```

标准兜底仍可使用 `geosite:cn`、`geoip:cn` 和 `geoip:private`。sing-box 路径要求设备上的 `geoview >= 0.1.10`。

## sources 与自定义 tag

自定义 tag 的唯一真源是 [`sources.yaml`](sources.yaml)，tag 等于条目的 `name`：

```yaml
sources:
  - {name: my-domain-list, behavior: domain, sides: [geosite], url: "https://example.com/domain.yaml"}
  - {name: my-ip-list, behavior: ipcidr, sides: [geoip], url: "https://example.com/ip.yaml"}
```

- `domain` 源写入同名 geosite tag。
- `ipcidr` 源写入同名 geoip tag。
- `classical` 源按域名/IP 拆分；有哪一侧就写哪一侧的同名 tag。
- `applications` 或仅含 `PROCESS-*`（包括 PATH/REGEX）的源会跳过。
- `IP-SUFFIX` 无法无损转换为 CIDR，会明确记为 skipped，不写入 geoip。
- 下载/解析失败、期望侧为空或与 community 文件撞名会让整个构建失败，Release 不更新。

Clash provider 到自定义 tag 的映射实现已并入 [`tools/clash2passwall/map_dat.cjs`](tools/clash2passwall/map_dat.cjs)。

## clash2passwall `--dat`

使用仓库内转换器把 Clash rules 转成 `geosite:<tag>` / `geoip:<tag>` 的 PassWall2 UCI 片段。`--tag-manifest` 是当前构建产生的机器可读标签真源；`--repo` 显式确定安装脚本的下载仓库：

```bash
node tools/clash2passwall/clash2passwall.js \
  /path/to/clash.yaml \
  --dat \
  --tag-manifest build/expected_tags.json \
  --repo OWNER/clash-rules-srs \
  --out /tmp/passwall2-dat
```

输出包括 `passwall2_shunt_rules_dat.conf`、映射说明和 `install_shunt_rules_dat.sh`。分流使用稳定具名 UCI section，原 Clash 分组名保存在 `remarks`，不会再出现匿名 `cfg...` 名称。安装脚本先在临时 UCI 目录完成 URL 修改、规则替换和语法验证，再原子替换真实配置；任一步失败都会从备份恢复，节点与其他 section 不会删除。

未在生成时传 `--repo` 时，执行前必须显式提供仓库；脚本不会使用占位 URL：

```sh
REPO_SLUG=OWNER/clash-rules-srs sh install_shunt_rules_dat.sh
```

脚本会同时写入 geosite/geoip latest URL，并提示 geoview 版本要求。

## 本地构建与验证

需要 Python 3.11+、Go，以及网络访问：

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
bash scripts/bootstrap_vendor.sh
export PATH="$PWD/vendor/bin:$PATH"

.venv/bin/python -m unittest discover -s tests -v
npm --prefix tools/clash2passwall ci --ignore-scripts
npm --prefix tools/clash2passwall test
.venv/bin/python scripts/build.py
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geosite.dat --expect build/expected_tags.json --side geosite
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geosite.dat --expect build/expected_tags.json --side geosite --forbid
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geoip.dat --expect build/expected_tags.json --side geoip
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geoip.dat --expect build/expected_tags.json --side geoip --forbid
(cd publish && sha256sum -c geosite.dat.sha256sum && sha256sum -c geoip.dat.sha256sum)
```

`scripts/bootstrap_vendor.sh` 会拉取 community、domain-list-custom、Loyalsoldier/geoip 和 geoview；`scripts/build.py --skip-compile` 可只验证拉源、分桶和中间树。

## CI 与边界

`.github/workflows/build.yml` 每天 02:17 UTC 或手动执行。只读 build job 完成 Python/Node 测试、完整构建、正负 tag 探针和 sha 校验，再把精确四件套交给独立写权限 publish job。publish job 创建新的草稿 Release，上传并经 API 回读确认资产集合恰为四项后才公开并切为 latest，避免用户看到混合代际文件。

当前本地仓库尚未配置获授权的远端；仓库创建、推送和第一次线上 Release 明确延期，不能把上面的 `OWNER` 模板当作已存在的公开 URL。

本项目不做 `.srs` 发布、mihomo 规则交付、PassWall2 源码修改或无断流热更新；规则更新仍由 PassWall2 现有 geodata 更新机制负责。
