const assert = require("assert");

const {
  DAT_RULESET_MAP,
  applyDatRuleset,
  mapBuiltinGeositeGeoip,
} = require("../map_dat.cjs");


function emptyOutput() {
  return { domain: [], ip: [], policy: null };
}


const expectedMappings = {
  reject: ["domain", "loyalsoldier-reject"],
  icloud: ["domain", "loyalsoldier-icloud"],
  apple: ["domain", "loyalsoldier-apple"],
  google: ["domain", "loyalsoldier-google"],
  proxy: ["domain", "loyalsoldier-proxy"],
  direct: ["domain", "loyalsoldier-direct"],
  private: ["domain", "loyalsoldier-private"],
  gfw: ["domain", "loyalsoldier-gfw"],
  "tld-not-cn": ["domain", "loyalsoldier-tld-not-cn"],
  telegramcidr: ["ip", "loyalsoldier-telegramcidr"],
  cncidr: ["ip", "loyalsoldier-cncidr"],
  lancidr: ["ip", "loyalsoldier-lancidr"],
  YouTube: ["domain", "xiaolin-youtube"],
  Netflix: ["domain", "xiaolin-netflix"],
  Spotify: ["domain", "xiaolin-spotify"],
  BilibiliHMT: ["domain", "xiaolin-bilibili"],
  TikTok: ["domain", "xiaolin-tiktok"],
  AI: ["domain", "sukka-ai"],
};

for (const [provider, [side, tag]] of Object.entries(expectedMappings)) {
  const mapping = DAT_RULESET_MAP[provider];
  assert.ok(mapping, `missing DAT mapping for ${provider}`);
  assert.strictEqual(mapping.side, side);
  assert.strictEqual(mapping.name, tag);
}

{
  const output = emptyOutput();
  applyDatRuleset("gfw", output);
  assert.ok(output.domain.includes("geosite:loyalsoldier-gfw"));
  assert.ok(!output.domain.includes("geosite:gfw"));
}

{
  const output = emptyOutput();
  applyDatRuleset("proxy", output);
  assert.ok(output.domain.includes("geosite:loyalsoldier-proxy"));
  assert.ok(!output.domain.includes("geosite:geolocation-!cn"));
}

{
  const reject = emptyOutput();
  const ai = emptyOutput();
  const telegram = emptyOutput();
  applyDatRuleset("reject", reject);
  applyDatRuleset("AI", ai);
  applyDatRuleset("telegramcidr", telegram);
  assert.ok(reject.domain.includes("geosite:loyalsoldier-reject"));
  assert.ok(ai.domain.includes("geosite:sukka-ai"));
  assert.ok(telegram.ip.includes("geoip:loyalsoldier-telegramcidr"));
}

{
  const output = emptyOutput();
  applyDatRuleset("Netflix", output, {
    hasGeoip: new Set(["xiaolin-netflix"]),
  });
  assert.ok(output.domain.includes("geosite:xiaolin-netflix"));
  assert.ok(output.ip.includes("geoip:xiaolin-netflix"));
}

{
  const output = emptyOutput();
  mapBuiltinGeositeGeoip("GEOSITE", "CN", output);
  mapBuiltinGeositeGeoip("GEOIP", "CN", output);
  mapBuiltinGeositeGeoip("GEOIP", "LAN", output);
  assert.deepStrictEqual(output.domain, ["geosite:cn"]);
  assert.deepStrictEqual(output.ip, ["geoip:cn", "geoip:private"]);
}

assert.strictEqual(DAT_RULESET_MAP.applications, null);
console.log("test_dat_map: ok");
