---
description: Run reviewers on specific paths or on changed files. Used by the pipeline (no args = git diff) and for ad-hoc reviews (with paths).
argument-hint: <optional paths, e.g. src/main, src/test>
allowed-tools: Read, Glob, Grep, Bash, Agent
---

Run reviewers on: **$ARGUMENTS**

## Step 1: List target files

**Paths provided** (comma-separated, e.g. `src/main, src/test`): split on commas, trim, use
`Glob` to list all files under each path:

```
Glob(pattern="**/*", path="<path1>")
Glob(pattern="**/*", path="<path2>")
```

Run all globs in parallel (single message).

**No paths** (pipeline mode): detect changed files via git:

```bash
git diff --name-only HEAD 2>/dev/null
git diff --name-only --cached 2>/dev/null
git ls-files --others --exclude-standard 2>/dev/null
git diff --name-only HEAD~1 2>/dev/null
```

Combine into deduplicated list. All commands empty → fall back to `git ls-files`.

**Also capture the range**, and pass it on in Step 5. A reviewer given `HEAD~3..HEAD` reads a
diff; one given nothing re-reads whole packages, which is where a re-gate's cost actually
goes. Pipeline mode after a fix pass: the range is the fix commits, not the whole feature
branch.

## Step 2: Discover reviewer agents

Use `Grep` to find agents with `type: reviewer` in frontmatter. Both searches in parallel:

```
Grep(pattern="type: reviewer", path="$HOME/.claude/agents/", glob="**/Agent.md")
Grep(pattern="type: reviewer", path=".claude/agents/", glob="**/Agent.md")
```

Global path missing → skip silently; project reviewers are enough.

**Sanity check:** this step yields **zero** reviewers → stop, report
`REVIEWER DISCOVERY FAILED`. Do not continue, do not report PASS. Every reviewer lives at
`.claude/agents/<name>/Agent.md` with `type: reviewer` + a `triggers:` list in frontmatter;
zero matches means layout drifted, not that code is clean.

For each matched file, `Read` only first 10 lines (frontmatter). Check `type: reviewer`
appears **inside the YAML frontmatter block** (between `---` markers), not body text. Discard
files where it only appears in body.

From each valid reviewer's frontmatter extract `name` + `triggers`. Read all matched files in
parallel.

## Step 3: Apply project trigger overrides

`.claude/review-triggers.json` exists in project root → read it, override triggers for matching
reviewer names. Missing → skip this step.

## Step 4: Filter by relevance

For each reviewer, check whether ANY target file matches ANY of its `triggers` globs (after
overrides). Skip reviewers with no matching files.

## Step 5: Launch relevant reviewers in parallel

Spawn all matching reviewers in a **single message** via `Agent`:

```
Agent(subagent_type="<name>", prompt="Review <commit range, or the listed paths>. Read
.claude/briefs/review.md first<, and .claude/briefs/proof.md — only for test-reviewer and
correctness-reviewer><, and .claude/briefs/navigation.md — only for arch-reviewer and
correctness-reviewer>. Scope: <the matched files, listed>. Start from the diff
and read only what it touches; widen only when the diff cannot settle a question, and say
which finding forced it. Report every finding you have in this round — a MINOR held back for
a later pass costs a whole extra gate. <On a re-gate: your prior findings were X; confirm each
is closed or still open. Findings STATE.md already records as deferred are out of scope.>")
```

Name the files. A reviewer told only "focus on `internal/`" reads the package; one handed six
paths reads six diffs.

**Run the coverage gate once, before spawning, and paste its output into every prompt:**
`.claude/scripts/uncovered-diff.py <range base>`. Uncovered added lines are an untested-change
finding no reviewer needs to rediscover by reading; handing every reviewer the same list stops
three of them paying to find it separately. Its rows are grouped per run with the enclosing
function, which is also the cheapest form to paste. If it exits 1 (anything outside the
"declared unreachable" section), send it back to the developer before spending a review
round at all. Also paste `.claude/scripts/test-stats.py --base <range base> --changed` so
reviewers read the test-count deltas instead of recounting.

Do NOT review code yourself — only orchestrate.

Spawn fails with `Agent type '<name>' not found` → agent was added after session start, roster
not re-enumerated. Report `REVIEWER SPAWN FAILED: <name> — restart the session`. Do **not**
report PASS for a reviewer that never ran.

## Step 6: Report

Reviewers return findings in shared format
`[BLOCKER|MAJOR|MINOR|NIT] <file>:<line> — <defect>` with `Failure:` and `Fix:` lines.
MINOR/NIT carry `Why:` instead of `Failure:` — a smell has no constructible failure by
definition. Accept either; missing `Failure:` on a MINOR is not malformed.

Merge into one severity-ranked list, deduplicating findings two reviewers raised on same
`file:line`. Keep reviewer name as prefix so developer knows who to re-run.

```
## Review Report

### Target
<path or "changed files">

### Triggered reviewers
- <name>: triggered by <matched files>

### Skipped reviewers
- <name>: no files matched triggers

### BLOCKER
<findings, prefixed with reviewer name>

### MAJOR
<findings, prefixed with reviewer name>

### MINOR
<findings, prefixed with reviewer name>

### NIT
<findings, prefixed with reviewer name>

### Strengths
<positive notes worth another author copying — omit if none>

### Verdict: PASS | PASS WITH FOLLOW-UPS | FAIL
- **FAIL** — any BLOCKER or MAJOR exists. Must be fixed before feature is done.
- **PASS WITH FOLLOW-UPS** — only MINOR/NIT findings. Feature is done; findings go to
  developer as fix-if-cheap, not a mandatory round trip.
- **PASS** — no findings.
```

**Pipeline mode: persist the report.** Write it to
`docs/specifications/<feature-slug>/REVIEW-<NN>.md` (next unused two-digit round) before
dispatching any fix pass. Fix-pass prompt cites that file instead of pasting findings, the
`METRICS.md` round row counts from it, and `pipeline-reviewer`'s retro reads it; report left
only in conversation is gone.

Style preferences never fail the gate. A `refactor-advisor` suggestion alone is
**PASS WITH FOLLOW-UPS**, not FAIL.
