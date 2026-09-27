---
name: clean-architecture
description: Go architecture for quarry — the single-module cmd/internal layout, thin-main + feature-package split, functional-options DI, the Store-interface + adapters port pattern, the dependency rule (what may import what), and Go code + doc conventions. Invoke when deciding where code lives, what a package may import, how to structure a new feature or command, or how to wire dependencies.
---

`quarry` is a **single Go binary**, one module at the repo root
(`github.com/koblas/quarry`). NOT layered the Vaughn-Vernon / IDDD way — no aggregates, no
`UseCase` classes, no `domain/application/infrastructure/api` folders, no
`Repository`-per-aggregate. The carryover from Clean Architecture is the *spirit*: strict
dependency direction, ports-and-adapters at every I/O boundary, thin delivery layer, deps
injected (never reached for). Below is how that looks here.

## Repository layout

```
cmd/quarry/                  # THIN binary — wiring + process entrypoint only
  main.go                   #   parse args/env, build deps, call run(), map error → exit code
internal/cli/               # command + flag definitions; parses input, delegates, formats output
internal/<feature>/         # a feature package — its logic, its ports, its adapters
  doc.go                    #   package doc
  <feature>.go              #   Server/service struct + functional options + logic
  store.go                  #   Store interface (the persistence port)
  memory.go                 #   in-memory adapter (dev + tests)
  <backend>.go              #   production adapter
internal/platform/<name>/   # generic, dependency-light infrastructure; no feature knowledge
```

Nothing above is sacred except the **dependency rule** and the **port placement**. Add a
directory when a package earns one; do not pre-create empty layers.

## The thin-main / feature-package split

`cmd/quarry` is **wiring only**: read config and flags, construct concrete dependencies,
assemble the feature packages with functional options, run, and translate a returned error
into an exit code. Keep `main()` itself to a handful of lines wrapping a `run() error` so
the body is testable and `defer`s actually run.

```go
func main() {
    if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
        fmt.Fprintln(os.Stderr, "quarry:", err)
        os.Exit(1)
    }
}
```

**No business logic under `cmd/`.** Streams (`io.Writer`), the clock's inputs, the
environment and the args are all passed in, never reached for from a deeper package — that
is what makes the command testable without a subprocess.

## Feature package shape (functional-options DI)

```go
package summarize

type Server struct {        // private fields = injected dependencies
    store  Store
    source Source
    out    io.Writer
}

type Option func(*Server)                                 // functional options
func WithStore(s Store) Option   { return func(x *Server) { x.store = s } }
func WithOutput(w io.Writer) Option { return func(x *Server) { x.out = w } }

func NewServer(opts ...Option) *Server {                  // constructor
    s := &Server{ /* defaults */ }
    for _, o := range opts { o(s) }
    // default a missing optional dep; fail loudly on a missing required one
    return s
}
```

Rules:

- **Functional options, not constructor-arg lists or a DI framework.** Every dep gets a
  `WithX(...) Option`.
- **Compile-time interface guard** (`var _ Iface = (*Server)(nil)`) wherever the type is
  required to satisfy an interface someone else declares.
- **Delivery is thin.** A CLI command parses and validates input *format*, calls the
  feature package, and formats the result. No business rules in `internal/cli`; no
  flag-parsing or terminal formatting in the feature package.
- Keep decision logic transport-free — a pure function or a method that takes values and
  returns values — so it is testable without building a command line.

## Ports & adapters

- **`Store` = the persistence port**, an interface defined **in the feature package that
  consumes it** (`internal/<feature>/store.go`). Not in a separate `domain/` tree.
- **Adapters sit beside it in the same package**: the production adapter plus an
  in-memory adapter (`memory.go`) for dev and tests. Convert domain↔persistence at the
  adapter boundary.
- **Every other external dependency is a port too** — an HTTP API, the filesystem, a
  subprocess, the network. Declare a small interface at the point of consumption; the real
  client is one adapter, a fake is another.
- Keep ports **minimal**. An interface with one implementation and twelve methods is not a
  port, it is the implementation spelled twice.
- Name it `Store` (plus adapters named for their backing). No `Repository`, `Finder`,
  `Reader`, or aggregate package here.

## Dependency rule (strict — the load-bearing part)

Dependencies point one way:

```
cmd/quarry  ──►  internal/cli  ──►  internal/<feature>  ──►  internal/platform/*
```

- `cmd/quarry` may import anything under `internal/`. **Nothing imports `cmd`.**
- `internal/cli` may import feature packages and `internal/platform/*`. A feature package
  **never** imports `internal/cli` — a feature that needs to write output takes an
  `io.Writer`.
- `internal/<feature>` may import `internal/platform/*` and the standard library. **A
  feature package does not import another feature package.** If two features need the same
  type, it belongs in `internal/platform/*` or in a small shared types package — never
  imported sideways.
- `internal/platform/*` imports only other platform packages and third-party libraries.
  Generic, no feature knowledge.

An import that violates this is a finding regardless of how convenient it is. The usual fix
is to move the shared thing down, or to invert it into an interface the consumer declares.

## Error handling at boundaries

- **Wrap with context, once per boundary**: `fmt.Errorf("load config %s: %w", path, err)`.
  Do not wrap the same error twice on the way out, and never wrap with a string that
  repeats what the inner error already says.
- **Sentinels and typed errors are the contract.** Export `ErrNotFound`-style sentinels (or
  a typed error) for conditions callers must branch on; document them on the function that
  returns them. Callers use `errors.Is` / `errors.As`, never string matching.
- **Exit codes are a user-visible contract.** They are decided in `cmd/quarry` by inspecting
  the returned error, not by calling `os.Exit` from deep in the tree. Nothing below `main`
  calls `os.Exit` or `log.Fatal`.
- Errors that reach the user are user-facing copy: lowercase, no stack trace, and they say
  what to do next.

## Go code conventions

- **Functional options** for construction; **interfaces defined at the point of
  consumption**, kept small.
- **No `I` prefix** on interfaces.
- Prefer **explicit, intention-revealing names**; extract repeated literals into **named
  constants** (TTLs, prefixes, limits).
- `context.Context` is the first parameter of anything that does I/O, and it is threaded
  all the way down. Never store one in a struct.
- Keep `internal/platform/*` generic and dependency-light; feature-specific logic belongs
  in the feature package.
- **Comments are written for `go doc`** — a package `doc.go`, contract-shaped function
  docs, no history. See *Documentation & comments* below.

## Documentation & comments (MANDATORY — write for `go doc`)

`go doc` output is the package's contract. It must convey *intention* — what a package
and each function is for — well enough that a reader (or a model) never has to open the
source. Comments that only make sense next to the diff that produced them are noise in
that output.

### 1. Every package has a `doc.go`

One `doc.go` per package, holding the package comment and nothing else (no code). Starts
`Package <name> ` and says what the package *does* and what it owns — not how it grew.

```go
// Package summarize condenses a source document into a brief: it selects the
// passages that carry the document's claims, orders them, and renders them in
// the requested output format.
//
// Input arrives as values, never as flags or an io.Reader the package opened
// itself, so every decision here is testable without a command line.
package summarize
```

### 2. Every exported symbol has a doc comment; unexported ones that carry a decision do too

- **Starts with the symbol name**, then a complete sentence: `// mintSession creates …`,
  not `// creates …` or `// This function creates …`. `go doc` prints it verbatim.
- States **what it does, what it returns, and what it refuses** — the contract a caller
  needs. Name the error sentinels and the conditions that produce them — in a clause
  each, not a paragraph each.
- **Cite the standard when the behaviour is one.** `RFC 6749 §5.1`, `OIDC Core §3.1.2.1`,
  `RFC 7009 §2.2` — section symbol included. A citation replaces a paragraph of
  explanation and is the one form of "why" that never goes stale.
- **Budget, and it binds:** exported func/type ~4 lines; unexported func/type 1–2 lines;
  `const`, `var`, sentinel error 1 line. Past the budget the text is *how* — the
  algorithm, a slot order, a walk through each branch — and *how* belongs in the body at
  the line it explains, not in the doc comment
  ([go.dev/doc/comment](https://go.dev/doc/comment), *Funcs*).
- **Each fact once, where the code enforces it.** A constraint shared by several symbols
  is one line beside the code that sets it, not restated on each.
- **No spec or finding ids** (`R7`, `BR-3`) — they point outside `go doc`. State the rule.
- **A fix makes a comment shorter or truer, never longer.**

```go
// Bad — 16 lines of doc on a 14-line function: slot order, what the mapper
// returns, every re-read branch. The slot-order fact is also restated on two
// other symbols.
// resolveTransferCancellation classifies a cancelled Transfer transaction.
// commitTransfer lists the witness slot before the debit slot in slots (R8),
// so mapTransactWriteError resolves a transaction cancelled by BOTH
// conditions to the witness's own sentinel, not errWriteContention -- ...
// (11 more lines)

// Good — the contract; the ordering note lives once, beside the slots literal.
// resolveTransferCancellation maps a cancelled transfer transaction to a
// store error. A failed witness check is ambiguous, so it re-reads:
// ErrInsufficientFunds if the debit would now overdraw the account,
// otherwise errWriteContention.
```

### 3. Never document history in a comment

Comments describe the code as it **is**. The following all belong in git, the PR, or a
report under `.claude/reports/` — never in a doc comment:

- `// Previously this …`, `// Used to …`, `// Changed to …`, `// Now that …`
- dated narrative: `// Until the 2026-09-10 audit …`, `// Phase 7 added …`
- review/finding ids as narrative: `(R1 finding)`, `(audit finding M5)`, `(D3)`,
  `SCENARIO-04` — the durable fact is the *rule*, so state the rule
- `// TODO(name)` describing a past decision rather than pending work

```go
// Bad — history; a reader learns about a diff, not about the function.
// renderBrief used to always emit markdown; since the M-series audit it
// honours --format (see the 2026-09-10 report).

// Good — the rule, stated as the contract it is.
// renderBrief writes b to w in the requested format. It returns
// ErrUnknownFormat for a format the renderer does not implement; callers
// validate the flag value before calling.
```

### 4. Inside a function, comment only the non-obvious *why*

Delete anything that restates the next line, narrates the flow, or records how the code
got here. What earns a comment inside a body:

- an **ordering constraint** or an invariant that is not visible locally
- a **fail-safe** choice whose opposite looks reasonable
  (`// A transport fault is not evidence the document is gone; keep the cached copy.`)
- a **spec requirement** that explains an otherwise odd branch, with its citation

```go
// Bad — apparent behavior; the code already says this.
// Loop over the sections and find the matching id.
for i := range doc.Sections {

// Bad — history.
// We moved this above the config lookup in the flag migration.

// Good — the constraint the reader cannot see.
// Resolve the path before the size check, so a symlink cannot report one
// file's size and open another.
```

### 5. Verify what the reader actually gets

`go doc` is cheap and is the check: read the package the way a caller will.

```bash
go doc ./internal/<name>            # exported surface + package doc
go doc ./internal/<name> <Symbol>   # one symbol
go doc -all ./internal/platform/<name>
```

If the output does not say what the package is for, or a function's line does not say
what it guarantees, the comment is wrong — not the reader.

Scope: production Go under `cmd/` and `internal/`. Test files follow the `go-testing`
skill (*Test comments*): the test name carries the rule under test, so a test comment
defaults to none and never exceeds two lines.

## Testing conventions (summary — defer to the `go-testing` skill for depth)

- **testify** (suite optional). Declarative tests, Given-When-Then separated by blank
  lines, no control flow in test bodies.
- **White-box** (`package foo`) tests are normal for exercising unexported logic; an
  external `foo_test` package is fine too.
- **In-memory adapters** (`memory.go`) for the persistence port; hand-written fakes or
  generated mocks for the other ports; `httptest` when an adapter speaks HTTP.
- **No injected clock.** Production calls `time.Now()` directly; time-dependent tests use
  `testing/synctest` bubbles (Go 1.27) to control and advance a fake clock. Do not thread a
  `func() time.Time` through production code.

## When adding a new feature package (checklist)

1. Create `internal/<name>/` with `doc.go` (package doc — see *Documentation & comments*).
2. Define the `Server` struct, its `Option`s and `WithX` helpers, and `NewServer`.
3. Declare the ports it consumes as small interfaces in that package; add the in-memory
   adapter alongside, plus the production adapter if it persists or talks to the network.
4. Keep the decision logic transport-free; wire the command surface in `internal/cli` and
   the concrete dependencies in `cmd/quarry`.
5. Check the dependency rule: does this package import another feature package? Move the
   shared thing down instead.
6. Tests per the conventions above.
