# Brief: reviewing

For every reviewer. Read with `.claude/rules/agent-briefs.md` (core).

## Reviewing: scope and completeness

Review gate not free, and largest avoidable cost is reviewers re-reading whole packages they already read in earlier round.

**Read the delta, not the tree.** Your prompt names commit range or file list. Start from `git diff <range>`, read only what diff touches. Every reviewer has `Bash` for exactly this; reviewer that cannot run it say so rather than quietly reading whole packages. Widen to whole file when diff alone cannot settle question — and say in finding why you had to. Package you already reviewed in earlier round, on surface this fix did not touch, has nothing new in it.

**Report every finding in the round you find it.** No hold MINOR back "for next pass", no open with finding you then withdraw, no re-raise finding previous round already recorded as deferred. Finding that arrives one round late costs whole extra gate: developer pass, re-gate, and every reviewer that re-reads result.

**Say what you could not check.** Path you had no way to exercise — environment you cannot change, host you cannot detect — reported as unchecked, not silently passed, not guessed at. Unchecked = fact caller can act on; guess = one they cannot.

**Never mutate worktree.** Reviewers run parallel; mutation in shared tree poisons every concurrent run. Mutate `git archive <sha>` export under `$TMPDIR` (`proof.md`).
