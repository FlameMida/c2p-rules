const assert = require("assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const root = path.resolve(__dirname, "..");
const output = fs.mkdtempSync(path.join(os.tmpdir(), "clash2passwall-security-"));

assert.throws(
  () => execFileSync(
    "node",
    [
      path.join(root, "clash2passwall.js"),
      path.join(__dirname, "fixtures", "malicious_control.yaml"),
      "--dat",
      "--tag-manifest",
      path.join(__dirname, "fixtures", "tags_full.json"),
      "--repo",
      "ExampleOwner/clash-rules-srs",
      "--out",
      output,
    ],
    { stdio: "pipe" },
  ),
  /control|newline|status/i,
);

for (const file of fs.readdirSync(output)) {
  const text = fs.readFileSync(path.join(output, file), "utf8");
  assert.doesNotMatch(text, /id >\/tmp\/pwned/);
}

const c1Input = path.join(output, "malicious_c1.yaml");
fs.writeFileSync(
  c1Input,
  "rules:\n  - DOMAIN,example.com,Group\u0085Name\nproxy-groups: []\nrule-providers: {}\n",
  "utf8",
);
assert.throws(
  () => execFileSync(
    "node",
    [
      path.join(root, "clash2passwall.js"),
      c1Input,
      "--dat",
      "--tag-manifest",
      path.join(__dirname, "fixtures", "tags_full.json"),
      "--out",
      output,
      "--no-install",
    ],
    { stdio: "pipe" },
  ),
  /control|newline|status/i,
);

console.log("test_security: ok");
