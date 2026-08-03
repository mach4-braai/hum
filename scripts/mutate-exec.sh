#!/usr/bin/env bash
set -u

backup=$(mktemp)
cp "$MUTATE_ORIGINAL" "$backup"
cp "$MUTATE_CHANGED" "$MUTATE_ORIGINAL"

output=$(go test -timeout "${MUTATE_TIMEOUT}s" "$MUTATE_PACKAGE" 2>&1)
status=$?

cp "$backup" "$MUTATE_ORIGINAL"
rm -f "$backup"

case $output in
*"test timed out"*)
	diff --label=Original --label=New -u "$MUTATE_ORIGINAL" "$MUTATE_CHANGED"
	exit 3
	;;
*"[build failed]"*)
	exit 2
	;;
esac

if [ "$status" -ne 0 ]; then
	exit 0
fi

diff --label=Original --label=New -u "$MUTATE_ORIGINAL" "$MUTATE_CHANGED"
exit 1
