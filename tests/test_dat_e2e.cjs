const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");


const root = path.resolve(__dirname, "..");
const output = path.join(__dirname, "out_dat_e2e");
fs.rmSync(output, { recursive: true, force: true });

execFileSync(
  "node",
  [
    path.join(root, "clash2passwall.js"),
    path.join(__dirname, "fixtures", "mini_clash.yaml"),
    "--dat",
    "--out",
    output,
    "--no-install",
  ],
  { stdio: "pipe" },
);

const config = fs.readFileSync(
  path.join(output, "passwall2_shunt_rules_dat.conf"),
  "utf8",
);
assert.match(config, /geosite:loyalsoldier-gfw/);
assert.match(config, /geosite:loyalsoldier-proxy/);
assert.doesNotMatch(config, /geolocation-!cn/);
assert.match(config, /geosite:loyalsoldier-reject/);
assert.match(config, /geosite:sukka-ai/);
assert.match(config, /geoip:loyalsoldier-telegramcidr/);
assert.match(config, /geosite:xiaolin-netflix/);
assert.match(config, /geoip:xiaolin-netflix/);
assert.match(config, /geosite:cn/);
assert.match(config, /geoip:cn/);
assert.match(config, /geoip:private/);

console.log("test_dat_e2e: ok");
