#!/bin/sh
set -eu
grep -q "/releases/download/$FAKE_EXPECTED_TAG/geosite.dat" "$PASSWALL2_CONF"
grep -q "/releases/download/$FAKE_EXPECTED_TAG/geoip.dat" "$PASSWALL2_CONF"
[ ! -e "$PASSWALL2_RULE_LOCK" ] || exit 0
: > "$PASSWALL2_RULE_LOCK"
geosite_url=$(awk '$1 == "option" && $2 == "geosite_url" { gsub(/\047/, "", $3); print $3; exit }' "$PASSWALL2_CONF")
case "$geosite_url" in
	*gh-proxy.com*) source_name='gh-proxy.com' ;;
	*ghfast.top*) source_name='ghfast.top' ;;
	*github.com*) source_name='github.com' ;;
	*) echo "unexpected geosite URL: $geosite_url" >&2; exit 12 ;;
esac
[ -z "${FAKE_UPDATER_ATTEMPTS:-}" ] || printf '%s\n' "$source_name" >> "$FAKE_UPDATER_ATTEMPTS"
case ",${FAKE_UPDATER_TIMEOUT_SOURCES:-}," in
	*,$source_name,*) exit 124 ;;
esac
sleep "${FAKE_UPDATER_DELAY:-0}"
mkdir -p "$PASSWALL2_ASSET_DIR"
case ",${FAKE_UPDATER_BAD_SHA_SOURCES:-}," in
	*,$source_name,*)
		printf '%s' "bad-site-$source_name" > "$PASSWALL2_ASSET_DIR/geosite.dat"
		printf '%s' "bad-ip-$source_name" > "$PASSWALL2_ASSET_DIR/geoip.dat"
		;;
	*)
		printf '%s' "$FAKE_SITE_CONTENT" > "$PASSWALL2_ASSET_DIR/geosite.dat"
		printf '%s' "$FAKE_IP_CONTENT" > "$PASSWALL2_ASSET_DIR/geoip.dat"
		;;
esac
status=${FAKE_UPDATER_STATUS:-0}
[ "$status" -ne 0 ] || rm -f "$PASSWALL2_RULE_LOCK"
exit "$status"
