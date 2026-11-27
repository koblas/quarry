#!/bin/sh
# The Verify block as one command: build, the full covered suite once, the
# coverage gate, race on the touched packages, lint, the not-built marks, and
# test counts. The not-built step reads open issues with gh (needs gh auth).
# The coverage gate is the pipeline plugin's diff-coverage check, judged on this
# script's own profile: every Go block changed since <start> runs in a test, or
# says why it cannot with `// uncovered: <reason>`. It and the not-built check
# are read from a koblas/throughline checkout: $THROUGHLINE, else .throughline
# (CI checks it out there at the pinned release tag), else the marketplace copy
# Claude Code keeps at ~/.claude/plugins/marketplaces/throughline. With none,
# both steps fail (rc=2).
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

throughline=${THROUGHLINE:-}
if [ -z "$throughline" ]; then
	for candidate in "$root/.throughline" "${HOME:-}/.claude/plugins/marketplaces/throughline"; do
		if [ -f "$candidate/plugins/pipeline/scripts/check.py" ]; then
			throughline=$candidate
			break
		fi
	done
fi
scripts="$throughline/plugins/pipeline/scripts"
coverage=skipped
if [ "$tests" -eq 0 ]; then
	if [ -f "$scripts/check.py" ]; then
		# check.diff_coverage is what the plugin's `check.py phase` runs: diff-coverage.py's
		# uncovered() on a profile already made, so the suite does not run a second time.
		python3 -I - "$scripts" "$root" "$cover" "$start" <<'EOF'
import importlib.util
import sys
from pathlib import Path

scripts, root, cover, start = sys.argv[1:]
spec = importlib.util.spec_from_file_location("check", Path(scripts) / "check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)
root = Path(root)
found = check.diff_coverage(root, ["./..."], Path(cover), start, check.snapshot(root))
for line in found:
    print(line)
print(f"diff-coverage: {len(found)} changed block(s) uncovered" if found else "diff-coverage: ok", file=sys.stderr)
sys.exit(1 if found else 0)
EOF
		coverage=$?
	else
		echo "diff-coverage: no koblas/throughline checkout found; set THROUGHLINE to one" >&2
		coverage=2
	fi
fi
echo "diff-coverage rc=$coverage"

race=skipped
if [ $# -gt 0 ]; then
	go test -race "$@"
	race=$?
fi
echo "go test -race rc=$race"

golangci-lint run ./...
lint=$?
echo "golangci-lint rc=$lint"

if [ -f "$scripts/not-built.py" ]; then
	python3 "$scripts/not-built.py"
	notbuilt=$?
else
	echo "not-built: no koblas/throughline checkout found; set THROUGHLINE to one" >&2
	notbuilt=2
fi
echo "not-built rc=$notbuilt"

python3 .claude/scripts/test-stats.py --base "$start" --changed

for rc in "$build" "$tests" "$coverage" "$race" "$lint" "$notbuilt"; do
	case $rc in
	0 | skipped) ;;
	*) exit 1 ;;
	esac
done
exit 0
