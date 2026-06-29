#!/bin/sh
# coverage.sh — run the test suite with coverage and print a per-function summary
# plus the total. Writes coverage.out (gitignored). HTML report with: -html
set -eu

cd "$(dirname "$0")"

profile=coverage.out
go test -coverprofile="$profile" -covermode=atomic ./... >/dev/null

echo "── coverage by function ──"
go tool cover -func="$profile" | tail -n 40

total=$(go tool cover -func="$profile" | awk '/^total:/ {print $3}')
echo
echo "TOTAL: ${total}"

if [ "${1:-}" = "-html" ]; then
	go tool cover -html="$profile"
fi
