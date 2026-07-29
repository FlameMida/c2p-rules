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
  - {name: my-domain-list, behavior: domain, url: "https://example.com/domain.yaml"}
  - {name: my-ip-list, behavior: ipcidr, url: "https://example.com/ip.yaml"}
```

- `domain` 源写入同名 geosite tag。
- `ipcidr` 源写入同名 geoip tag。
- `classical` 源按域名/IP 拆分；有哪一侧就写哪一侧的同名 tag。
- `applications` 或仅含 `PROCESS-NAME` 的源会跳过。
- 下载/解析失败、期望侧为空或与 community 文件撞名会让整个构建失败，Release 不更新。

Clash provider 到自定义 tag 的固定映射实现见 companion 项目的 `/Users/flame/clash2passwall/map_dat.cjs`。

## clash2passwall `--dat`

使用 companion 转换器把 Clash rules 转成 `geosite:<tag>` / `geoip:<tag>` 的 PassWall2 UCI 片段：

```bash
node /Users/flame/clash2passwall/clash2passwall.js \
  /path/to/clash.yaml \
  --dat \
  --out /tmp/passwall2-dat
```

输出包括 `passwall2_shunt_rules_dat.conf`、映射说明和 `install_shunt_rules_dat.sh`。安装脚本只覆盖 `shunt_rules`，不会删除用户节点；用环境变量指定实际仓库：

```sh
OWNER=YOUR_GITHUB_USER REPO=clash-rules-srs sh install_shunt_rules_dat.sh
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
.venv/bin/python scripts/build.py
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geosite.dat --expect build/expected_tags.json --side geosite
.venv/bin/python scripts/probe_tags.py \
  --dat publish/geoip.dat --expect build/expected_tags.json --side geoip
(cd publish && sha256sum -c geosite.dat.sha256sum && sha256sum -c geoip.dat.sha256sum)
```

`scripts/bootstrap_vendor.sh` 会拉取 community、domain-list-custom、Loyalsoldier/geoip 和 geoview；`scripts/build.py --skip-compile` 可只验证拉源、分桶和中间树。

## CI 与边界

`.github/workflows/build.yml` 每天 02:17 UTC 或手动执行。测试、完整构建、两侧 tag 探针和 sha 校验全部成功后，才更新 GitHub latest Release 的四个资产。

本项目不做 `.srs` 发布、mihomo 规则交付、PassWall2 源码修改或无断流热更新；规则更新仍由 PassWall2 现有 geodata 更新机制负责。
