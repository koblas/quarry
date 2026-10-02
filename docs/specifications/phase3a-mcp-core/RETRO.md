# Retro proposals: phase3a-mcp-core

pipeline-reviewer retro, 2026-10-02. Proposals only; `.claude/` changes go in their own change.

Evidence read: METRICS.md, REVIEW-01/02, STATE.md, spec Rule 4, .claude/scripts/mutation-sample.py, .claude/commands/run-reviewers.md, .claude/rules/go-code.md, the reviewer and developer agent files.

## Where the tokens went
Total subagent cost was 15.8M IE. The orchestrator added about 4.7M as an upper bound.
- Top three scenarios: SCENARIO-06 at 2,270k (14%), SCENARIO-01 at 1,585k (10%), SCENARIO-03 at 1,483k (9%).
- The "-" unit (scoping plus gate fix) was 4,016k (25%). The gate-fix developer run alone was 1,011k, the single most expensive run.
- Fix passes: gate-fix 1,011k (6%) plus checkpoint-fix 348k (2%) is 8% of total. Build is 47%. Checkpoints cost 1,120k (7%) and produced two fix passes.
- Developer is 56% of cost (28 runs, 8.9M). The median run is 274k, so the cost is mostly context re-read per run, not fix churn.

## Late findings that belong to an earlier stage
- **Gate R1 MAJOR (stderr copied the isError text, leaking row values).** Caught late in METRICS (shipped by S03). Root cause is scoping: the Phase-1 spec said "stderr = the isError text" in the §4 table, while Rule 4 forbade leaking values. product-vision's Phase-1 pass ruled copy per outcome but never checked the spec's rules against each other.
  - S03's checkpoint passed it because the checkpoint questions are about coverage and tests, not data flow.
  - Cost: one gate-fix run (1,011k) plus a second gate round (correctness, test and refactor reviewers, 214s mutation).
- **Mid-feature copy rulings.** There were 4 `product-vision` rulings plus 2 orchestrator rulings. One was the S03 limit-1 and no-statement wording. One was the S11 warning order. One was the S06 timeout line. The orchestrator also ruled the S07 sort and ordering. These are copy for outcomes that scoping could have enumerated: refusals, warning orders, timeout and limit wording. The existing rule "Rule copy when new failure mode appears" worked as designed. The scoping gap is that no table of per-tool outcomes was required up front.
- **Doc-budget MINORs recurred at nearly every checkpoint.** Seven checkpoints (S01 through S06) and gate R1 and R2 all report them. The budget rule is already in `.claude/rules/go-code.md` (paths `**/*.go`, so developers load it). The checkpoint and refactor-advisor enforce it, but nothing in the developer's Verify loop does. Comment MINORs go to STATE.md `## Open debts` when there is no fix pass, so they accumulate unowned. Gate R1 still found five budget breaches.

## Ranked rule changes
1. **`.claude/agents/pipeline-reviewer` is not the home. Target `.claude/briefs/build.md` → *Developer runs* (Verify phase) plus `.claude/agents/developer/Agent.md` → Verify step.**
   Add one step: before handing off, list every doc comment the run added (`git diff <start> -U0`, comment lines) and trim to the `go-code.md` budget. Cite `go-code.md`; do not restate it.
   - Evidence: nearly every checkpoint and both gate rounds carried the same MINOR, and each one cost reviewer tokens and cheap-fold developer tokens.
   - Better still: add a `comment-budget` mode to `.claude/scripts/uncovered-diff.py` or a new script that counts doc-comment lines on added symbols. Then it is a mechanical gate, like coverage, and the reviewers stop spending tokens on it.
   - Today only fix-mode item 8 (developer Agent.md:189) addresses comments.
2. **`.claude/commands/intent-and-goal.md` (scoping step) plus `product-vision` Phase 1: add a "cross-rule consistency check" and an outcome table.**
   - Spec rules that name an output channel (stderr, logs, exit codes) must be checked against the rules that name data-leak limits. Require a per-tool outcome table (outcome, client text, stderr line, exit) before approval.
   - This would have removed the gate-R1 MAJOR, the 1,011k gate-fix run, and most of the 6 copy rulings. It is the single change with the largest fix-pass saving.
3. **`.claude/agents/test-reviewer/Agent.md` and the other reviewer agents: add an explicit no-load-generator rule.**
   - A reviewer must not spawn background or busy-loop processes, stress loops, or repeat-until-fail runs beyond `-count=N` on a single test.
   - To reproduce a flake it uses `go test -run X -count=20` (single process) and reports; it never launches parallel load.
   - Anything it starts must be killed by the same Bash call (`timeout`, a wait on the PID).
   - Home: one line in `.claude/briefs/review.md`, which every reviewer reads. Do not put it in `agent-briefs.md`, which is always loaded.
4. **`.claude/scripts/mutation-sample.py` and `.claude/commands/run-reviewers.md` (the mutation-sample paragraph).**
   - The baseline run uses `args.timeout * 3` (360s) over all dependents of a changed package. `internal/report` dependents exceed it (script line 229). Default `--timeout 120` is too low there.
   - Make the baseline budget its own flag (`--baseline-timeout`, default 900) rather than a multiple.
   - run-reviewers must run the sample after reviewers return, or before spawning them, never concurrently. Today it can overlap, and `cmd/quarry` baseline failed under reviewer load.
   - On a baseline timeout the script exits 2 without saying which package. Print the package and elapsed time.
   - Two aborted attempts cost retries; the R1 sample then took 901s.
5. **`.claude/agents/test-reviewer/Agent.md` → *Checkpoint mode*: add a data-flow question.**
   - "Does any new output channel (log, stderr, error text) carry values from user data or backend error text?"
   - This is a smaller version of change 2, for features where scoping has already happened.
6. **`.claude/briefs/build.md` → *Planning*: unowned open debts.**
   - STATE.md `## Open debts` has about 20 "unowned" items at the end of phase3a. Add a line that the final `product-vision` or orchestrator step assigns each debt to a named next feature or drops it. The one real hazard is "OWNED BY 3b" for account-refusal text embedded in the stderr line (REVIEW-02 result.go:55). That one must be a scenario in phase 3b.

## Notes
- No contradiction or dangling-pointer defects found in the files I read for this retro.
- The `go-code.md` budget (about 4 lines exported, 1 to 2 unexported) is stated once there. refactor-advisor (Agent.md:40) and test-reviewer (Agent.md:45-47) cite it. That is correct, and change 1 should also cite it rather than copy the numbers.
- A retro cannot see what the busy-loop incident did to the machine beyond the orchestrator's report; I did not verify it independently.
