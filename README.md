# clash-rules-srs

把 Clash 规则源（Loyalsoldier / xiaolin-007 / Sukka 等）**1:1** 自建为 sing-box `.srs` 订阅，由 GitHub Actions **每日自动**拉取原源 → 转换 → 发布。供 OpenWrt PassWall2（sing-box 内核）的 `rule-set:remote:` 订阅，摆脱对第三方聚合源的依赖、保证与原 Clash 规则逐条一致。

> 仅做 sing-box `.srs`。xray 内核请直接用 MetaCubeX/Loyalsoldier 的标准 `geosite.dat`/`geoip.dat`（内容等价），不自建。

## 订阅 URL

托管在仓库 `release` 分支，用 jsdelivr 加速。把 `USER` 换成你的 GitHub 用户名（仓库名 `clash-rules-srs`）：

**域名类**（填进 PassWall2 shunt_rules 的 `domain_list`）：

```
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-reject.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-icloud.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-apple.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-google.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-proxy.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-direct.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-private.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-gfw.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-tld-not-cn.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/xiaolin-youtube.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/xiaolin-netflix.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/xiaolin-spotify.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/xiaolin-bilibili.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/xiaolin-tiktok.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/sukka-ai.srs
```

**IP 类**（填进 `ip_list`）：

```
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-telegramcidr.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-cncidr.srs
https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-lancidr.srs
```

国内若 jsdelivr 不稳，把前缀换成 `https://gh-proxy.com/https://raw.githubusercontent.com/USER/clash-rules-srs@release/...`。

## PassWall2 用法

在 PassWall2「分流规则」编辑页，每条规则的文本框里粘贴 `rule-set:remote:<上面某个 URL>`：

- 域名类 → 填进 **Domain（domain_list）**：`rule-set:remote:https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-reject.srs`
- IP 类 → 填进 **IP（ip_list）**：`rule-set:remote:https://cdn.jsdelivr.net/gh/USER/clash-rules-srs@release/loyalsoldier-cncidr.srs`

sing-box 启动时自动下载，之后每天自动刷新。

> 注：`xiaolin-bilibili` 等含 IP-CIDR 的混合源，其 `.srs` 内同时有域名和 IP；PassWall2 里 domain_list 只匹配域名，若要 IP 也生效，把同一条同时填进 domain_list 和 ip_list。

## 自定义源

编辑 `sources.yaml`，在 `sources:` 下追加一行即可：

```yaml
  - {name: my-rule, behavior: domain, url: "https://example.com/list.txt"}
```

字段：`name`（输出文件名 `<name>.srs`）、`behavior`（`domain`/`ipcidr`/`classical`）、`url`、可选 `format`（默认 `yaml`，纯规则行用 `text`）。下次 CI 自动纳入。

## 本地构建

```bash
brew install sing-box          # macOS；或从 github.com/SagerNet/sing-box releases 下载
pip install -r requirements.txt
python scripts/build.py        # 产出 dist/*.srs
```

## 转换规则（1:1）

- `behavior: domain` 的 `+.foo`/`*.foo` → sing-box `domain_suffix`；精确域名 → `domain`；含通配 → `domain_regex`
- `behavior: ipcidr` → `ip_cidr`
- `behavior: classical` / `text`：每行按 `DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD/DOMAIN-REGEX/IP-CIDR` 分桶；`PROCESS-NAME` 等路由器无意义的跳过
- sing-box source JSON 每类一个 rule（同类在数组内 OR，不同类分 rule 再 OR），`sing-box rule-set compile` 压成 `.srs`，不改内容。

## CI

`.github/workflows/build.yml`：每日 02:17 UTC 自动构建，或 Actions 页手动触发。产物以 orphan commit 推到 `release` 分支（只留最新，jsdelivr `@release` 可加速）。
