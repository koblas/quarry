# quarry product spec

The current truth about what quarry does: goals, surface, business rules, copy and the dated
Decisions. Every design step reads [decisions.md](decisions.md) first. A change to a rule or to
user-visible copy edits the file it belongs to in the same pull request as the code.

Split on 2026-11-26 from `docs/initial-prd.md` (Sep 27, 2026), which was mirrored from a claude.ai
artifact (rev 47). That artifact is retired: this folder is the authority. Section headings are
unchanged, so `## <heading>` anchors survive the move.

| File | Sections |
| --- | --- |
| [overview.md](overview.md) | Overview, Prior art, Goals and non-goals |
| [users.md](users.md) | Users and use cases |
| [architecture.md](architecture.md) | Architecture, Data model |
| [components.md](components.md) | Components and requirements: sync, CLI, MCP server, Claude skill, data-quality feedback, reporting currency and ACB |
| [security-privacy.md](security-privacy.md) | Security and privacy |
| [testing.md](testing.md) | Testing and validation |
| [milestones-risks.md](milestones-risks.md) | Milestones, Risks and open questions |
| [decisions.md](decisions.md) | Decisions, and the rulings folded from feature specs |

A requirement that is written but not built carries its open issue inline, as the marks in
[testing.md](testing.md) do; the pipeline plugin's `not-built.py` fails CI when a mark names an issue that is not open.
Issues for this spec carry the label `area:components`.
