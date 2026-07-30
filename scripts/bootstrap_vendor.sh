#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENDOR_ROOT="$PROJECT_ROOT/vendor"
BIN_ROOT="$VENDOR_ROOT/bin"
mkdir -p "$VENDOR_ROOT" "$BIN_ROOT"

sync_rolling_data() {
  local repository="$1"
  local destination="$2"
  if [[ ! -d "$destination/.git" ]]; then
    git clone --depth 1 --filter=blob:none "$repository" "$destination"
  else
    git -C "$destination" remote set-url origin "$repository"
    git -C "$destination" fetch --depth 1 origin HEAD
    git -C "$destination" checkout --detach FETCH_HEAD
  fi
}

checkout_pinned_tool() {
  local repository="$1"
  local destination="$2"
  local commit="$3"
  if [[ ! -d "$destination/.git" ]]; then
    git clone --filter=blob:none --no-checkout "$repository" "$destination"
  else
    git -C "$destination" remote set-url origin "$repository"
  fi
  git -C "$destination" fetch --depth 1 origin "$commit"
  git -C "$destination" checkout --detach "$commit"
  [[ "$(git -C "$destination" rev-parse HEAD)" == "$commit" ]]
}

sync_rolling_data \
  https://github.com/v2fly/domain-list-community.git \
  "$VENDOR_ROOT/domain-list-community"
checkout_pinned_tool \
  https://github.com/Loyalsoldier/domain-list-custom.git \
  "$VENDOR_ROOT/domain-list-custom" \
  efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d
checkout_pinned_tool \
  https://github.com/Loyalsoldier/geoip.git \
  "$VENDOR_ROOT/geoip" \
  85084dfbe282e4e9cb460b07196e6eecfd126d19
checkout_pinned_tool \
  https://github.com/snowie2000/geoview.git \
  "$VENDOR_ROOT/geoview" \
  3c91926d360b8f49d47520639e574608318baf12

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
