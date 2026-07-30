const assert = require("assert");
const { execFileSync, spawnSync } = require("child_process");
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
    "--tag-manifest",
    path.join(__dirname, "fixtures", "tags_full.json"),
    "--repo",
    "ExampleOwner/clash-rules-srs",
    "--out",
    output,
  ],
  { stdio: "pipe" },
);

const script = fs.readFileSync(
  path.join(output, "install_shunt_rules_dat.sh"),
  "utf8",
);
assert.doesNotMatch(script, /YOUR_GITHUB_USER/);
assert.match(script, /ExampleOwner\/clash-rules-srs/);
assert.match(script, /releases\/latest\/download\/geosite\.dat/);
assert.match(script, /releases\/latest\/download\/geoip\.dat/);
assert.match(script, /delete passwall2\.@shunt_rules\[0\]/);
assert.doesNotMatch(script, /<<'PWEOF'/);
assert.doesNotMatch(script, /cat >> "\$CONF"/);
assert.doesNotMatch(script, /delete passwall2\.@nodes/);
assert.match(script, /geoview/);
assert.match(script, /0\.1\.10/);
const encoded = script.match(/printf '%s' '([A-Za-z0-9+/=]+)' \| base64 -d/);
assert.ok(encoded, "installer should embed only base64-encoded UCI data");
assert.match(Buffer.from(encoded[1], "base64").toString("utf8"), /geosite:loyalsoldier-gfw/);
execFileSync("sh", ["-n", path.join(output, "install_shunt_rules_dat.sh")]);

const noRepoOutput = path.join(__dirname, "out_install_no_repo");
fs.rmSync(noRepoOutput, { recursive: true, force: true });
execFileSync(
  "node",
  [
    path.join(root, "clash2passwall.js"),
    path.join(__dirname, "fixtures", "mini_clash.yaml"),
    "--dat",
    "--tag-manifest",
    path.join(__dirname, "fixtures", "tags_full.json"),
    "--out",
    noRepoOutput,
  ],
  { stdio: "pipe" },
);
const noRepoScript = path.join(noRepoOutput, "install_shunt_rules_dat.sh");
assert.doesNotMatch(fs.readFileSync(noRepoScript, "utf8"), /YOUR_GITHUB_USER/);
assert.throws(
  () => execFileSync("sh", [noRepoScript], { stdio: "pipe", env: { PATH: process.env.PATH } }),
  /REPO_SLUG|OWNER.*REPO|status/i,
);
const invalidRepo = spawnSync("sh", [noRepoScript], {
  encoding: "utf8",
  env: { PATH: process.env.PATH, REPO_SLUG: "owner/repo;touch-pwned" },
});
assert.notStrictEqual(invalidRepo.status, 0);
assert.match(invalidRepo.stderr, /set REPO_SLUG=owner\/repo/);
assert.doesNotMatch(invalidRepo.stderr, /config not found/);

console.log("test_install_script: ok");
