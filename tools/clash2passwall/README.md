# clash2passwall

把 **Clash Verge Rev** 的分流规则（`clash-verge.yaml`）一键转换成 **PassWall2** 的 `shunt_rules` UCI 配置，规则集自动映射到 MetaCubeX `meta-rules-dat` 的 sing-box `.srs` 订阅源（国内镜像）。

> 前提：PassWall2 已切换 **sing-box 内核**（xray 内核不支持 `.srs` 订阅）。

## 快速使用

```bash
# 在 Mac 上（已装 Node.js）
node clash2passwall.js "/Users/flame/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/clash-verge.yaml"
```

生成到 `./output/`：
- `passwall2_shunt_rules.conf` — UCI 配置片段（审阅用）
- `install_shunt_rules.sh` — 路由器端一键安装脚本（自包含）
- `mapping_guide.txt` — 出站映射建议 + 降级报告

## 选项

| 选项 | 说明 |
|---|---|
| `--mirror <name>` | `.srs` 镜像（仅 sing-box 模式）：`gh-proxy`(默认) / `ghfast` / `ghproxy` / `jsdelivr` / `fastly` / `raw` |
| `--out <dir>` | 输出目录（默认 `./output`） |
| `--xray` | 生成 **xray 内核**版本：rule-set 转 `geosite:`/`geoip:` 前缀，不用 `.srs` 订阅 |
| `--no-install` | 只生成 `.conf`，不生成安装脚本 |

### sing-box 模式（默认）vs xray 模式（`--xray`）

- **sing-box 模式**：RULE-SET → `rule-set:remote:...srs` 订阅，sing-box 内核自动每天刷新，不依赖 geo 文件。Shunt 节点类型选 **Sing-Box**。
- **xray 模式**：RULE-SET → `geosite:`/`geoip:` 前缀，依赖路由器上的 `geosite.dat`/`geoip.dat`（由 PassWall2 "规则管理"页的定时更新维护）。Shunt 节点类型选 **Xray**。需确保 geosite.dat 含用到的分类（MetaCubeX/Loyalsoldier 源均含 `category-ads-all`/`cn`/`gfw`/`apple`/`youtube` 等）。

xray 模式输出文件带 `_xray` 后缀，不会覆盖 sing-box 版。

## 映射规则

| Clash 规则 | PassWall2 写法 |
|---|---|
| `DOMAIN,x` | `full:x` |
| `DOMAIN-SUFFIX,x` | `domain:x` |
| `DOMAIN-KEYWORD,x` | 裸 `x` |
| `DOMAIN-REGEX,x` | `regexp:x` |
| `GEOSITE,x` | `geosite:x`（用本地 geosite.dat） |
| `GEOIP,x` | `geoip:x` |
| `IP-CIDR/x6/SUFFIX` | 裸 CIDR |
| `RULE-SET,name` | 查内置表 → `rule-set:remote:...srs` |
| `MATCH,p` | Shunt 节点的 Default |
| `PROCESS-*`/`IP-ASN`/`AND/OR/NOT`/`SUB-RULE` | **降级**（写进报告，不转换） |

## 路由器端安装

`install.sh` 会自动：备份 `/etc/config/passwall2` → **清空所有现有 shunt_rules（含自带的 DirectGame/ProxyGame/Direct/China/QUIC/UDP…）** → 写入 Clash 转换的新规则。

```sh
scp output/install_shunt_rules.sh root@路由器IP:/tmp/
ssh root@路由器IP "sh /tmp/install_shunt_rules.sh"
```

然后到 LuCI：节点列表 → 编辑（或新建）一个 sing-box **Shunt** 类型节点 → 为每条新分流规则选出站 → 基本设置里把主节点设为该 Shunt 节点 → 保存并应用。

回滚：`cp /etc/config/passwall2.bak.* /etc/config/passwall2 && uci commit passwall2 && /etc/init.d/passwall2 restart`

## 关于实时更新

sing-box 内核会自动按 **1 天**周期刷新所有 `rule-set:remote:` 的 `.srs`，无需重启、无需 Mac 端常开。若要更快（分钟级），需解除 `util_sing-box.lua` 里 `update_interval` 的注释（见调研结论）。
