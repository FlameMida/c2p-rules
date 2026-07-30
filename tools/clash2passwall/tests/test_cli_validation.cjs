const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const root = path.resolve(__dirname, "..");
const directory = fs.mkdtempSync(path.join(os.tmpdir(), "clash2passwall-validation-"));
const manifest = path.join(__dirname, "fixtures", "tags_full.json");

function convert(yaml, extra = []) {
  const input = path.join(directory, `input-${Math.random()}.yaml`);
  const output = path.join(directory, `output-${Math.random()}`);
  fs.writeFileSync(input, yaml, "utf8");
  execFileSync("node", [
    path.join(root, "clash2passwall.js"), input,
    "--dat", "--tag-manifest", manifest,
    "--out", output, "--no-install",
    ...extra,
  ], { stdio: "pipe" });
  return fs.readFileSync(path.join(output, "passwall2_shunt_rules_dat.conf"), "utf8");
}

const config = convert(`rules:\n  - DOMAIN,cn.example,中文分组\n  - DOMAIN,a.example,A-B\n  - DOMAIN,b.example,A B\n  - DOMAIN,c.example,${"Long".repeat(40)}\n  - IP-SUFFIX,8.8.8.0/24,ShouldSkip\nproxy-groups: []\nrule-providers: {}\n`);
assert.match(config, /option remarks '中文分组'/);
assert.match(config, /config shunt_rules 'c2p_rule_[0-9a-f]{10}'/);
assert.match(config, /config shunt_rules 'c2p_A_B_[0-9a-f]{10}'/);
assert.doesNotMatch(config, /ShouldSkip|8\.8\.8\.0\/24/);
const ids = [...config.matchAll(/^config shunt_rules '([^']+)'/gm)].map((match) => match[1]);
assert.strictEqual(new Set(ids).size, ids.length);
assert.ok(ids.every((id) => id.length <= 64), "UCI section IDs must stay bounded");
assert.ok(ids.every((id) => id.startsWith("c2p_")), "generated IDs need a dedicated namespace");

function policyIds(text) {
  const result = {};
  for (const block of text.split(/\n\n+/)) {
    const id = block.match(/^config shunt_rules '([^']+)'/);
    const remarks = block.match(/option remarks '([^']+)'/);
    if (id && remarks) result[remarks[1]] = id[1];
  }
  return result;
}
const reordered = convert(`rules:\n  - DOMAIN,b.example,A B\n  - DOMAIN,a.example,A-B\nproxy-groups: []\nrule-providers: {}\n`);
const originalIds = policyIds(config);
const reorderedIds = policyIds(reordered);
assert.strictEqual(reorderedIds["A-B"], originalIds["A-B"]);
assert.strictEqual(reorderedIds["A B"], originalIds["A B"]);

const prototypeNames = convert(`rules:\n  - DOMAIN,proto.example,__proto__\n  - DOMAIN,constructor.example,constructor\n  - RULE-SET,__proto__,Proxy\nproxy-groups: []\nrule-providers: {}\n`);
assert.match(prototypeNames, /option remarks '__proto__'/);
assert.match(prototypeNames, /option remarks 'constructor'/);
assert.doesNotMatch(prototypeNames, /geosite:\[object Object\]/);

for (const yaml of [
  "rules: DOMAIN,example.com,Proxy\n",
  "rules: []\nproxy-groups: nope\n",
  "rules: []\nrule-providers: []\n",
  "rules:\n  - 123\n",
]) {
  assert.throws(() => convert(yaml, ["--yaml-engine", "js-yaml"]), (error) => error.status !== 0);
}

const fixture = path.join(__dirname, "fixtures", "mini_clash.yaml");
const outputs = ["builtin", "js-yaml"].map((engine) => {
  const output = path.join(directory, engine);
  execFileSync("node", [
    path.join(root, "clash2passwall.js"), fixture,
    "--dat", "--tag-manifest", manifest,
    "--yaml-engine", engine,
    "--out", output, "--no-install",
  ]);
  return fs.readFileSync(path.join(output, "passwall2_shunt_rules_dat.conf"), "utf8");
});
assert.strictEqual(outputs[0], outputs[1]);

console.log("test_cli_validation: ok");
