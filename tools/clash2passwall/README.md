# clash2passwall

把 Clash Verge Rev 配置中的 `rules`、`rule-providers` 和 `proxy-groups` 转成 PassWall2 `shunt_rules` UCI 配置。本工具已随 geodata 构建链路一起维护，推荐使用 `--dat` 模式消费本项目生成的 `geosite.dat` / `geoip.dat`。

## 安装与测试

```bash
npm ci --ignore-scripts
npm test
```

固定依赖 `js-yaml` 用于完整 YAML；也可用 `--yaml-engine builtin` 强制内置子集解析器。测试同时覆盖两种解析模式、恶意控制字符、标签清单、具名分组和安装器事务回滚。

## dat 模式

先在仓库根构建，得到 `build/expected_tags.json`，再运行：

```bash
node tools/clash2passwall/clash2passwall.js \
  /path/to/clash.yaml \
  --dat \
  --tag-manifest build/expected_tags.json \
  --repo OWNER/clash-rules-srs \
  --out /tmp/passwall2-dat
```

`--tag-manifest` 不可省略。它决定每个 `geosite:` / `geoip:` 引用是否真实存在，也决定 Netflix、Bilibili 是否生成 IP 侧引用；代码中没有第二份静态 geoip tag 清单。

输出：

- `passwall2_shunt_rules_dat.conf`：供审阅的 UCI 片段。
- `mapping_guide_dat.txt`：出站建议与明确降级项。
- `install_shunt_rules_dat.sh`：事务性安装器。

每条分流规则使用稳定合法的具名 UCI section；Clash 原分组名保存在 `option remarks`。中文名会使用稳定 hash 作为内部 ID，但 LuCI 显示的仍是中文 `remarks`；ASCII 名尽量原样保留，碰撞时追加短 hash。

安装器不会嵌入固定 heredoc，而是解码 base64 配置到临时 UCI 目录，完成 URL 修改、旧分流删除、提交和解析验证后再原子替换 `/etc/config/passwall2`。节点与其他 section 保留；临时提交、解码、最终提交任一步失败都会恢复备份。没有 `--repo` 时，执行前必须设置：

```sh
REPO_SLUG=OWNER/clash-rules-srs sh install_shunt_rules_dat.sh
```

兼容 `OWNER=... REPO=...`。脚本拒绝占位或非法仓库名，并拒绝覆盖尚有未提交 UCI 变更的配置。

## 其他模式

```bash
# sing-box .srs 兼容模式
node tools/clash2passwall/clash2passwall.js /path/to/clash.yaml --out /tmp/pw2

# xray geosite/geoip 标准 tag 兼容模式
node tools/clash2passwall/clash2passwall.js /path/to/clash.yaml --xray --out /tmp/pw2-xray
```

可用选项：`--mirror`、`--out`、`--no-install`、`--xray`、`--dat`、`--tag-manifest`、`--repo`、`--yaml-engine auto|builtin|js-yaml`。

不支持的 `PROCESS-*`、逻辑规则、ASN 与 `IP-SUFFIX` 会进入降级报告，不会生成语义错误的规则。
