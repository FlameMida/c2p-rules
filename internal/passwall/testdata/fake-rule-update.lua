#!/bin/sh
set -eu
grep -q "/releases/download/$FAKE_EXPECTED_TAG/geosite.dat" "$PASSWALL2_CONF"
grep -q "/releases/download/$FAKE_EXPECTED_TAG/geoip.dat" "$PASSWALL2_CONF"
mkdir -p "$PASSWALL2_ASSET_DIR"
printf '%s' "$FAKE_SITE_CONTENT" > "$PASSWALL2_ASSET_DIR/geosite.dat"
printf '%s' "$FAKE_IP_CONTENT" > "$PASSWALL2_ASSET_DIR/geoip.dat"
exit "${FAKE_UPDATER_STATUS:-0}"
