---
description: Creates a new reviewer agent with the correct frontmatter and structure. Asks clarifying questions, then generates the agent file.
argument-hint: <optional-reviewer-name>
allowed-tools: Read, Write, Edit, Glob, AskUserQuestion
---

Create a new reviewer agent. Name provided: **$ARGUMENTS**

## Step 1: Gather information

Ask the user these (skip any already answered via the argument):

1. **Name**: what is this reviewer called? (e.g. `presentation-reviewer`, `security-reviewer`).
   kebab-case.
2. **Purpose**: what does it check? (e.g. "ensures API response DTOs don't leak domain
   internals")
3. **Scope**: which files trigger it? Ask for one or more glob patterns. Offer examples:
   - Production code: `cmd/**/*.go`, `internal/**/*.go`
   - Test code: `**/*_test.go`
   - Delivery layer: `internal/cli/**`
   - Feature packages: `internal/<feature>/**`
   - Shared infrastructure: `internal/platform/**`
   - Config files: `**/*.yml`, `**/*.yaml`, `**/*.toml`, `**/*.json`
   - Build/toolchain: `devenv.nix`, `go.mod`
4. **Placement**: global (`~/.claude/agents/`) or project-specific (`.claude/agents/`)? Default
   project-specific.
5. **Checklist**: what specific things does it check? Have the user describe the rules,
   conventions, patterns it enforces. Probe for:
   - What BLOCKER looks like (the worst thing it can find)
   - What MAJOR looks like (a defect with a constructible failure)
   - What MINOR looks like (a smell with no constructible failure)
6. **Model**: which tier? Default `sonnet` for reviewers.

## Step 2: Review existing reviewers

Before creating, glob for existing reviewer agents in the target location to avoid duplication.
Similar reviewer exists → ask the user whether to update it instead.

## Step 3: Generate the agent file

Create the agent at `<placement>/<reviewer-name>/Agent.md` with this structure:

`type: reviewer` and `triggers:` are **required** — `/run-reviewers` discovers reviewers
by grepping for them. A reviewer missing either is invisible to the gate.

```markdown
---
name: <reviewer-name>
description: Chief <Purpose> Officer for <scope>. <What it guards.> Invoke <at which points — design, implementation, review>. Returns ranked findings; it does not rewrite the code.
type: reviewer
triggers: [<glob patterns from Step 1>]
tools: Read, Glob, Grep
model: <model>
effort: <low|medium|high>
color: <pick a color not used by existing reviewers>
---

You are the Chief <Purpose> Officer for <scope>.

Your mandate: <one sentence on what must be true when you pass it>.

## Scope

<What this reviewer owns — and explicitly what belongs to the other reviewers
(arch-reviewer: structure; correctness-reviewer: wrong behavior; refactor-advisor:
quality; test-reviewer: tests; api-reviewer: HTTP conformance;
product-vision: whether the surface should exist at all).>

## What to check

<Generated from the checklist in Step 1 — organized as sections>

## Output format

Findings ranked most-severe first:

```
[BLOCKER|MAJOR|MINOR|NIT] <file>:<line> — <the defect, one sentence>
  Failure: <concrete input or state → the wrong result>
  Fix: <specific change>
```

Then exactly one verdict line: **PASS**, **PASS WITH FOLLOW-UPS**, or **BLOCKED**
(blocking items named).

Severity contract, shared across all reviewers in this repo:

- **BLOCKER** — <the worst thing this reviewer can find>
- **MAJOR** — <a real defect with a constructible failure>
- **MINOR** — <a smell with no failure you could construct>
- **NIT** — preference. Never blocks.

If you cannot construct a concrete failure, downgrade the finding to MINOR and say that
you could not. A plausible-sounding finding that cannot fail is noise.

## Rules

- Ranked by severity, always.
- Distinguish "this is wrong" from "I'd write it differently". Only the first blocks.
- A new dependency is not a defect.
- Match the file's existing idiom rather than imposing a different one.
- You do not rewrite the code. Name the defect precisely enough to fix in one pass.
```

## Step 4: Project trigger overrides

If the reviewer is **global** but the user mentions it will be used in projects with different file conventions (e.g. a project whose tests are `*.spec.ts` rather than `*_test.go`), inform them they can override triggers per project by adding an entry to `.claude/review-triggers.json`:

```json
{
  "<reviewer-name>": ["**/*.spec.ts", "**/*.test.ts"]
}
```

The review-gate reads this file and replaces the agent's frontmatter triggers with the override. Only reviewers that need different patterns need an entry.

## Step 6: Confirm

Show the user the generated file path and a summary of what the reviewer will check and when it triggers. Remind them it is registered in the reviewer table and will be picked up by the `review-gate` on the next scenario run.
