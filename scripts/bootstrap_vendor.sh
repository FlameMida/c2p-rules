#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENDOR_ROOT="$PROJECT_ROOT/vendor"
BIN_ROOT="$VENDOR_ROOT/bin"
mkdir -p "$VENDOR_ROOT" "$BIN_ROOT"

clone_or_update() {
  local repository="$1"
  local destination="$2"
  if [[ ! -d "$destination/.git" ]]; then
    git clone --depth 1 "$repository" "$destination"
  else
    git -C "$destination" pull --ff-only
  fi
}

clone_or_update \
  https://github.com/v2fly/domain-list-community.git \
  "$VENDOR_ROOT/domain-list-community"
clone_or_update \
  https://github.com/Loyalsoldier/domain-list-custom.git \
  "$VENDOR_ROOT/domain-list-custom"
clone_or_update \
  https://github.com/Loyalsoldier/geoip.git \
  "$VENDOR_ROOT/geoip"
clone_or_update \
  https://github.com/snowie2000/geoview.git \
  "$VENDOR_ROOT/geoview"

(
  cd "$VENDOR_ROOT/geoip"
  go build -o "$BIN_ROOT/geoip" .
)
(
  cd "$VENDOR_ROOT/geoview"
  go build -o "$BIN_ROOT/geoview" .
)

echo "Vendor tools ready. Run:"
echo "export PATH=\"$BIN_ROOT:\$PATH\""
