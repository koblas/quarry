# Review Report — round 04 (re-gate after fix pass 3, final-pass copy)

### Target
Fix range `c7713737..4478fa91` (tree clean).

### Triggered reviewers
- test-reviewer (copy branch + table rows; correctness not re-run: no new fallible call).

### Prior findings
- Final-pass MAJOR 1 (Kept line) and MAJOR 2 (other-scope hint): **CLOSED**; spec, production and test strings byte-identical; six scope arms + mixed-order row present.

### MAJOR
- **test-reviewer** `internal/cli/claude_uninstall_test.go:173-199` — other-scope rows use only `managed`/`enterprise`; an allow-list mutant (`== "managed" || == "enterprise"`) survives and would hand an unknown or empty scope the `--scope <x>` remove advice; `%q` wrapping unpinned. Fix (pin-only): rows for `"scope":"we\"ird"` (expects `"we\"ird"` quoted) and an entry with no `scope` key (expects `""`). **Deferred to STATE.md Open debts under the fix-pass cap (3): pin-only, changes no exit code or written file.**

### MINOR
- **test-reviewer** `render_claude.go:122-124` — `remainingCopyHint` doc 3 lines (budget 2). Deferred.

### NIT
- **test-reviewer** — the two other-scope rows vary scope and projectPath together. Deferred.
- **test-reviewer** README:44 — "that copy and the quarry marketplace stay" is untrue when the marketplace is absent/foreign or the copy is `managed`. Ruled copy; recorded for a later copy pass.

### Verdict: PASS WITH FOLLOW-UPS (by cap rule; test-reviewer's own verdict was BLOCKED on the pin-only MAJOR)
