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
| R3 | planned | | | | |
| R4 | planned | | | | |
| R5 | partial | `.claude/scripts/post-edit-check.sh` (PostToolUse hook in `.claude/settings.json`); `.claude/briefs/build.md` → *Edit hook* | | | Deviation: hook runs gofmt + `go build` only. Narrow tests would report expected red in acceptance / test-first phases; `golangci-lint` too slow per edit — both stay at phase boundaries |
| R6 | planned | | | | |
| R7 | planned | | | | |
| I1–I5 | held | reviewer gate, severity contract, `/run-reviewers` tool output, sequential developers, acceptance-test-first + final product-vision pass | | | |

## Baseline

From `feature-metrics.py` on the three features built before these changes. Every run is
heuristically attributed (no run tags existed), so checkpoint fix passes are counted inside
scenario rows and the gate-fix kind undercounts. Weighted = input-equivalent tokens (IE).

| Feature | Units | Subagent weighted | Developer share (weighted) | Median weighted per unit | Developer run median / p90 | Final-gate rounds | Checkpoint + gate BLOCKER+MAJOR per unit | Escaped defects |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| phase1-import-store | 12 | 88.4M | 79% | 4.0M | 4.0M / 10.1M | 4 | 3.3 | not tracked |
| phase0-snapshot | 9 | 79.6M | 80% | 4.3M | 3.3M / 9.7M | 6 | 3.0 | not tracked |
| discovery-quicken-library | 1 | 4.8M | 73% | 3.0M | 1.8M / 2.3M | 2 | 1.0 | not tracked |

Orchestrator (upper bound, weighted): phase1 9.4M, phase0 12.5M, discovery 1.4M.

Pass thresholds (spec → *Rollout and evaluation*): median weighted per unit at least 15%
lower; gate rounds not higher; BLOCKER+MAJOR per unit no more than 25% higher; escaped
defects not higher.
