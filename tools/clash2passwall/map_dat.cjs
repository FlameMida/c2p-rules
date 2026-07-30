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

const DAT_GEOIP_TAGS = new Set(["xiaolin-netflix", "xiaolin-bilibili"]);

function applyDatRuleset(name, output, options = {}) {
  const mapping = DAT_RULESET_MAP[name];
  if (mapping === null) return true;
  if (!mapping) return false;

  if (mapping.side === "domain") {
    output.domain.push("geosite:" + mapping.name);
    const hasGeoip = options.hasGeoip || DAT_GEOIP_TAGS;
    if (mapping.alsoIp && hasGeoip.has(mapping.name)) {
      output.ip.push("geoip:" + mapping.name);
    }
  } else {
    output.ip.push("geoip:" + mapping.name);
  }
  return true;
}

function mapBuiltinGeositeGeoip(type, value, output) {
  if (type === "GEOSITE") {
    output.domain.push("geosite:" + value.toLowerCase());
  } else if (type === "GEOIP") {
    const normalized = value.toUpperCase() === "LAN" ? "private" : value.toLowerCase();
    output.ip.push("geoip:" + normalized);
  }
}

module.exports = {
  DAT_GEOIP_TAGS,
  DAT_RULESET_MAP,
  applyDatRuleset,
  mapBuiltinGeositeGeoip,
};
