# Adoption: quarry against "Spec: agent pipeline efficiency improvements"

Spec: <https://claude.ai/code/artifact/98e363c4-5ae5-4051-ad18-2e18edc94c7b>

R1–R7 landed together (one commit per requirement, in rollout order) at the owner's
request, not one per feature as the spec's rollout section recommends. The next features
therefore judge the set, not each change; revert a single requirement's commit if the ledger
points at it.

| ID | Status | Where implemented | Baseline | After | Notes |
| --- | --- | --- | --- | --- | --- |
| R1 | done | `.claude/scripts/feature-metrics.py`; CLAUDE.md → *Run tags*; `.claude/briefs/metrics.md` | see *Baseline* | | Orchestrator row is an upper bound (session window). Weights default to 0.1 cache read / 5 output; check against current pricing |
| R2 | done | `.claude/briefs/build.md` → *Developer runs*; architect `Runs:` header; CLAUDE.md step 5; developer prompt contract | developer run median / p90 in *Baseline* | | Phase report lives in the plan file, rewritten each run. Early hand-off uses a ~40-tool-call proxy for "context past 50% of window", which the agent cannot measure directly |
| R3 | done | architect → *Size verdict* (mandatory SPLIT thresholds, `Size:` header line) | largest unit share: phase1 31%, phase0 13% | | "One package" read as one feature package; `internal/cli` and `cmd/quarry` wiring for it don't count, since every command scenario touches them. Ledger p90 threshold activates at 20+ recorded units |
| R4 | done | architect → *Size verdict* (LIGHT); `.claude/briefs/build.md` → *Light lane*; CLAUDE.md step 4 | | | LIGHT decided at the sizing pass, so the per-scenario architect run is the cost saved. Excludes the mandatory test-first set; `L` run escalates to the architect if the plan grows past 3 Build steps |
| R5 | partial | `.claude/scripts/post-edit-check.sh` (PostToolUse hook in `.claude/settings.json`); `.claude/briefs/build.md` → *Edit hook* | | | Deviation: hook runs gofmt + `go build` only. Narrow tests would report expected red in acceptance / test-first phases; `golangci-lint` too slow per edit — both stay at phase boundaries. Verified: silent on a clean `cmd/quarry` edit with no binary left behind, silent when the file's checkout differs from `CLAUDE_PROJECT_DIR`, exit 2 with the compiler error on a broken package. Not yet seen firing live: it loads in the next session |
| R6 | partial | `spec-check.py --run`; CLAUDE.md step 7; `.claude/briefs/build.md` → *Scenario traceability* | | | Mapping checked mechanically and each ticked test executed at the gate (control: an injected `t.Fatal` was reported). Not done: executable Gherkin (Cucumber-style step definitions) and the reverted-change red check — the checkpoint's question 1 still judges whether the test asserts the scenario's `Then` |
| R7 | done | `.claude/scripts/mutation-sample.py`; `/run-reviewers` Step 5; test-reviewer → *Review procedure* | no mutation data before this change | | Controls: 6 of 6 mutants killed on the tested `sql` fixes; 3 of 3 survived on an untested throwaway package. One operator flip per line; own package's tests first, then — only if it survives — every package whose test build imports it, each stage limited to max(`--timeout`, 2 × its baseline). `--profile` path not exercised yet |
| I1–I5 | held | reviewer gate, severity contract, `/run-reviewers` tool output, sequential developers, acceptance-test-first + final product-vision pass | | | |

## Baseline

From `feature-metrics.py` on the four features built before these changes. Every run is
heuristically attributed (no run tags existed), so checkpoint fix passes are counted inside
scenario rows and the gate-fix kind undercounts. Weighted = input-equivalent tokens (IE).

| Feature | Units | Subagent weighted | Developer share (weighted) | Median weighted per unit | Developer run median / p90 | Final-gate rounds | Checkpoint + gate BLOCKER+MAJOR per unit | Escaped defects |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| phase2a-read-foundation | 10 | 19.9M | 62% | 1.6M | 0.7M / 1.5M | 2 | 0.8 | not tracked |
| phase1-import-store | 12 | 88.4M | 79% | 4.0M | 4.0M / 10.1M | 4 | 3.3 | not tracked |
| phase0-snapshot | 9 | 79.6M | 80% | 4.3M | 3.3M / 9.7M | 6 | 3.0 | not tracked |
| discovery-quicken-library | 1 | 4.8M | 73% | 3.0M | 1.8M / 2.3M | 2 | 1.0 | not tracked |

Orchestrator (upper bound, weighted): phase2a 5.3M, phase1 9.4M, phase0 12.5M, discovery 1.4M.

phase2a is the primary comparator: it is the most recent and ran on the same model mix (Sonnet 5.5 developers) the next features will use. The older three ran on Sonnet 5 and cost 2–5x more per unit, so a comparison against them would credit these changes with a model upgrade.

Pass thresholds (spec → *Rollout and evaluation*): median weighted per unit at least 15%
lower; gate rounds not higher; BLOCKER+MAJOR per unit no more than 25% higher; escaped
defects not higher.
