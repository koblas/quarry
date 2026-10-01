---
name: go-testing
description: Use whenever writing, modifying, or reviewing tests in this project. Defines the expected style for unit tests and command-level (CLI slice) tests. Go-specific — examples use testify, declarative style with tables for same-assertion cases, hand-written fakes, in-memory adapters, and synctest for time.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
paths: *.go
---

Go, `testing` + **testify** (`assert`/`require`; `suite` optional). Tests declarative; a table where cases share one assertion, separate functions otherwise. Project conventions enforced by `test-reviewer` agent.

## Black-box package by default (MANDATORY)

**Every test file MUST declare `package <name>_test`, not `package <name>`.**
Tests exercise the component through its _exported_ surface — the same surface a caller sees. That is the default. The reviewer **flags any `package <name>` (white-box) test file as a violation** unless justified (below).

```go
// Good — black-box: external package, public surface only.
package summarize_test

import "github.com/koblas/quarry/internal/summarize"

// Bad — white-box: same package, reaches unexported funcs. Requires justification.
package summarize
```

- **Why:** black-box tests can't bind to internals, so they survive refactors and document the contract. A test reaching an unexported func couples to a decision that is not in the contract.
- **Narrow exception (must be justified in-file):** white-box `package <name>` is permitted ONLY for **unexported decision logic extracted for combinatorial reasons** that can't be driven economically through the public surface. When taking the exception:
  - Keep the extracted func unexported — do **not** widen visibility for a test.
  - Put white-box tests in their own file (e.g. `parse_internal_test.go`) with a top-of-file comment naming the unexported unit and why the public surface won't do. Everything testable through the public API stays black-box.
- **Don't** reach for white-box just to read a private field or skip wiring — that's the violation the default exists to stop. Restructure for testability instead.

## Test structure (mandatory)

Every test follows **Given-When-Then**, separated by blank lines, no `// Given` comments.

### Test comments — the name is the documentation

A test's name already says what it pins. **Default: no comment.** Tests are where comments
should be tightest.

- **Allowed:** a setup constraint the name cannot carry — why this test needs a real
  directory and not the in-memory store, the full `run()` path and not a direct call, a
  field left unset rather than `""`. **Max 2 lines above the func, one fact per line; max 1
  line inside the body**, at the odd line it explains.
- **Never:** restating the name; narrating the setup step by step; explaining how the
  production code decides (ordering, parser internals — that is the production code's
  job); spec, finding or review ids (`R7`, `SCENARIO-04`, `REVIEW-03`); mutation evidence
  ("reddens when …" — it goes in the report / STATE.md handoff).
- **A fix makes a test comment shorter, never longer.**

```go
// Bad — 10 lines on an 11-line test: restates the name, explains how flag parsing
// orders defaults, cites what an unchanged sibling command does.

// Good
// Needs run(): calling the command directly skips flag validation.
// Path is unset, not "": the check is for presence, so "" would reach the handler.
func Test_check_without_a_path_is_refused_before_reading_files(t *testing.T) {
```

A test whose name says it all — `Test_status_reports_the_first_unticked_step_when_two_scenarios_are_open` — gets no comment.

- **Given/When/Then must trace cleanly.** Every value When/Then references must be explicit in Given. When a test queries or asserts by a specific id, exit code, etc., setup must put that value on the seeded data — don't rely on a factory default. If a value matters to an assertion, make it an explicit arg to the test-data builder; don't bury it as a default.

```go
// Bad — DOMAIN is silently the builder default; the test reads as if domain doesn't matter
seed(t, store, row(userID, withSource("rss")))
found := result.find(domain)

// Good — DOMAIN is explicit in both the seed and the assertion
seed(t, store, row(userID, domain, withSource("rss")))
found := result.find(domain)
```

- Black-box `package <name>_test` default (see **Black-box package by default** above) — exercise through the exported surface.
- Use the minimum fixture/input needed to prove the behavior; remove records irrelevant to the assertion.
- When behavior is provable with 1-2 domain values, don't use larger reference datasets.
- Prefer shared constants for recurring domain values over ad-hoc literals.
- **Construct the system-under-test via a `newXxx(t *testing.T)` helper called at the top of each test** — the Go analog of before-each. Shared fakes / `Server` / stores _every test uses identically_ are built here. Never package-level `var`s (shared mutable state across tests). With `suite`, that's `SetupTest`.
- **Seed test data inside the test body, never in a helper/SetupTest.** Data setup (`store.save(...)`, `fake.add(...)`) lives in each test so the test is self-contained.
- **Prefer enriching the fake over building a bespoke inline mock.** When a test needs a port to fail for one scenario, give the project's fake a small field + method (e.g. `failNext(err)`); the fake is still built in the helper, and the `failNext(...)` call is per-test data setup in the test body.

```go
// In the fake (a hand-written fake implementing the Store port):
type fakeFooStore struct {
    foos    map[string]*Foo
    nextErr error // set by failNext for one scenario
}

func (f *fakeFooStore) failNext(err error) { f.nextErr = err }

func (f *fakeFooStore) Find(ctx context.Context, id string) (*Foo, error) {
    if f.nextErr != nil {
        return nil, f.nextErr
    }
    return f.foos[id], nil
}

// In the test:
func Test_returns_error_when_the_lookup_fails(t *testing.T) {
    srv := newFooServer(t)
    srv.store.(*fakeFooStore).failNext(errBoom)

    _, err := srv.Run(t.Context(), userID)

    require.ErrorIs(t, err, errBoom)
}
```

```go
func Test_returns_expected_section_count_when_the_document_has_headings(t *testing.T) {
    srv := newSummarizeServer(t, withDocument(shortDoc, longDoc))

    result, err := srv.Summarize(t.Context(), request)

    require.NoError(t, err)
    assert.Equal(t, 2, result.TotalSections)
}
```

## Mandatory Review

**Every new or modified test must be reviewed by the `test-reviewer` agent.**

## Naming

- Test function: `func Test_<behavior_in_camel_case>(t *testing.T)` or `func Test<Unit>_<behavior>(t *testing.T)` — must start with `Test`. Subtests (`t.Run("...")`) use a natural-language string.
- Failure scenarios prefer `FailsWhen<condition>` / `ReturnsErrorWhen_…` over `PanicsWhen…`.
- Command tests prefer `Returns<exit_code>When<condition>` or `Prints<what>When<condition>`; HTTP handler tests prefer `Returns<status>When<condition>`.
- **Use plain business language, not invented jargon.** Avoid verbs the domain doesn't use (`credits`, `honors`, `respects`). Prefer `ignores…`, `returns…`, `saves…`, `rejects…`.
- **Name the behavior, not the mechanism.** The name should communicate the observable outcome from the caller's view, not the internal implementation.
  - **Exception: when the mechanism IS the guarantee, name it.** A cache is internal — but if "don't refetch on second call" is the contract, name it (`DoesNotRefetchOnSecondCall`).
  - **Incidental → drop it:** `FailsWhenTheIndexLookupErrors` leaks an internal table; the contract is `ReturnsNotFoundWhenTheDocumentIsUnknown`.

## Logic in Tests (Forbidden)

**Never use `if`, `else`, `while`, `switch`, in a test body.**
Tests stay declarative and linear. If branching seems necessary, split the scenarios or redesign the setup.

`for` is deliberately absent from that list — a table's loop is the one permitted form, because it carries no decision. A `for` that chooses what to assert is the forbidden shape wearing a loop.

## Table tests (use them to kill boilerplate)

**Use a table when the cases share one assertion and differ only in input and expected value.** Seven functions whose bodies are the same three lines with a different string literal are worse than one table: the rule is spread across seven names, and adding the eighth case means copying the boilerplate again. Collapse them.

```go
// Good — one assertion, eight inputs. The rule reads in one place.
func Test_CountLines(t *testing.T) {
    cases := []struct {
        name string
        body string
        want int
    }{
        {name: "empty body", body: "", want: 0},
        {name: "only a newline", body: "\n", want: 1},
        {name: "one line, trailing newline", body: "a\n", want: 1},
        {name: "two lines, no trailing newline", body: "a\nb", want: 2},
        {name: "CRLF counts as LF", body: "a\r\nb\r\n", want: 2},
    }

    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            assert.Equal(t, c.want, markdown.CountLines(c.body))
        })
    }
}
```

Rules:

- **`t.Run` with a per-case name, always.** A failure must name the case, not a row index. The name states the rule that case pins (`"CRLF counts as LF"`), not the literal (`"a\r\nb\r\n"`).
- **One assertion shape for the whole table.** Cases asserting different tuples are different behaviors — that is *One behavior per test* violated, with a table as the disguise. Keep those as separate functions. A table of "these all return false" is right; a table mixing "returns false" with "returns 3 and the text `two`" is not.
- **Same tuple shape is necessary, not sufficient: the cases must also be one behavior family.** A table collapsing genuinely distinct production paths under one boolean — "input missing" beside "input malformed" beside "input violates an invariant" — passes every other rule here and is still *One behavior per test* violated. The test is whether the cases are the same rule's inputs or different rules sharing a return type. Seven reasons a parser finds no open item is one family; syntax validity beside semantic validation beside defaults-filling is three.
- **No logic in the case struct.** No `wantErr bool` driving an `if` in the loop body, no optional-field branching. Two assertion shapes means two tables, or two functions.
- **Every case must discriminate on its own.** This is the failure mode that makes a table worse than what it replaced: cases that all pass for one reason, so the table proves one thing while claiming to prove seven. Verify by mutation — break the rule one case exists for, and **exactly that subtest** must redden. If breaking one guard reddens four cases, they were never independent; if it reddens none, the case was decorative. Drop decorative cases rather than keeping them for symmetry.

  Unlike the three rules above it, this one **cannot be checked by reading a diff** — it needs production code broken and the suite run. It is discharged the way `.claude/briefs/proof.md` already requires of any mutation claim: the author states which mutation was run and which subtest it reddened, and the reviewer checks that report. A table arriving with no such statement has not satisfied this rule; it has skipped it.

**Do not table** setup-heavy scenarios, cases needing different fakes or fixtures, or anything where the "table" becomes a config object with a branch per field. When the struct grows a field only some cases use, the table has stopped reducing boilerplate and started hiding it.

## One behavior per test

Each test verifies one behavior. If the test name needs "and", split it. Multiple `assert`/`require` calls each proving a _different_ behavior (not different facets of the same outcome) is the same smell.

- **Watch the seed shape, not just the assertions.** A test can name one rule but exercise two if the seed is shaped for both. A test named "returns distinct user IDs" seeding three rows for user A and one for B exercises both _dedup_ and _multi-user enumeration_; a single `Len(…, 2)` does work for both. Split. Heuristic: if removing one _type_ of seed variation still proves the named rule, that variation was testing a different rule.

```go
// Bad — one test, two rules. Len(ids, 2) does work for both dedup AND enumeration.
func Test_returns_distinct_user_ids_of_users_with_non_deleted_domains(t *testing.T) {
    seed(t, store,
        row(userA, withDeletedAt(nil)),
        row(userA, withDeletedAt(nil)),
        row(userA, withDeletedAt(nil)), // over-specified — 2 rows is enough for dedup
        row(userB, withDeletedAt(nil)),
    )

    ids, err := finder.FindAll(t.Context())

    require.NoError(t, err)
    assert.Len(t, ids, 2)
    assert.ElementsMatch(t, []string{userA, userB}, ids)
}
```

```go
// Good — split. Each test seeds the minimum for the single rule it names.

func TestReturnsAUserIdOnlyOnceWhenTheUserHasMultipleNonDeletedDomains(t *testing.T) {
    seed(t, store,
        row(userA, withDeletedAt(nil)),
        row(userA, withDeletedAt(nil)),
    )

    ids, err := finder.FindAll(t.Context())

    require.NoError(t, err)
    assert.Equal(t, []string{userA}, ids)
}

func Test_returns_one_user_id_for_each_user_with_at_least_one_non_deleted_domain(t *testing.T) {
    seed(t, store,
        row(userA, withDeletedAt(nil)),
        row(userB, withDeletedAt(nil)),
    )

    ids, err := finder.FindAll(t.Context())

    require.NoError(t, err)
    assert.ElementsMatch(t, []string{userA, userB}, ids)
}
```

## Designing the test list (ZOMBIES + mutation check)

Before writing tests, walk the **ZOMBIES** categories explicitly — systematic, not "interesting":

- **Z**ero — empty / nil / no-result input
- **O**ne — exactly one item (catches inversions and missing filters)
- **M**any — N>1: all-same, all-different, mixed
- **B**oundary — min/max, off-by-one, time-window edges
- **I**nterface — contract shape (types, fields, optional/required)
- **E**xceptions — errors, infrastructure faults, error sentinels, exit codes
- **S**imple — keep each scenario minimal

For every test ask the **mutation question**:

> "If I flip an operator (`>` ↔ `<`, `== 1` ↔ `== 0`, `&&` ↔ `||`) or drop a
> filter clause in production, would this test catch it?"

If you can't name a mutation the test rules out, it is vacuous — redesign or delete it.

### Asymmetric data for discriminating filters

A test on a filtering/aggregating function must use data that distinguishes the correct implementation from likely mutants. Symmetric data (one matching + one non-matching, asserting `== 1`) is a smell — the correct predicate and its inversion both yield 1. Pick counts that diverge.

```go
// Bad — 1 matching + 1 non-matching, predicate `flagged`: count is 1 either way.
seed(t, store, row(withFlagged(true)), row(withFlagged(false)))
assert.Equal(t, 1, query.Count(t.Context()))

// Good — 2 matching + 1 non-matching: correct predicate yields 2, mutation yields 1.
seed(t, store, row(withFlagged(true)), row(withFlagged(true)), row(withFlagged(false)))
assert.Equal(t, 2, query.Count(t.Context()))
```

Reference: James Grenning, ["TDD Guided by Zombies"](https://blog.wingman-sw.com/tdd-guided-by-zombies).

## Assertions

- **testify**: `require` for a fatal precondition (stop the test — e.g. `require.NoError(t, err)` before dereferencing a result); `assert` for the rest. Argument order: `assert.Equal(t, expected, actual)`.
- **Precision-sensitive values**: use `assert.InDelta` / `InEpsilon` ONLY for values production legitimately makes fuzzy (currency rounding, accumulated float math). For anything you control, pick fixtures whose math resolves to exact integers and use `assert.Equal`. A tolerance assertion over a fixture you chose to be fractional is a smell.
- Avoid magic numbers; use named constants where meaning matters.
- **No redundant intermediate assertions.** Don't assert a precondition already implied by the next assertion (e.g. asserting `ok` from a map lookup when the next line asserts the value — the test fails anyway if it is absent). Exception: `require` guards preventing a nil-deref panic are fine.
- **Subset matchers are NOT equality.** `assert.Contains` / `assert.Subset` pass when items are _included_ — extras slip through. For "exactly these in any order" use `assert.ElementsMatch`; for exact ordered equality use `assert.Equal`.

## Test data minimality

- Smallest input/fixture set proving the behavior.
- Oversized datasets where fewer records assert the same rule are a violation.
- Repeated raw domain literals → shared constants when values recur across tests.

## Repeated construction = extract a helper

- When the same construction (`Server`, fake, fixture) appears identically in 3+ tests or across 2+ files, extract a `newXxx(t *testing.T, opts...)` helper or fixture builder.
- The helper absorbs incidental params (fakes, temp dirs) so each test specifies only what matters to its scenario; use functional options on the helper for per-test knobs.
- **Prepare the seam before changing signatures.** Before adding a parameter to a constructor called in many tests, extract/extend the helper first — one edit, not shotgun surgery. "Make the change easy, then make the easy change." — Kent Beck

## Test data visibility

- **All test data referenced in assertions must be visible in the test body.** Package-level `var`s building test data used implicitly by tests are a violation — the reader shouldn't scroll to package scope to understand an assertion. Pass data via a helper or use named constants.
- **Don't assert on internal-signal return types.** If a return value is only consumed internally, don't write a test that only asserts on it; behavioral tests already prove it.
- **"Unchanged" assertions must use distinct before/after values.** When asserting "data unchanged after operation," the two sides must have visibly different identifiers, else the test passes vacuously.

## Test behavior, not library boundaries

- **When a library implements your product behavior, the behavior is still yours to test.** The library is an implementation detail, not an excuse to skip.
- **If a behavior is hard to test, restructure for testability first** — split the pure decision out of the delivery layer. Most "untestable" behaviors are a design signal.
- **Delete vacuous tests.** A test passing regardless of correctness is worse than none. If you can't make it fail by removing the behavior, delete it.

## Testing Strategy & Efficiency

**Prefer fast, economical, deterministic tests.** Before reaching for slow or non-deterministic dependencies (disk, env vars, subprocesses, network, real HTTP), exhaust the cheaper options:

- **Unit tests** of pure funcs (parsers, decision logic, formatters) when combinatorial complexity makes going through the command impractical. Otherwise test through the command.
- **In-memory adapters** — the package's own `memory.go` Store — instead of a real backend.
- **Narrow command tests** at the delivery boundary, faking only the ports the command reaches.
- `t.TempDir()` over a hand-rolled temp directory; it is cleaned up for you and is unique per test.
- **Never shell out to the built binary.** `run(ctx, args, stdout, stderr)` is callable in-process; a subprocess test proves the same thing an order of magnitude slower and hides the panic.

## Fakes over mocks (default)

- **Hand-written fakes for every port.** The `Store` interface gets an in-package fake (or reuse the real `memory.go` adapter). A fake commonly embeds the interface and overrides only the few methods exercised, so unimplemented methods panic if called unexpectedly:

```go
// Embed the interface, override what the test exercises. An unexpected call to
// anything else panics (nil method) — which is the signal you want.
type fakeFetcher struct {
    Fetcher // embedded — unimplemented methods panic
    docs map[string]Document
}

func (f *fakeFetcher) Get(_ context.Context, id string) (Document, error) {
    d, ok := f.docs[id]
    if !ok {
        return Document{}, ErrNotFound
    }
    return d, nil
}
```

- **Test the `Server` concretely**, not behind an interface introduced only for tests. Don't widen visibility for tests, and don't drop to white-box `package foo` to reach internals — black-box `package foo_test` against the exported surface is the default (see **Black-box package by default**).
- **Fresh instances per test** via the `newXxx(t)` helper. No `Reset()`/`Clear()` methods on fakes — construct a new one. For real resources needing teardown, use `t.Cleanup`.

```go
type fakeGuestStore struct{ guests []Guest }

func (f *fakeGuestStore) FindAll(context.Context) ([]Guest, error) {
    return append([]Guest(nil), f.guests...), nil
}
```

## Response sequencing for external-call fakes

- **One fake per port, with response sequencing** — a slice of responses (or a call counter) the fake plays back in order. Do NOT create separate fake types for success / error / timeout (`throwingClient`, `failingClient`, `spyClient`) — unify them into one configurable fake. Model variants with a small response struct (`type resp struct { out *Foo; err error }`) and a `[]resp` the fake advances through.
- **Auto-advance**: each call consumes the next response; the last repeats once exhausted. No manual `advance()`.

## Command tests (slice standard)

The command surface is tested by calling the command entrypoint in-process with explicit args and buffers, then asserting on the returned error and the captured streams. Assert the **error or exit classification first**, then the output.

```go
func TestReturnsAnErrorWhenTheInputFileIsMissing(t *testing.T) {
    var stdout, stderr bytes.Buffer

    err := cli.Run(t.Context(), []string{"summarize", "missing.md"}, &stdout, &stderr)

    require.ErrorIs(t, err, cli.ErrUsage)
    assert.Empty(t, stdout.String())
}
```

```go
func TestPrintsTheBriefWhenTheDocumentParses(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "doc.md")
    require.NoError(t, os.WriteFile(path, []byte(validDoc), 0o600))

    var stdout, stderr bytes.Buffer
    err := cli.Run(t.Context(), []string{"summarize", path}, &stdout, &stderr)

    require.NoError(t, err)
    assert.Contains(t, stdout.String(), "expected heading")
}
```

For pure decision logic, prefer calling the extracted pure func directly — faster and clearer than routing through argument parsing.

**If and when `quarry` grows an HTTP surface**, mount the handler under `httptest.Server` and issue a real request, asserting HTTP status first, then body/headers. Those dispatch tests bind real sockets and therefore cannot run inside a `synctest` bubble.

## Input validation matrix (what to cover)

For any command that accepts input, or any mutating HTTP endpoint if one exists:

- Happy path (exit 0 / `200`–`204`).
- Malformed input or parse error → usage error / `400`.
- Missing required argument or flag → usage error / `400`.
- Domain invariant violation → domain error with its own exit classification / `400` or `409`.
- Referenced resource not found → not-found error / `404`.
- Unexpected infra/runtime failure where applicable → internal error / `500`.

Flag parsing rejects some inputs before your code runs — test that boundary too (an unknown flag is the flag package's error, not yours, and the message the user sees is still a contract).

## Async / time / concurrency tests

- **No real `time.Sleep` for waiting, no injected clock.** Production calls `time.Now()`; time-dependent tests run inside a Go 1.27 `testing/synctest` bubble and advance the fake clock with `time.Sleep` + `synctest.Wait`.
- **Allocate backends with background goroutines OUTSIDE the bubble.** Anything that spawns its own goroutines at construction (a cache with background eviction, an embedded server) must be constructed in the outer test body and passed in, else the bubble never reaches "all durably blocked".

```go
func TestSessionExpiresAfterMaxAge(t *testing.T) {
    cache, err := newEvictingCache(1024)   // built OUTSIDE the bubble (background goroutines)
    require.NoError(t, err)

    synctest.Test(t, func(t *testing.T) {
        srv := newServerWith(cache)
        sess := mustCreateSession(t, srv)

        time.Sleep(2 * time.Hour) // fake clock — instant
        synctest.Wait()

        _, ok := srv.activeSession(t.Context(), sess.ID, /*maxAge*/ 3600)
        assert.False(t, ok)
    })
}
```

- **Formatted local dates pin the zone.** Test asserting a date or time rendered in local time sets `time.Local` to a fixed zone with `t.Cleanup` restore (no `t.Parallel`), and uses an instant whose local and UTC dates differ. Unpinned, test passes or fails by machine.
- A library that itself reads the wall clock must stay on real time — do NOT bubble it, else the code under test desyncs from the library's window. Tests that bind real sockets also stay real-time; they assert routing, not time progression.

## Test file size & grouping

- Files over ~300-400 lines covering unrelated behavior should be split by behavior/command.
- Tests live at the package's public boundary in `package <name>_test` (default — see **Black-box package by default**). White-box `package <name>` is a justified exception, only for extracted unexported decision logic.

## Backend integration tests (real filesystem / external service, contract-style)

- Use the cheapest real thing: `t.TempDir()` for the filesystem, `httptest.NewServer` for an HTTP dependency. A container is a last resort and must be justified.
- Verify the CRUD contract: save/read, list-empty/list-populated, update, delete-existing, delete-missing idempotence.
- These are slower; gate their breadth — prove the storage contract here, prove business rules against the in-memory adapter.

## Contract tests for the Store port

- A `Store` with both a `memory` adapter and a real adapter should share one contract test: `func testStoreContract(t *testing.T, mk func() Store)` exercised once per adapter. Both must pass identically — that's what lets tests trust the in-memory fake to stand in for production.

## What to test

For **commands / Server methods**: happy path, empty results, validation + edge cases, error handling (correct sentinel and exit classification), delegation to ports, persisted-object shape, and what lands on stdout vs stderr.

For **pure funcs** (decision logic, parsers, formatters): the full ZOMBIES set.

For **mappers / format conversion**: field-mapping + value-conversion correctness.

## What NOT to test

- Unexported funcs _directly_ when already covered through a command or pure entry point (extract + test directly only for combinatorial explosion; keep the extracted thing unexported).
- Trivial getters/setters with no logic.
- Framework/library internals.
- A `Server` behind a test-only interface — test it concretely.
