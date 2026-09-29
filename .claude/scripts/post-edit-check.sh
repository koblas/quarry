#!/usr/bin/env bash
# post-edit-check — PostToolUse hook for Edit/Write/MultiEdit on Go files (R5).
#
# Runs gofmt and `go build` on the edited file's package. Silent and exit 0 when both pass,
# when the file is not Go, or when the pinned toolchain is not reachable (never blocks a
# session over its own environment). On failure prints at most 50 lines to stderr and exits
# 2, which Claude Code feeds back to the agent that made the edit.
#
# Tests are deliberately NOT run: acceptance and test-first phases are red by design, so a
# test run per edit would report expected failures. Tests stay at phase boundaries
# (.claude/briefs/build.md). Lint stays in the Sweep phase (golangci-lint is too slow per
# edit).
set -uo pipefail

file="$(/usr/bin/python3 -c 'import json,sys; print((json.load(sys.stdin).get("tool_input") or {}).get("file_path",""))' 2>/dev/null)"
case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac
[ -f "$file" ] || exit 0

root="${CLAUDE_PROJECT_DIR:-$(git -C "$(dirname "$file")" rev-parse --show-toplevel 2>/dev/null)}"
[ -n "$root" ] && cd "$root" || exit 0

if ! command -v go >/dev/null 2>&1 || [ -z "${DEVENV_ROOT:-}" ]; then
  command -v direnv >/dev/null 2>&1 && eval "$(direnv export bash 2>/dev/null)"
fi
command -v go >/dev/null 2>&1 || exit 0
pin="$(sed -n 's/^go \([0-9.]*\)$/go\1/p' go.mod 2>/dev/null | head -1)"
case "$(go env GOVERSION 2>/dev/null)" in
  "$pin"*) ;;
  *) exit 0 ;;
esac

rel="${file#"$root"/}"
pkg="./$(dirname "$rel")"
out="$( { gofmt -l "$file" | sed 's/^/gofmt: needs formatting: /'; go build "$pkg" 2>&1; } )"
[ -z "$out" ] && exit 0

total="$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
{
  echo "post-edit-check ($rel):"
  printf '%s\n' "$out" | head -50
  [ "$total" -gt 50 ] && echo "... $((total - 50)) more line(s) cut"
} >&2
exit 2
