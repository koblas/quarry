---
name: legacy-code
description: Use when working on untested, tangled, or outdated code — adding tests to system with none, untangling shared mutable state, replacing deprecated patterns, language/framework migrations, dependency rewrites. Follows Michael Feathers' "Working Effectively With Legacy Code" approach. Language-agnostic.
---

Phase order matter. No skip ahead. **Get code under test before change it.**

## Phase 0 — Audit

- Map boundary: what call into target, what target call out to.
- Risk-rank each touched file by branching, side-effects, current test coverage gap.
- Document **surprising findings** as locked-in tests, not silent fixes. Find dead code, obvious bug, contradictory invariant → add test pinning *current* (maybe wrong) behavior, comment why, put fix in separate PR with own ticket. Conflating "lock in current behavior" with "fix bug while there" make both reviews worse.

## Phase 1 — Safety net (Feathers' rule: get it under test before you change it)

- Pick smallest seam that let observe behavior. Mocks, fakes, stubs, recording adapters — whatever get passing test for current behavior.
- Mocks for infrastructure (databases, queues, third-party APIs) acceptable as **interim** — note in PR body what they miss (schema drift, network behavior, transactional edges, library quirks), migration-window risk only.
- Defer real-infra integration tests (containers, fakes + contract tests) to Phase 4. Adding inside migration PR widen change too much, stall progress.

## Phase 2 — Untangle shared mutable state during the change, not after

- Module-level / static / singleton mutable state = most common reason tests awkward. File-top caches, registries, session maps, environment-coupled config readers — all fight reset between tests.
- Lift into named factories with tunables passed in (`createCache(ttl)`, `createRegistry(loader)`). Consumer = factory taking state as input.
- Tests then build fresh instances per scenario. No reset rituals, no module reloads, no fake-timer dance around hidden init.
- Do in **same PR** as change already making. Split to follow-up tend to leave seam half-built, tests stay awkward.

## Phase 3 — Modernize incrementally

- One concern per PR. Keep review surface small.
- **Leaf-first.** Start files with no inbound/outbound local deps; safest renames/conversions. Work outward.
- Plan waves before writing — put wave list in first PR description, tick off in later PRs. Reviewers follow plan; future you resume after context break.
- No interleave migrations with feature work. Pause feature work on touched files until migration through them done.

## Phase 4 — Replace interim mocks

- Once system on new language/framework/pattern, swap mocks for real-infra tests: containers, fakes + contract tests against real dep, recorded interactions verified vs current capture.
- Target files flagged infrastructure-touching in Phase 0.
- Own PR(s), after migration done — awkward to add when surrounding code mid-rewrite.

## Cross-cutting principles

- **Pin behavior, then change.** Never change behavior in same commit that capture it. Test commit reviewable alone; change commit show exactly what shifted.
- **Surprising things become tests, not silent fixes.** Even if you "know" something wrong, prove current behavior with test first. Separate PR fixes. Audit more valuable than fix.
- **No backwards-compatibility cruft for one-shot migration.** Replacing pattern wholesale → replace. No carry both versions unless real partial-rollout reason (canary, dual-write, staged rollout).
- **Track gotchas as found.** First migration of kind hit surprises. Write in PR body; become playbook for next. After three migrations same kind, gotchas list = most valuable artifact.
- **Mocks → fakes → real infra = one-way street.** No mocks "for now" without concrete plan to remove. Not gonna remove → write fake from start.

## Out of scope (use a different skill)

- **Greenfield code** → use `tdd` + `clean-architecture`.
- **Pure dependency upgrades** with no behavior change → standard upgrade work, not legacy-code.
- **Architectural redesign** → separate planning concern, follow once legacy code testable.

---

Skill intentionally generic and language-agnostic. Project-specific applications (e.g. JavaScript → TypeScript wave templates, monorepo toolchain notes, reference PRs) live in project-level skill files complementing this one.