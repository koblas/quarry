# Feature specs

One file per feature, `docs/specs/<slug>.md`, written at scoping by the pipeline plugin. The
product spec in `docs/product/` holds the rules and copy; a feature spec links to it and does not
restate it.

A feature spec has four parts:

1. **Intent and out of scope.** What the change is for, and what it deliberately leaves out.
2. **Decisions.** The kickoff's decisions list, dated and numbered.
3. **Scenarios.** One per action, each with a case table: one row per case (happy case first, then
   each outcome that matters), giving the input, the expected result and, for a refusal, the exit
   code and the exact line. One `scenario` sub-issue per scenario.
4. **Links.** The `feature` issue, the product-spec sections it changes, and the pull request.

The kickoff is three lines: what Claude understood, the open questions, and whether the feature
runs here or unattended.

Features built before this layout are under `docs/specifications/<slug>/` and use Gherkin; they
are not converted.
