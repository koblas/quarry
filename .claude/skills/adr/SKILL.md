---
name: adr
description: Create an Architecture Decision Record (ADR) in docs/adr/. Describe the decision and context, or have a conversation to refine it.
argument-hint: <decision-title-or-description>
allowed-tools: Read, Write, Glob, Bash
---

Create Architecture Decision Record for: **$ARGUMENTS**

## Process

1. **Find next number**: Scan `docs/adr/` for existing ADRs, determine next sequential number (zero-padded to 3 digits).
2. **Gather context**: If argument clear decision with enough context, proceed. If vague or missing "why", ask clarifying questions before writing — ADR without clear rationale worthless.
3. **Write ADR** using template below.
4. **File name**: `docs/adr/NNN-slug.md` where `slug` lowercase, hyphenated summary (max 5 words).

## Template

```markdown
# ADR-NNN: Title

## Status

Accepted

## Context

Why this decision came up. What forces are at play — constraints, requirements, trade-offs.
Keep it concise but complete enough that a newcomer understands the problem.

## Decision

What we decided. State it as a fact, not a proposal.

## Consequences

What follows — both positive and negative. Be honest about trade-offs.
Use bullet points prefixed with **Positive:**, **Negative:**, or **Trade-off:**.
```

## Rules

- One decision per ADR. Multiple decisions → multiple ADRs.
- Plain language. No jargon without explanation.
- "Context" section must explain *why* — not just *what*.
- "Consequences" section must include at least one negative or trade-off. Every decision has cost.
- Keep each section concise — aim 2-5 sentences in Context and Decision, 3-6 bullets in Consequences.
- If `docs/adr/` missing, create it.
- Don't modify existing ADRs unless asked. To reverse decision, create new ADR superseding old one, update old status to `Superseded by ADR-NNN`.