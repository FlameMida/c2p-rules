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
const {
  DAT_RULESET_MAP,
  applyDatRuleset,
  mapBuiltinGeositeGeoip,
} = require("./map_dat.cjs");

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
let MODE = "sing-box"; // "sing-box" | "xray" | "dat"
let AVAILABLE_TAGS = null;
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

function assertSafeScalar(value, label) {
  if (typeof value !== "string") throw new Error(`${label} must be a string`);
  if (/[\u0000-\u001f\u007f\u2028\u2029]/u.test(value)) {
    throw new Error(`${label} contains a control character or newline`);
  }
  return value;
}

function validateConfig(cfg) {
  if (!cfg || typeof cfg !== "object" || Array.isArray(cfg)) {
    throw new Error("YAML root must be a mapping");
  }
  if (cfg.rules != null && !Array.isArray(cfg.rules)) throw new Error("rules must be a list");
  if (cfg["proxy-groups"] != null && !Array.isArray(cfg["proxy-groups"])) {
    throw new Error("proxy-groups must be a list");
  }
  const providers = cfg["rule-providers"];
  if (providers != null && (typeof providers !== "object" || Array.isArray(providers))) {
    throw new Error("rule-providers must be a mapping");
  }
  for (const [index, rule] of (cfg.rules || []).entries()) assertSafeScalar(rule, `rules[${index}]`);
  for (const [index, group] of (cfg["proxy-groups"] || []).entries()) {
    if (!group || typeof group !== "object" || Array.isArray(group)) {
      throw new Error(`proxy-groups[${index}] must be a mapping`);
    }
    if (group.name != null) assertSafeScalar(group.name, `proxy-groups[${index}].name`);
    if (group.type != null) assertSafeScalar(group.type, `proxy-groups[${index}].type`);
  }
  for (const [name, provider] of Object.entries(providers || {})) {
    assertSafeScalar(name, "rule-provider name");
    if (!provider || typeof provider !== "object" || Array.isArray(provider)) {
      throw new Error(`rule-provider ${name} must be a mapping`);
    }
    for (const field of ["type", "behavior", "url"]) {
      if (provider[field] != null) assertSafeScalar(provider[field], `rule-provider ${name}.${field}`);
    }
  }
}

function loadConfig(file, yamlEngine) {
  const text = fs.readFileSync(file, "utf8");
  if (yamlEngine === "js-yaml" && !jsyaml) throw new Error("js-yaml engine requested but js-yaml is not installed");
  const useJsYaml = yamlEngine === "js-yaml" || (yamlEngine === "auto" && jsyaml);
  const cfg = useJsYaml ? jsyaml.load(text) : builtinParse(text);
  validateConfig(cfg);
  cfg.rules = cfg.rules || [];
  cfg["rule-providers"] = cfg["rule-providers"] || {};
  cfg["proxy-groups"] = cfg["proxy-groups"] || [];
  return cfg;
}

function loadTagManifest(file) {
  const manifest = JSON.parse(fs.readFileSync(file, "utf8"));
  const required = manifest.required || manifest;
  const tags = {};
  for (const side of ["geosite", "geoip"]) {
    if (!Array.isArray(required[side]) || !required[side].every((tag) => typeof tag === "string" && /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(tag))) {
      throw new Error(`tag manifest ${side} must be a list of valid tag names`);
    }
    tags[side] = new Set(required[side]);
  }
  return tags;
}

// ============================================================
// 规则映射
// ============================================================
function applyRuleset(name, out, providers, degrade) {
  if (MODE === "dat") {
    if (DAT_RULESET_MAP[name] === null) {
      degrade.push({ line: "RULE-SET," + name, reason: "进程类规则(applications)，路由器场景无意义，已跳过" });
      return;
    }
    if (applyDatRuleset(name, out, { availableTags: AVAILABLE_TAGS })) return;
    degrade.push({
      line: "RULE-SET," + name,
      reason: "dat 模式没有该 provider 到自建 geodata tag 的固定映射，已跳过",
    });
    return;
  }
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
    case "GEOSITE":
      if (MODE === "dat") {
        if (!mapBuiltinGeositeGeoip(type, parts[1], out, AVAILABLE_TAGS)) {
          degrade.push({ line: rawLine, reason: `tag manifest 缺少 geosite:${parts[1].toLowerCase()}，已跳过` });
        }
      }
      else out.domain.push("geosite:" + parts[1]);
      setPol(2);
      break;
    case "GEOIP":
      if (MODE === "dat") {
        const normalized = parts[1].toUpperCase() === "LAN" ? "private" : parts[1].toLowerCase();
        if (!mapBuiltinGeositeGeoip(type, parts[1], out, AVAILABLE_TAGS)) {
          degrade.push({ line: rawLine, reason: `tag manifest 缺少 geoip:${normalized}，已跳过` });
        }
      }
      else out.ip.push("geoip:" + parts[1]);
      setPol(2);
      break;
    case "IP-CIDR":
    case "IP-CIDR6":       out.ip.push(parts[1]);                   setPol(2); break;
    case "IP-SUFFIX":
      degrade.push({ line: rawLine, reason: "PassWall2 ip_list 不支持 IP-SUFFIX 的无损语义，已跳过" });
      return null;
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

function uciQuote(value) {
  return "'" + value.replace(/'/g, "'\\''") + "'";
}

function makeSectionIds(order) {
  const used = new Set();
  return Object.fromEntries(order.map((policy) => {
    let base = policy.normalize("NFKD").replace(/[^A-Za-z0-9_]+/g, "_").replace(/^_+|_+$/g, "");
    if (!base || /^[0-9]/.test(base)) base = "rule_" + base;
    if (!base || base === "rule_") {
      const digest = require("crypto").createHash("sha256").update(policy).digest("hex").slice(0, 10);
      base = "rule_" + digest;
    }
    if (base.length > 48) {
      const digest = require("crypto").createHash("sha256").update(policy).digest("hex").slice(0, 10);
      base = `${base.slice(0, 48)}_${digest}`;
    }
    let candidate = base;
    if (used.has(candidate)) {
      const digest = require("crypto").createHash("sha256").update(policy).digest("hex").slice(0, 8);
      candidate = `${base}_${digest}`;
    }
    let serial = 2;
    while (used.has(candidate)) candidate = `${base}_${serial++}`;
    used.add(candidate);
    return [policy, candidate];
  }));
}

function generateConf(order, pr) {
  let out = "";
  const sectionIds = makeSectionIds(order);
  for (const policy of order) {
    const r = pr[policy];
    out += "config shunt_rules " + uciQuote(sectionIds[policy]) + "\n";
    out += "\toption remarks " + uciQuote(policy) + "\n";
    out += "\toption network 'tcp,udp'\n";
    if (r.domain.length) out += "\toption domain_list " + uciMultiValue(r.domain) + "\n";
    if (r.ip.length)     out += "\toption ip_list " + uciMultiValue(r.ip) + "\n";
    if (r.port.length)   out += "\toption port '" + r.port.join(",") + "'\n";
    if (r.source.length) out += "\toption source '" + r.source.join(" ") + "'\n";
    out += "\n";
  }
  return out;
}

function generateInstall(conf, order, mode, repoSlug) {
  const encodedConf = Buffer.from(conf, "utf8").toString("base64");
  const defaultRepo = mode === "dat" ? (repoSlug || "") : "";
  const datSetup = mode === "dat" ? `
REPO_SLUG="\${REPO_SLUG:-${defaultRepo}}"
if [ -z "$REPO_SLUG" ] && [ -n "\${OWNER:-}" ] && [ -n "\${REPO:-}" ]; then
  REPO_SLUG="$OWNER/$REPO"
fi
if ! printf '%s\n' "$REPO_SLUG" | grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$'; then
  echo "ERROR: set REPO_SLUG=owner/repo (or regenerate with --repo owner/repo)" >&2
  exit 2
fi
` : "";
  const datCommands = mode === "dat" ? `
uci -c "$STAGE_ROOT" -q set "passwall2.@global_rules[0].geosite_url=https://github.com/$REPO_SLUG/releases/latest/download/geosite.dat"
uci -c "$STAGE_ROOT" -q set "passwall2.@global_rules[0].geoip_url=https://github.com/$REPO_SLUG/releases/latest/download/geoip.dat"
` : "";
  const datVerify = mode === "dat" ? `
uci -q get passwall2.@global_rules[0].geosite_url >/dev/null
uci -q get passwall2.@global_rules[0].geoip_url >/dev/null
` : "";

  return `#!/bin/sh
# Generated PassWall2 shunt installer. Stages and validates before atomic replacement.
set -eu
CONF="\${PASSWALL2_CONF:-/etc/config/passwall2}"
${datSetup}
if [ ! -f "$CONF" ]; then
  echo "ERROR: PassWall2 config not found: $CONF" >&2
  exit 2
fi
if [ -n "$(uci changes passwall2 2>/dev/null || true)" ]; then
  echo "ERROR: passwall2 has uncommitted UCI changes; commit or revert them first" >&2
  exit 2
fi

TS=$(date +%s 2>/dev/null || echo bak)
BACKUP="$CONF.bak.$TS"
STAGE_ROOT=$(mktemp -d "\${TMPDIR:-/tmp}/clash2passwall.XXXXXX")
STAGED_CONF="$STAGE_ROOT/passwall2"
LIVE_STAGE="$CONF.new.$$"
SUCCESS=0

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ "$SUCCESS" -ne 1 ] && [ -f "$BACKUP" ]; then
    cp "$BACKUP" "$CONF" || true
    uci -q commit passwall2 >/dev/null 2>&1 || true
  fi
  [ ! -e "$LIVE_STAGE" ] || rm -f "$LIVE_STAGE"
  [ ! -d "$STAGE_ROOT" ] || rm -rf "$STAGE_ROOT"
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

cp "$CONF" "$BACKUP"
cp "$CONF" "$STAGED_CONF"
${datCommands}
while uci -c "$STAGE_ROOT" -q delete passwall2.@shunt_rules[0]; do :; done
uci -c "$STAGE_ROOT" -q commit passwall2
printf '%s' '${encodedConf}' | base64 -d >> "$STAGED_CONF"
uci -c "$STAGE_ROOT" -q show passwall2 >/dev/null

cp "$STAGED_CONF" "$LIVE_STAGE"
chmod 600 "$LIVE_STAGE"
mv "$LIVE_STAGE" "$CONF"
uci -q commit passwall2
${datVerify}
SUCCESS=1
trap - EXIT HUP INT TERM
rm -rf "$STAGE_ROOT"

echo "✅ 已事务性覆盖导入 ${order.length} 条 shunt_rules；其他 section 保持不变。"
${mode === "dat" ? 'echo "NOTE: sing-box kernel requires geoview >= 0.1.10"' : ""}
echo "备份: $BACKUP"
`;
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
  const a = { positional: [], mirror: "gh-proxy", out: null, noInstall: false, xray: false, dat: false, tagManifest: null, repo: null, yamlEngine: "auto" };
  const takeValue = (flag, index) => {
    if (index + 1 >= argv.length || argv[index + 1].startsWith("--")) throw new Error(`${flag} requires a value`);
    return argv[index + 1];
  };
  for (let i = 0; i < argv.length; i++) {
    const t = argv[i];
    if (t === "--mirror") a.mirror = takeValue(t, i++);
    else if (t === "--out") a.out = takeValue(t, i++);
    else if (t === "--tag-manifest") a.tagManifest = takeValue(t, i++);
    else if (t === "--repo") a.repo = takeValue(t, i++);
    else if (t === "--yaml-engine") a.yamlEngine = takeValue(t, i++);
    else if (t === "--no-install") a.noInstall = true;
    else if (t === "--xray") a.xray = true;
    else if (t === "--dat") a.dat = true;
    else if (t === "-h" || t === "--help") a.help = true;
    else if (t.startsWith("-")) throw new Error(`unknown option: ${t}`);
    else a.positional.push(t);
  }
  return a;
}

function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (error) {
    console.error("ERROR: " + error.message);
    process.exit(1);
  }
  if (args.help || !args.positional.length) {
    console.error("用法: node clash2passwall.js <clash.yaml> [--mirror name] [--out dir] [--xray|--dat --tag-manifest expected_tags.json] [--repo owner/repo] [--yaml-engine auto|builtin|js-yaml] [--no-install]");
    process.exit(args.help ? 0 : 1);
  }

  if (args.xray && args.dat) {
    console.error("--xray 与 --dat 不能同时使用");
    process.exit(1);
  }
  if (!["auto", "builtin", "js-yaml"].includes(args.yamlEngine)) {
    console.error("未知 YAML 引擎: " + args.yamlEngine);
    process.exit(1);
  }
  if (args.repo && !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(args.repo)) {
    console.error("--repo 必须是 owner/repo");
    process.exit(1);
  }
  if (args.dat && !args.tagManifest) {
    console.error("--dat 必须提供 --tag-manifest <expected_tags.json>");
    process.exit(1);
  }

  const inputFile = args.positional[0];
  if (!MIRRORS[args.mirror]) {
    console.error("未知镜像: " + args.mirror + "，可选: " + Object.keys(MIRRORS).join("/"));
    process.exit(1);
  }
  MIRROR = MIRRORS[args.mirror];
  MODE = args.dat ? "dat" : (args.xray ? "xray" : "sing-box");

  let cfg;
  try {
    if (args.dat) AVAILABLE_TAGS = loadTagManifest(args.tagManifest);
    cfg = loadConfig(inputFile, args.yamlEngine);
  } catch (error) {
    console.error("ERROR: " + error.message);
    process.exit(1);
  }
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
    if (![...mapped.domain, ...mapped.ip, ...mapped.port, ...mapped.source].length) continue;
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
  const suffix = MODE === "xray" ? "_xray" : (MODE === "dat" ? "_dat" : "");
  const confFile = "passwall2_shunt_rules" + suffix + ".conf";
  const installFile = "install_shunt_rules" + suffix + ".sh";
  const guideFile = "mapping_guide" + suffix + ".txt";
  fs.writeFileSync(path.join(outDir, confFile), conf, "utf8");
  fs.writeFileSync(path.join(outDir, guideFile), guide, "utf8");
  if (!args.noInstall) {
    fs.writeFileSync(path.join(outDir, installFile), generateInstall(conf, order, MODE, args.repo), { encoding: "utf8", mode: 0o700 });
  }

  // 终端摘要
  console.log("============================================================");
  const modeDetail = MODE === "sing-box"
    ? "：rule-set → .srs 订阅"
    : (MODE === "dat" ? "：rule-set → 自建 geodata tag" : "：rule-set → 标准 geosite:/geoip:");
  console.log(" Clash → PassWall2 转换完成  [" + MODE + " 模式" + modeDetail + "]");
  console.log("============================================================");
  console.log(" 输入:        " + inputFile);
  const selectedEngine = args.yamlEngine === "auto" ? (jsyaml ? "js-yaml" : "builtin") : args.yamlEngine;
  console.log(" YAML 引擎:   " + selectedEngine);
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
