#!/bin/sh
# The Verify block as one command: build, the full covered suite once, the
# coverage gate, race on the touched packages, lint, and test counts.
# One plain call: the coverage profile is made, read and named in the same
# process, so no shell variable has to survive between tool calls.
#
# usage: .claude/scripts/verify.sh <start> [race-pkg ...]
#   <start>    commit the scenario or fix pass started from
#   race-pkg   packages for go test -race, e.g. ./internal/report/...
#
# Prints one "<step> rc=<n>" line per step; exits 1 if any step failed.
set -u

start=${1:?usage: verify.sh <start> [race-pkg ...]}
shift

root=$(git rev-parse --show-toplevel) || exit 2
cd "$root" || exit 2
dir=$(mktemp -d "${TMPDIR:?TMPDIR must be set}/verify.XXXXXX") || exit 2
cover="$dir/cover.out"
log="$dir/test.log"

go build ./...
build=$?
echo "go build rc=$build"

go test -count=1 -coverpkg=./... -coverprofile="$cover" ./... >"$log" 2>&1
tests=$?
grep -E '^(FAIL|--- FAIL|panic:)' "$log"
echo "go test rc=$tests (log: $log)"

uncovered=skipped
if [ "$tests" -eq 0 ]; then
	python3 .claude/scripts/uncovered-diff.py --profile "$cover" "$start"
	uncovered=$?
fi
echo "uncovered-diff rc=$uncovered"

race=skipped
if [ $# -gt 0 ]; then
	go test -race "$@"
	race=$?
fi
echo "go test -race rc=$race"

golangci-lint run ./...
lint=$?
echo "golangci-lint rc=$lint"

python3 .claude/scripts/test-stats.py --base "$start" --changed

for rc in "$build" "$tests" "$uncovered" "$race" "$lint"; do
	case $rc in
	0 | skipped) ;;
	*) exit 1 ;;
	esac
done
exit 0
