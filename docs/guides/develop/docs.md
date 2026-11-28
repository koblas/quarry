# How to write documentation

This site is built by [Zensical](https://zensical.org) from `mkdocs.yml` (Zensical reads the MkDocs format) and the pages in `docs/`, except `docs/adr`, `docs/prior-art`, `docs/product`, `docs/security` and `docs/specs`, which stay in the repository. It follows the [Divio documentation system](https://docs.divio.com/documentation-system/): every page is a tutorial, a how-to guide, an explanation or reference, never a mix.

## 1. Pick the quadrant

| The reader wants to… | Quadrant | Where | Rules |
|---|---|---|---|
| Learn by doing, from nothing | Tutorial | `docs/tutorials/` | Numbered steps that always work; one path; explain nothing beyond what the step needs; end with "Next". |
| Get one task done | How-to | `docs/guides/` | Title starts with "How to"; assumes the reader knows what quarry is; steps, not background; links to reference for keys and flags. |
| Understand why or how | Concept | `docs/concepts/` | Prose and diagrams; no steps, no flag or key tables. |
| Look something up | Reference | `docs/reference/` | Tables; every command, flag and config key; describes, never instructs. |

One audience per page. No internal references — spec or scenario ids, ADR numbers, `(not built: #N)` marks: say what quarry does, and link the design document on GitHub only where a reader needs the rationale.

## 2. Write it

- Add the page to `nav:` in `mkdocs.yml` (a test fails otherwise).
- Never name a page `claude.md` in any letter case: macOS's case-insensitive filesystem makes it a `CLAUDE.md`, which Claude Code loads as instructions (a test fails).
- A behavior change usually touches one how-to and one reference page; a new idea also gets a concept paragraph.
- Use Material content tabs (`=== "Claude Code"`) for alternatives, admonitions for warnings, each with a title.
- Every statement traces to code or `--help` output; never document from memory.

## 3. Write it in the site's voice

- Tutorials say "you" in the present tense, how-tos are imperative, concepts describe quarry in the third person, reference says "X does Y".
- Short declaratives, present tense, exact numbers. No "simply", "just", "please", "we", or marketing adjectives.
- quarry is lowercase, also at the start of a sentence. Commands, flags, keys and paths go in backticks.
- Link text names the target, never "here".

## 4. Check it

```bash
make docs
```

builds strictly: a broken link or anchor fails. It needs [uv](https://docs.astral.sh/uv/) and runs Zensical through `uvx`.

```bash
go test ./docs ./internal/config
```

fails when a command or flag in `quarry --help` has no entry on [The CLI](../../reference/cli.md), a config key has none on [config.toml](../../reference/config.md), a page is missing from the nav, or a public page cites an internal reference.

```bash
make docs-serve
```

previews at <http://127.0.0.1:8000>. CI's `docs` workflow builds the site on every pull request that touches it, and publishes `main` to GitHub Pages.
