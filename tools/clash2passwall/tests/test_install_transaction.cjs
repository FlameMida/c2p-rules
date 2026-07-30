const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const root = path.resolve(__dirname, "..");
const initial = `config global_rules 'global'\n\toption geosite_url 'old-site'\n\toption geoip_url 'old-ip'\n\nconfig nodes 'keep_node'\n\toption remarks 'Keep Me'\n\toption protocol 'vless'\n\nconfig shunt_rules 'old_rule'\n\toption remarks 'Old Rule'\n\toption domain_list 'full:old.example'\n`;

function prepare() {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "clash2passwall-transaction-"));
  const output = path.join(directory, "output");
  const config = path.join(directory, "passwall2");
  const bin = path.join(directory, "bin");
  fs.mkdirSync(bin);
  fs.writeFileSync(config, initial, "utf8");
  fs.copyFileSync(path.join(__dirname, "helpers", "fake_uci.cjs"), path.join(bin, "uci"));
  fs.chmodSync(path.join(bin, "uci"), 0o755);
  fs.writeFileSync(
    path.join(bin, "base64"),
    "#!/bin/sh\nif [ \"${FAKE_BASE64_FAIL:-}\" = 1 ]; then exit 9; fi\nexec /usr/bin/base64 \"$@\"\n",
    { mode: 0o755 },
  );
  execFileSync("node", [
    path.join(root, "clash2passwall.js"),
    path.join(__dirname, "fixtures", "mini_clash.yaml"),
    "--dat",
    "--tag-manifest", path.join(__dirname, "fixtures", "tags_full.json"),
    "--repo", "ExampleOwner/clash-rules-srs",
    "--out", output,
  ]);
  return {
    config,
    script: path.join(output, "install_shunt_rules_dat.sh"),
    env: {
      ...process.env,
      PATH: bin + path.delimiter + process.env.PATH,
      PASSWALL2_CONF: config,
      FAKE_UCI_LIVE_CONF: config,
      TMPDIR: directory,
    },
  };
}

{
  const fixture = prepare();
  execFileSync("sh", [fixture.script], { env: fixture.env, stdio: "pipe" });
  const installed = fs.readFileSync(fixture.config, "utf8");
  assert.match(installed, /config nodes 'keep_node'[\s\S]*option remarks 'Keep Me'/);
  assert.doesNotMatch(installed, /config shunt_rules 'old_rule'/);
  assert.match(installed, /config shunt_rules 'Proxy'/);
  assert.match(installed, /option geosite_url 'https:\/\/github\.com\/ExampleOwner\/clash-rules-srs\/releases\/latest\/download\/geosite\.dat'/);
  assert.match(installed, /option geoip_url 'https:\/\/github\.com\/ExampleOwner\/clash-rules-srs\/releases\/latest\/download\/geoip\.dat'/);
}

for (const injected of ["FAKE_UCI_FAIL_STAGE_COMMIT", "FAKE_BASE64_FAIL", "FAKE_UCI_FAIL_LIVE_COMMIT"]) {
  const fixture = prepare();
  assert.throws(
    () => execFileSync("sh", [fixture.script], { env: { ...fixture.env, [injected]: "1" }, stdio: "pipe" }),
    (error) => Number.isInteger(error.status) && error.status !== 0,
  );
  assert.strictEqual(fs.readFileSync(fixture.config, "utf8"), initial, `${injected} must roll back`);
}

console.log("test_install_transaction: ok");
