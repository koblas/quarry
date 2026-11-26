#!/usr/bin/env bash
# Checks that every non-merge commit in <base>..<head> has a Conventional Commits subject:
#   type(scope): summary      scope optional, may list several: (cli,report); "!" marks a break
# Types are the ones this repo uses plus the standard extras. Run in CI on each pull request;
# run it by hand before pushing:  .claude/scripts/commit-lint.sh origin/main HEAD
set -euo pipefail

base=${1:?usage: commit-lint.sh <base> [head]}
head=${2:-HEAD}
base=$(git rev-parse --verify --quiet "$base^{commit}") || { echo "commit-lint: unknown base \"$1\"" >&2; exit 2; }
head=$(git rev-parse --verify --quiet "$head^{commit}") || { echo "commit-lint: unknown head \"${2:-HEAD}\"" >&2; exit 2; }
pattern='^(feat|fix|docs|refactor|test|chore|ci|build|perf|style|revert)(\([a-z0-9._/,-]+\))?!?: [^ ].*'

bad=0
while IFS= read -r line; do
	sha=${line%% *}
	subject=${line#* }
	if ! grep -qE "$pattern" <<<"$subject"; then
		echo "commit-lint: ${sha:0:8} \"$subject\"" >&2
		bad=$((bad + 1))
	fi
done < <(git log --no-merges --format='%H %s' "$base..$head")

if [ "$bad" -gt 0 ]; then
	echo "commit-lint: $bad commit(s) need a subject like 'fix(snapshot): refuse an empty file'." >&2
	echo "commit-lint: types: feat fix docs refactor test chore ci build perf style revert." >&2
	exit 1
fi
echo "commit-lint: ok"
