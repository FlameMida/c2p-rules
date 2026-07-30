const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");


const root = path.resolve(__dirname, "..");
const output = path.join(__dirname, "out_dat_cli");
fs.rmSync(output, { recursive: true, force: true });

execFileSync(
  "node",
  [
    path.join(root, "clash2passwall.js"),
    path.join(__dirname, "fixtures", "dat_mode.yaml"),
    "--dat",
    "--out",
    output,
    "--no-install",
  ],
  { stdio: "pipe" },
);

const configPath = path.join(output, "passwall2_shunt_rules_dat.conf");
assert.ok(fs.existsSync(configPath), "dat mode should use a distinct output suffix");
const config = fs.readFileSync(configPath, "utf8");
assert.match(config, /geosite:loyalsoldier-gfw/);
assert.doesNotMatch(config, /geosite:gfw/);
assert.match(config, /geosite:xiaolin-netflix/);
assert.match(config, /geoip:xiaolin-netflix/);
assert.match(config, /geosite:cn/);
assert.match(config, /geoip:cn/);
assert.match(config, /geoip:private/);

console.log("test_dat_cli: ok");
