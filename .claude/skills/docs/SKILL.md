---
name: docs
description: "Use when a behavior change needs documenting, when asked to write or update a page of the user documentation site, or when a change touches cmd/, internal/cli or internal/config without touching docs/. Puts the change in the right Divio quadrant (tutorial, how-to, concept, reference), keeps one audience per page, then runs the doc checks."
argument-hint: "<behavior change, or the page to write or fix>"
---

# Document a change

Document this: $ARGUMENTS

The site is `mkdocs.yml` plus `docs/` minus the design documents it excludes
(`adr/`, `prior-art/`, `product/`, `security/`, `specs/`). Read
`docs/guides/develop/docs.md` first: it holds the quadrant table, the voice rules and the
checks. This skill is the procedure.

## 1. Find what changed

Read the diff (`git diff origin/main...HEAD`, or the files named) and list what a reader can
now observe: a command, flag, default, `config.toml` key or value, finding type, MCP tool,
file path, refusal or warning line. For a page request, read the page and the code it
describes. Never document from memory: every statement traces to code or to
`quarry <command> --help` output (build with `go build -o "$TMPDIR/quarry" ./cmd/quarry`).

## 2. Place each item

| Item | Page |
|---|---|
| Command, flag, default | `docs/reference/cli.md`, under `### \`quarry <command>\``; a flag in backticks as `--name` |
| `config.toml` key | `docs/reference/config.md`, the dotted key in backticks; the Example block must still load without a warning |
| A task a user now does differently | the how-to in `docs/guides/` |
| How snapshots, the store, findings, currencies or privacy work | `docs/concepts/how-quarry-works.md` |
| The first-run path | `docs/tutorials/first-question.md` only if its steps changed |

How-to never explains; reference never instructs; concepts never list flags or keys. A new
page goes in `mkdocs.yml` `nav:`. Never name a page `claude.md` in any case.

## 3. Check

```bash
go test ./docs ./internal/config
make docs-build
```

`make docs-build` needs `uv` and network access to PyPI for `uvx zensical`; if it cannot
run, say so in the report rather than skipping silently.
