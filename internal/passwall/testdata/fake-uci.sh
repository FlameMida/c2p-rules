#!/bin/sh
set -eu

config_dir=''
save_dir=''
no_commit=0
while [ "$#" -gt 0 ]; do
	case "$1" in
		-c) config_dir=$2; shift 2 ;;
		-P) save_dir=$2; no_commit=1; shift 2 ;;
		-t) save_dir=$2; shift 2 ;;
		-q) shift ;;
		*) break ;;
	esac
done

if [ -n "$config_dir" ] && [ "${FAKE_UCI_REQUIRE_ISOLATED_SAVEDIR:-false}" = true ] && [ -z "$save_dir" ]; then
	echo 'staging UCI did not isolate savedir' >&2
	exit 12
fi

command=$1
shift
if [ -n "$config_dir" ]; then
	conf="$config_dir/passwall2"
else
	conf=$PASSWALL2_CONF
fi
if [ -z "$save_dir" ]; then
	save_dir=${FAKE_UCI_LIVE_SAVEDIR:?}
fi
mkdir -p "$save_dir"
delta="$save_dir/passwall2.delta"

apply_delta() {
	[ ! -f "$delta" ] || while IFS= read -r assignment; do
		left=${assignment%%=*}
		value=${assignment#*=}
		name=${left##*.}
		temporary="$conf.fake.$$"
		awk -v name="$name" -v value="$value" '
			$1 == "option" && $2 == name { print "\toption " name " \047" value "\047"; found=1; next }
			{ print }
			END { if (!found) exit 42 }
		' "$conf" > "$temporary" || { status=$?; rm -f "$temporary"; exit "$status"; }
		mv "$temporary" "$conf"
	done < "$delta"
	rm -f "$delta"
}

case "$command" in
	changes)
		[ -z "${FAKE_UCI_CHANGES:-}" ] || printf '%s\n' "$FAKE_UCI_CHANGES"
		;;
	get)
		case "$1" in
			passwall2.@global_rules\[0\].v2ray_location_asset) printf '%s\n' "$PASSWALL2_ASSET_DIR" ;;
			*) exit 1 ;;
		esac
		;;
	set)
		assignment=$1
		printf '%s\n' "$assignment" >> "$delta"
		;;
	show)
		[ -f "$conf" ]
		;;
	commit)
		counter=${FAKE_UCI_COUNTER:-/tmp/fake-uci-counter}
		count=0
		[ ! -f "$counter" ] || count=$(cat "$counter")
		count=$((count + 1))
		printf '%s' "$count" > "$counter"
		[ "$count" -ne "${FAKE_UCI_FAIL_COMMIT:-0}" ] || exit 9
		[ "$no_commit" -eq 1 ] || apply_delta
		;;
	revert)
		rm -f "$delta"
		[ -z "${FAKE_UCI_REVERT_MARKER:-}" ] || : > "$FAKE_UCI_REVERT_MARKER"
		;;
	*)
		exit 2
		;;
esac
