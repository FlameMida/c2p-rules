const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");


const root = path.resolve(__dirname, "..");
const output = path.join(__dirname, "out_install");
fs.rmSync(output, { recursive: true, force: true });

execFileSync(
  "node",
  [
    path.join(root, "clash2passwall.js"),
    path.join(__dirname, "fixtures", "mini_clash.yaml"),
    "--dat",
    "--out",
    output,
  ],
  { stdio: "pipe" },
);

const script = fs.readFileSync(
  path.join(output, "install_shunt_rules_dat.sh"),
  "utf8",
);
assert.match(script, /OWNER="\$\{OWNER:-YOUR_GITHUB_USER\}"/);
assert.match(script, /REPO="\$\{REPO:-clash-rules-srs\}"/);
assert.match(script, /releases\/latest\/download\/geosite\.dat/);
assert.match(script, /releases\/latest\/download\/geoip\.dat/);
assert.match(script, /delete passwall2\.@shunt_rules\[0\]/);
assert.match(
  script,
  /while uci -q delete passwall2\.@shunt_rules\[0\]; do :; done\nuci commit passwall2\n\ncat >>/,
);
assert.doesNotMatch(script, /delete passwall2\.@nodes/);
assert.match(script, /geoview/);
assert.match(script, /0\.1\.10/);
assert.match(script, /geosite:loyalsoldier-gfw/);

console.log("test_install_script: ok");
