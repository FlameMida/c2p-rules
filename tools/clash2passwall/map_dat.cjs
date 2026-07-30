const DAT_RULESET_MAP = {
  reject: { name: "loyalsoldier-reject", side: "domain" },
  icloud: { name: "loyalsoldier-icloud", side: "domain" },
  apple: { name: "loyalsoldier-apple", side: "domain" },
  google: { name: "loyalsoldier-google", side: "domain" },
  proxy: { name: "loyalsoldier-proxy", side: "domain" },
  direct: { name: "loyalsoldier-direct", side: "domain" },
  private: { name: "loyalsoldier-private", side: "domain" },
  gfw: { name: "loyalsoldier-gfw", side: "domain" },
  "tld-not-cn": { name: "loyalsoldier-tld-not-cn", side: "domain" },
  telegramcidr: { name: "loyalsoldier-telegramcidr", side: "ip" },
  cncidr: { name: "loyalsoldier-cncidr", side: "ip" },
  lancidr: { name: "loyalsoldier-lancidr", side: "ip" },
  YouTube: { name: "xiaolin-youtube", side: "domain" },
  youtube: { name: "xiaolin-youtube", side: "domain" },
  Netflix: { name: "xiaolin-netflix", side: "domain", alsoIp: true },
  netflix: { name: "xiaolin-netflix", side: "domain", alsoIp: true },
  Spotify: { name: "xiaolin-spotify", side: "domain" },
  spotify: { name: "xiaolin-spotify", side: "domain" },
  BilibiliHMT: { name: "xiaolin-bilibili", side: "domain", alsoIp: true },
  Bilibili: { name: "xiaolin-bilibili", side: "domain", alsoIp: true },
  bilibili: { name: "xiaolin-bilibili", side: "domain", alsoIp: true },
  TikTok: { name: "xiaolin-tiktok", side: "domain" },
  tiktok: { name: "xiaolin-tiktok", side: "domain" },
  AI: { name: "sukka-ai", side: "domain" },
  ai: { name: "sukka-ai", side: "domain" },
  applications: null,
};

function applyDatRuleset(name, output, options = {}) {
  if (!Object.hasOwn(DAT_RULESET_MAP, name)) return false;
  const mapping = DAT_RULESET_MAP[name];
  if (mapping === null) return true;
  if (!mapping) return false;
  const availableTags = options.availableTags;
  if (!availableTags || !(availableTags.geosite instanceof Set) || !(availableTags.geoip instanceof Set)) {
    throw new Error("available geosite/geoip tag manifest is required");
  }

  if (mapping.side === "domain") {
    if (!availableTags.geosite.has(mapping.name)) return false;
    output.domain.push("geosite:" + mapping.name);
    if (mapping.alsoIp && availableTags.geoip.has(mapping.name)) {
      output.ip.push("geoip:" + mapping.name);
    }
  } else {
    if (!availableTags.geoip.has(mapping.name)) return false;
    output.ip.push("geoip:" + mapping.name);
  }
  return true;
}

function mapBuiltinGeositeGeoip(type, value, output, availableTags) {
  if (!availableTags || !(availableTags.geosite instanceof Set) || !(availableTags.geoip instanceof Set)) {
    throw new Error("available geosite/geoip tag manifest is required");
  }
  if (type === "GEOSITE") {
    const normalized = value.toLowerCase();
    if (!availableTags.geosite.has(normalized)) return false;
    output.domain.push("geosite:" + normalized);
  } else if (type === "GEOIP") {
    const normalized = value.toUpperCase() === "LAN" ? "private" : value.toLowerCase();
    if (!availableTags.geoip.has(normalized)) return false;
    output.ip.push("geoip:" + normalized);
  }
  return true;
}

module.exports = {
  DAT_RULESET_MAP,
  applyDatRuleset,
  mapBuiltinGeositeGeoip,
};
