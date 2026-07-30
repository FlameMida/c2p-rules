#!/usr/bin/env node
const fs = require("fs");

const [configPath, manifestPath] = process.argv.slice(2);
if (!configPath || !manifestPath) {
  console.error("usage: node verify_manifest_refs.cjs <passwall2.conf> <expected_tags.json>");
  process.exit(2);
}

const config = fs.readFileSync(configPath, "utf8");
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const required = manifest.required || manifest;
const available = {
  geosite: new Set(required.geosite || []),
  geoip: new Set(required.geoip || []),
};
const missing = [];
for (const match of config.matchAll(/\b(geosite|geoip):([A-Za-z0-9._-]+)/g)) {
  if (!available[match[1]].has(match[2])) missing.push(match[0]);
}
if (missing.length) {
  console.error("references missing from tag manifest: " + [...new Set(missing)].sort().join(", "));
  process.exit(1);
}
console.log("all geosite/geoip references exist in tag manifest");
