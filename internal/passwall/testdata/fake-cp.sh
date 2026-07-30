#!/bin/sh
set -eu

case "${1:-}" in
	*"${FAKE_CP_FAIL_SOURCE_SUFFIX:-__never_match__}")
		[ -z "${FAKE_CP_FAIL_SOURCE_SUFFIX:-}" ] || exit 77
		;;
esac
case "${1:-}" in
	*.bak.*)
		if [ "${FAKE_CP_REQUIRE_REVERT_FOR_BACKUP:-false}" = true ] && [ ! -f "${FAKE_CP_REVERT_MARKER:-}" ]; then
			echo 'live UCI delta was not reverted before config restore' >&2
			exit 78
		fi
		;;
esac
exec /bin/cp "$@"
