#!/bin/sh
set -eu

while [ "$#" -gt 0 ]; do
	case "$1" in
		-s|-k) shift 2 ;;
		--) shift; break ;;
		-*) exit 2 ;;
		*) break ;;
	esac
done
[ "$#" -ge 2 ] || exit 2
shift
exec "$@"
