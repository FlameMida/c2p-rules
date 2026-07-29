#!/usr/bin/env node
/*
 * clash2passwall.js — Clash Verge Rev → PassWall2 shunt_rules 转换器
 *
 * 作用：读取 Clash 配置（clash-verge.yaml），把其中的 rules / rule-providers / proxy-groups
 *       转换成 PassWall2 的 shunt_rules（分流规则）UCI 配置，RULE-SET 自动映射到
 *       MetaCubeX meta-rules-dat 的 sing-box .srs 订阅源（国内镜像）。
 *
 * 零依赖：内置 Clash YAML 子集解析器；若安装了 js-yaml 则自动改用它（更稳）。
 *
 * 用法：
 *   node clash2passwall.js <clash.yaml> [选项]
 *   node clash2passwall.js "/Users/you/.../clash-verge.yaml" --mirror ghfast
 *
 * 选项：
 *   --mirror <name>   .srs 源镜像：gh-proxy(默认) | ghfast | jsdelivr | fastly | raw
 *   --out <dir>       输出目录（默认 ./output）
 *   --no-install      不生成 install_shunt_rules.sh，只生成 .conf
 */

const fs = require("fs");
const path = require("path");

// ============================================================
// 镜像源（国内稳定优先）
// ============================================================
const MIRRORS = {
  "gh-proxy": "https://gh-proxy.com/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo",
  "ghfast":   "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo",
  "ghproxy":  "https://ghproxy.net/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo",
  "jsdelivr": "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo",
  "fastly":   "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo",
  "raw":      "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo",
};
let MIRROR = MIRRORS["gh-proxy"];
let MODE = "sing-box"; // "sing-box" | "xray"
const srs = (type, name) => `rule-set:remote:${MIRROR}/${type}/${name}.srs`;

// ============================================================
// RULE-SET 名称 → MetaCubeX .srs 映射表
//   t: geosite|geoip   n: 规则集名   f: domain|ip（写入 PassWall2 哪个字段）
// ============================================================
const RULESET_MAP = {
  // Loyalsoldier/clash-rules 常见名
  reject:        { t: "geosite", n: "category-ads-all", f: "domain" },
  icloud:        { t: "geosite", n: "icloud",           f: "domain" },
  apple:         { t: "geosite", n: "apple",            f: "domain" },
  microsoft:     { t: "geosite", n: "microsoft",        f: "domain" },
  google:        { t: "geosite", n: "google",           f: "domain" },
  proxy:         { t: "geosite", n: "geolocation-!cn",  f: "domain" },
  direct:        { t: "geosite", n: "cn",               f: "domain" },
  private:       { t: "geosite", n: "private",          f: "domain" },
  gfw:           { t: "geosite", n: "gfw",              f: "domain" },
  greatfire:     { t: "geosite", n: "geolocation-!cn",  f: "domain" },
  "tld-not-cn":  { t: "geosite", n: "tld-!cn",          f: "domain" },
  telegramcidr:  { t: "geoip",   n: "telegram",         f: "ip" },
  cncidr:        { t: "geoip",   n: "cn",               f: "ip" },
  lancidr:       { t: "geoip",   n: "private",          f: "ip" },
  applications:  null, // 进程类，路由器场景无意义
  // 常见流媒体 / 服务规则集名
  YouTube:       { t: "geosite", n: "youtube",          f: "domain" },
  youtube:       { t: "geosite", n: "youtube",          f: "domain" },
  Netflix:       { t: "geosite", n: "netflix",          f: "domain" },
  netflix:       { t: "geosite", n: "netflix",          f: "domain" },
  Spotify:       { t: "geosite", n: "spotify",          f: "domain" },
  spotify:       { t: "geosite", n: "spotify",          f: "domain" },
  TikTok:        { t: "geosite", n: "tiktok",           f: "domain" },
  tiktok:        { t: "geosite", n: "tiktok",           f: "domain" },
  BilibiliHMT:   { t: "geosite", n: "bilibili",         f: "domain" },
  Bilibili:      { t: "geosite", n: "bilibili",         f: "domain" },
  bilibili:      { t: "geosite", n: "bilibili",         f: "domain" },
  AI:            { t: "geosite", n: "category-ai-!cn",  f: "domain" },
  ai:            { t: "geosite", n: "category-ai-!cn",  f: "domain" },
  OpenAI:        { t: "geosite", n: "openai",           f: "domain" },
  openai:        { t: "geosite", n: "openai",           f: "domain" },
  Telegram:      { t: "geosite", n: "telegram",         f: "domain" },
  telegram:      { t: "geosite", n: "telegram",         f: "domain" },
  Github:        { t: "geosite", n: "github",           f: "domain" },
  github:        { t: "geosite", n: "github",           f: "domain" },
  Disney:        { t: "geosite", n: "disney",           f: "domain" },
  Disneyplus:    { t: "geosite", n: "disney",           f: "domain" },
  HBO:           { t: "geosite", n: "hbo",              f: "domain" },
  Amazon:        { t: "geosite", n: "amazon",           f: "domain" },
  Steam:         { t: "geosite", n: "steam",            f: "domain" },
};

// ============================================================
// YAML 解析（优先 js-yaml，否则内置）
// ============================================================
let jsyaml = null;
try { jsyaml = require("js-yaml"); } catch (e) { jsyaml = null; }

function stripQ(v) {
  v = String(v).trim();
  if (v.length >= 2 && ((v[0] === '"' && v[v.length - 1] === '"') || (v[0] === "'" && v[v.length - 1] === "'"))) {
    return v.slice(1, -1);
  }
  return v;
}

// 内置解析器：只提取 proxy-groups[].name、rule-providers{}、rules[]
function builtinParse(text) {
  const result = { "proxy-groups": [], "rule-providers": {}, rules: [] };
  let section = null;
  let curProvider = null;

  for (const raw of text.split(/\r?\n/)) {
    const line = raw.replace(/\t/g, "  ");
    if (!line.trim() || line.trim().startsWith("#")) continue;

    const top = line.match(/^([A-Za-z_-]+):\s*$/);
    if (top) {
      const k = top[1];
      section = (k === "rules" || k === "proxy-groups" || k === "rule-providers") ? k : null;
      curProvider = null;
      continue;
    }

    if (section === "rules") {
      const m = line.match(/^\s*-\s+(.+)$/);
      if (m) result.rules.push(m[1].trim());
    } else if (section === "proxy-groups") {
      const start = line.match(/^\s*-\s+(.*)$/);
      if (start) {
        const g = {};
        result["proxy-groups"].push(g);
        const nm = start[1].match(/^name:\s*(.+)$/);
        if (nm) g.name = stripQ(nm[1]);
      } else {
        const kv = line.match(/^\s+name:\s*(.+)$/);
        if (kv && result["proxy-groups"].length) {
          result["proxy-groups"][result["proxy-groups"].length - 1].name = stripQ(kv[1]);
        }
      }
    } else if (section === "rule-providers") {
      const provName = line.match(/^  (?! )([A-Za-z0-9_.-]+):\s*$/);
      if (provName) {
        curProvider = provName[1];
        result["rule-providers"][curProvider] = {};
      } else if (curProvider) {
        const kv = line.match(/^    ([A-Za-z_-]+):\s*(.*)$/);
        if (kv) result["rule-providers"][curProvider][kv[1]] = stripQ(kv[2]);
      }
    }
  }
  return result;
}

function loadConfig(file) {
  const text = fs.readFileSync(file, "utf8");
  const cfg = jsyaml ? jsyaml.load(text) : builtinParse(text);
  cfg.rules = cfg.rules || [];
  cfg["rule-providers"] = cfg["rule-providers"] || {};
  cfg["proxy-groups"] = cfg["proxy-groups"] || [];
  return cfg;
}

// ============================================================
// 规则映射
// ============================================================
function applyRuleset(name, out, providers, degrade) {
  const m = RULESET_MAP[name];
  if (m === null) {
    degrade.push({ line: "RULE-SET," + name, reason: "进程类规则(applications)，路由器场景无意义，已跳过" });
    return;
  }
  if (m) {
    if (MODE === "xray") {
      // xray 不支持 rule-set 订阅，改用 geosite:/geoip: 前缀（依赖 geosite.dat/geoip.dat）
      const line = m.t + ":" + m.n;
      if (m.f === "domain") out.domain.push(line);
      else out.ip.push(line);
    } else {
      // sing-box：rule-set:remote:.srs 订阅，内核自动刷新
      const url = srs(m.t, m.n);
      if (m.f === "domain") out.domain.push(url);
      else out.ip.push(url);
    }
    return;
  }
  // 不在映射表，尝试用 provider 声明推断
  const p = providers[name];
  if (p && p.url && /\.srs(\?|$|#)/.test(p.url)) {
    if (MODE === "xray") {
      degrade.push({ line: "RULE-SET," + name, reason: `xray 模式不支持 .srs 订阅且无 geosite 映射，请手动转 geosite:/geoip: 或内联（provider url: ${p && p.url}）` });
      return;
    }
    const field = p.behavior === "ipcidr" ? "ip" : "domain";
    const line = "rule-set:remote:" + p.url;
    if (field === "domain") out.domain.push(line);
    else out.ip.push(line);
    return;
  }
  degrade.push({
    line: "RULE-SET," + name,
    reason: `无内置映射且 provider 非 .srs 源；请手动处理（provider: type=${p && p.type}, behavior=${p && p.behavior}, url=${p && p.url}）`,
  });
}

// 把单条 Clash rule 映射为 { policy, domain[], ip[], port[], source[], match }
function mapRule(rawLine, providers, degrade) {
  const line = rawLine.trim();
  if (!line || line.startsWith("#")) return null;
  const parts = line.split(",").map((s) => s.trim()).filter((s) => s.length);
  if (!parts.length) return null;
  const type = parts[0];
  const out = { domain: [], ip: [], port: [], source: [], match: false, policy: null };

  // policy 通常在 parts[2]（MATCH 在 parts[1]）
  const setPol = (idx) => { if (parts[idx] && !isAttr(parts[idx])) out.policy = parts[idx]; };
  const setPolLast = () => {
    for (let i = parts.length - 1; i >= 1; i--) {
      if (!isAttr(parts[i]) && !isValue(i)) { out.policy = parts[i]; return; }
    }
  };

  switch (type) {
    case "MATCH":
      out.match = true; out.policy = parts[1]; return out;
    case "DOMAIN":         out.domain.push("full:" + parts[1]);     setPol(2); break;
    case "DOMAIN-SUFFIX":  out.domain.push("domain:" + parts[1]);   setPol(2); break;
    case "DOMAIN-KEYWORD": out.domain.push(parts[1]);               setPol(2); break;
    case "DOMAIN-REGEX":   out.domain.push("regexp:" + parts[1]);   setPol(2); break;
    case "GEOSITE":        out.domain.push("geosite:" + parts[1]);  setPol(2); break;
    case "GEOIP":          out.ip.push("geoip:" + parts[1]);        setPol(2); break;
    case "IP-CIDR":
    case "IP-CIDR6":
    case "IP-SUFFIX":      out.ip.push(parts[1]);                   setPol(2); break;
    case "DST-PORT":       out.port.push(parts[1]);                 setPol(2); break;
    case "SRC-IP-CIDR":    out.source.push(parts[1]);               setPol(2); break;
    case "RULE-SET":       applyRuleset(parts[1], out, providers, degrade); setPol(2); break;
    default:
      // 逻辑规则 AND/OR/NOT/SUB-RULE、PROCESS-*、IP-ASN、DSCP 等不支持
      degrade.push({ line: rawLine, reason: `规则类型 "${type}" 不支持转换（逻辑规则/进程/ASN 等），已跳过` });
      return null;
  }
  return out;
}
function isAttr(x) { return x === "no-resolve" || x === "src"; }
// setPol 的简单实现里上面没用到 setPolLast，保留 isValue 占位避免误用
function isValue() { return false; }

// ============================================================
// 输出生成
// ============================================================
function uciMultiValue(lines) {
  // UCI 多行值：单引号包裹，内部用真实换行；转义内部单引号
  const joined = lines.join("\n").replace(/'/g, "'\\''");
  return "'" + joined + "'";
}

function generateConf(order, pr) {
  let out = "";
  for (const policy of order) {
    const r = pr[policy];
    out += "config shunt_rules\n";
    out += "\toption remarks '" + policy.replace(/'/g, "") + "'\n";
    out += "\toption network 'tcp,udp'\n";
    if (r.domain.length) out += "\toption domain_list " + uciMultiValue(r.domain) + "\n";
    if (r.ip.length)     out += "\toption ip_list " + uciMultiValue(r.ip) + "\n";
    if (r.port.length)   out += "\toption port '" + r.port.join(",") + "'\n";
    if (r.source.length) out += "\toption source '" + r.source.join(" ") + "'\n";
    out += "\n";
  }
  return out;
}

function generateInstall(conf, order, mode) {
  // 自包含安装脚本：备份 → 清空所有旧 shunt_rules（含自带）→ 写入新规则 → commit
  const core = mode === "xray" ? "Xray" : "Sing-Box";
  const install = `#!/bin/sh
# 自动生成 — 清空所有旧 shunt_rules（含自带）后写入 Clash 转换的新规则
set -e
CONF=/etc/config/passwall2
TS=$(date +%s 2>/dev/null || echo bak)
cp "$CONF" "$CONF.bak.$TS"
echo "已备份原配置: $CONF.bak.$TS"

# ---- 清空所有现有 shunt_rules（自带的 DirectGame/ProxyGame/Direct/China/QUIC/UDP… 及之前手建的）----
deleted=0
for sec in $(uci show passwall2 2>/dev/null | awk -F'[.=]' '/=shunt_rules$/{print $2}'); do
	echo "  删除旧规则: $(uci -q get passwall2.$sec.remarks 2>/dev/null) ($sec)"
	uci -q delete passwall2.$sec
	deleted=$((deleted + 1))
done
echo "已清空 $deleted 条旧分流规则。"
uci commit passwall2

# ---- 写入新的 Clash 分流规则 ----
cat >> "$CONF" <<'PWEOF'
${conf}PWEOF
uci commit passwall2
echo ""
echo "✅ 已清空旧规则并写入 ${order.length} 条新分流规则。"
echo ""
echo "下一步（在 LuCI 操作）："
echo "  1) 节点列表 → 编辑/新建一个 ${core} 的 Shunt 类型节点"
echo "  2) 在节点编辑页底部表格里，为每条新分流规则选出站节点："
echo "     广告类 → Blackhole(阻断)；直连类 → Direct Connection；其余 → 你的代理节点"
echo "  3) 把 Default（漏网之鱼）设为想要的兜底节点"
echo "  4) 基本设置 → TCP 节点 / UDP 节点 → 选该 Shunt 节点 → 保存并应用"
echo ""
echo "若要撤销：cp \$CONF.bak.$TS \$CONF && uci commit passwall2 && /etc/init.d/passwall2 restart"
`;
  return install;
}

function suggestOutbound(policy) {
  const s = policy.toLowerCase();
  if (/广告|ad|reject|block|阻断|拦截|黑名单/.test(s)) return "_blackhole 阻断";
  if (/直连|direct|私有|private|lan|本地|中国|cn|国内/.test(s)) return "_direct 直连";
  return "→ 选你的代理节点";
}

function generateGuide(order, matchPolicy, groupTypes, degrade) {
  let g = "";
  g += "================ 分流规则 → 出站节点 映射建议 ================\n";
  g += "(在 PassWall2 的 Shunt 节点编辑页，为每条规则选下面的出站)\n\n";
  for (const p of order) {
    const gt = groupTypes[p] ? ` [原 Clash 组类型: ${groupTypes[p]}]` : "";
    g += "  • " + p + gt + "\n";
    g += "      建议出站: " + suggestOutbound(p) + "\n";
  }
  g += "\n";
  if (matchPolicy) {
    g += "  • Default（漏网之鱼，来自 Clash 的 MATCH）: " + matchPolicy + "\n";
    g += "      建议出站: " + suggestOutbound(matchPolicy) + "\n";
  }
  g += "\n注：select/url-test/fallback/load-balance 等动态策略组在 PassWall2 只能指向单个节点，\n";
  g += "    请按上面建议为每个组指定一个具体节点。\n";
  if (degrade.length) {
    g += "\n================ 降级报告（无法自动转换的规则） ================\n";
    for (const d of degrade) g += "  • " + d.line + "\n      原因: " + d.reason + "\n";
  }
  return g;
}

// ============================================================
// 主流程
// ============================================================
function parseArgs(argv) {
  const a = { positional: [], mirror: "gh-proxy", out: null, noInstall: false, xray: false };
  for (let i = 0; i < argv.length; i++) {
    const t = argv[i];
    if (t === "--mirror") a.mirror = argv[++i];
    else if (t === "--out") a.out = argv[++i];
    else if (t === "--no-install") a.noInstall = true;
    else if (t === "--xray") a.xray = true;
    else if (t === "-h" || t === "--help") a.help = true;
    else a.positional.push(t);
  }
  return a;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help || !args.positional.length) {
    console.error("用法: node clash2passwall.js <clash.yaml> [--mirror gh-proxy|ghfast|jsdelivr|fastly|raw] [--out dir] [--xray] [--no-install]");
    process.exit(args.help ? 0 : 1);
  }

  const inputFile = args.positional[0];
  if (!MIRRORS[args.mirror]) {
    console.error("未知镜像: " + args.mirror + "，可选: " + Object.keys(MIRRORS).join("/"));
    process.exit(1);
  }
  MIRROR = MIRRORS[args.mirror];
  MODE = args.xray ? "xray" : "sing-box";

  const cfg = loadConfig(inputFile);
  const providers = cfg["rule-providers"];
  const groupTypes = {};
  for (const g of cfg["proxy-groups"]) if (g.name) groupTypes[g.name] = g.type || "";

  const degrade = [];
  const order = [];
  const pr = {};
  let matchPolicy = null;

  for (const line of cfg.rules) {
    const mapped = mapRule(line, providers, degrade);
    if (!mapped) continue;
    if (mapped.match) { matchPolicy = mapped.policy; continue; }
    if (!mapped.policy) continue;
    if (!pr[mapped.policy]) {
      pr[mapped.policy] = { domain: [], ip: [], port: [], source: [] };
      order.push(mapped.policy);
    }
    const t = pr[mapped.policy];
    t.domain.push(...mapped.domain);
    t.ip.push(...mapped.ip);
    t.port.push(...mapped.port);
    t.source.push(...mapped.source);
  }

  const conf = generateConf(order, pr);
  const guide = generateGuide(order, matchPolicy, groupTypes, degrade);

  const outDir = path.resolve(args.out || path.join(process.cwd(), "output"));
  fs.mkdirSync(outDir, { recursive: true });
  const suffix = MODE === "xray" ? "_xray" : "";
  const confFile = "passwall2_shunt_rules" + suffix + ".conf";
  const installFile = "install_shunt_rules" + suffix + ".sh";
  const guideFile = "mapping_guide" + suffix + ".txt";
  fs.writeFileSync(path.join(outDir, confFile), conf, "utf8");
  fs.writeFileSync(path.join(outDir, guideFile), guide, "utf8");
  if (!args.noInstall) {
    fs.writeFileSync(path.join(outDir, installFile), generateInstall(conf, order, MODE), "utf8");
  }

  // 终端摘要
  console.log("============================================================");
  console.log(" Clash → PassWall2 转换完成  [" + MODE + " 模式" + (MODE === "xray" ? "：rule-set → geosite:/geoip:" : "：rule-set → .srs 订阅") + "]");
  console.log("============================================================");
  console.log(" 输入:        " + inputFile);
  console.log(" YAML 引擎:   " + (jsyaml ? "js-yaml" : "内置解析器"));
  console.log(" 内核模式:    " + MODE);
  if (MODE === "sing-box") console.log(" .srs 镜像:   " + args.mirror + "  (" + MIRROR + "/...)");
  console.log(" 解析到规则:  " + cfg.rules.length + " 条");
  console.log(" 生成分流组:  " + order.length + " 个");
  console.log(" 降级/跳过:   " + degrade.length + " 条");
  if (matchPolicy) console.log(" 漏网之鱼:    " + matchPolicy);
  console.log(" 输出目录:    " + outDir);
  console.log("   - " + confFile + "  (UCI 配置片段，供审阅)");
  console.log("   - " + installFile + "  (路由器端一键安装)");
  console.log("   - " + guideFile + "  (出站映射建议 + 降级报告)");
  if (degrade.length) {
    console.log("");
    console.log(" ⚠ 有 " + degrade.length + " 条规则无法自动转换，见 mapping_guide.txt");
  }
  console.log("");
  console.log(guide);
}

main();
