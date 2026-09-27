# Refactor Catalog

Language- and codebase-independent catalog of code smells + refactorings that fix them. Each entry same structure so refactor-advisor agent match smell against known pattern, propose fix.

Add new entry when refactoring session surface recurring smell not yet listed. Include concrete example from current codebase.

---

## Entry format

```
## <Pattern name>

### Smell
What the bad code looks like and why it hurts.

### Trigger
The specific moment when the smell becomes undeniable (usually: "we had to add a second/third X").

### Refactoring
Step-by-step transformation from the smelly code to the clean design.

### Structure after refactoring
The types / abstractions that emerge and their responsibilities.

### Tests
How to test the result. What the test suite looks like after the refactoring.

### Example
Pointer to a real codebase where this was applied.
```

---

## Specification (Criterion) pattern

### Smell

Use case/service piles up `if` branches deciding which records keep, mixed with orchestration logic (load data, sort, paginate). Every new filter rule modifies same use case.

### Trigger

Second independent filtering rule added. Inline conditionals grow, changes keep hitting class that should stay stable.

### Refactoring

1. Extract `Criterion<T>` interface, one method: `boolean matches(T item)`.
2. Implement one class per business rule. Each class owns only data it needs.
3. Introduce `Query` object bundling criteria + sorting/paging inputs.
4. Rewrite use case pipeline:
   - caller builds criteria list from current request/state
   - use case delegates to repository with `Query`
   - repository (or use case) applies `criteria.stream().allMatch(c -> c.matches(item))`
5. New rule = new `Criterion` class; existing use case code stays unchanged.

### Structure after refactoring

- `Criterion<T>` interface
- One `*Criterion` class per rule
- `Query` value object (`List<Criterion<T>>`, sort/pagination hints)
- Use case focused on orchestration
- Caller (controller/application service) assembles query

### Tests

- Unit tests per criterion: `true`, `false`, edge-case behavior.
- Use case tests verify orchestration wiring, not rule internals.
- Caller tests verify query assembly from request/state.

### Example

**Allocation API**
Filter candidates by threshold, date window, status.

- `AllocationCriterion` with `ThresholdCriterion`, `DateRangeCriterion`, `StatusCriterion`
- `AllocationQuery(criteria, sortOrder)` passed to allocation use case
- Controller/application layer builds criteria from request filters
- After refactor: use case stays stable while rules evolve independently

---

## Anemic domain model to rich model

### Smell

Domain entities/records just data containers — business rules + state transitions live elsewhere: controllers, use cases, mappers, **or standalone domain services ("calculators", "evaluators", "\*Service" classes) operating on single domain type, holding no state of own**. Rules get duplicated, easy to bypass, entity can't defend own values.

Frequent variant in TDD/Clean-Architecture codebases:

- Entity exported as `type X = { readonly a: A; readonly status: S; ... }`.
- Sibling file `XCalculator.ts` exports free function `calculateStatus(parts): S` deriving one of `X`'s own fields from others.
- Callers must remember calling calculator; nothing stops constructing `X` whose `status` field disagrees with `parts`.

### Trigger

Same domain rule appears multiple places (e.g. create + update flows), or new behavior needs touching several orchestration classes to keep invariants consistent.

**Also trigger when** standalone "Calculator", "Evaluator", "Resolver", or "\*Service" file in domain layer:

- exports single pure function (or stateless class, one method),
- takes one domain type (or tuple of its fields) as input,
- returns value that _is, or directly derives,_ one of that type's own fields, and
- has no second implementation, no port, no collaborators.

That's entity method masquerading as service. Behavior belongs on entity (constructor, factory, or method) so type can't be constructed inconsistent.

### Refactoring

1. Identify domain invariants/behaviors currently implemented outside domain.
2. Move invariant enforcement into constructors/factories/value objects.
3. Move business operations into domain methods instead of rebuilding raw objects in controllers.
4. Keep controllers/use cases focused on orchestration (I/O, lookup, transaction boundaries).
5. Replace primitive parameters with value objects when rules non-trivial.
6. Keep only simple mapping/transport concerns outside domain.

### Structure after refactoring

- Entities/value objects own invariants + behavior.
- Use cases orchestrate collaborators, not business calculations/rules.
- Controllers map external input/output only.
- Invalid states become unrepresentable or fail fast at domain boundaries.

### Tests

- Domain tests validate invariants + business transitions directly.
- Controller/use case tests assert orchestration + status mapping, not duplicated rules.
- Add regression tests proving rules can't bypass through different entry points.

### Example

**Order CRUD API**

- Keep required-field + currency invariants in domain entity.
- Prefer adding domain behaviors as business rules grow, rather than spreading logic
  across create + update controllers.
- Keep API tests focused on contract + delegation; keep domain rule details in domain tests.

---

## Architecture guardrail sweep

### Smell

Code passes functional tests but architecture drifts: layer boundaries blur, validation duplicated across adapters + core, acceptance tests stop running in CI, business policy hard-coded in use cases.

### Trigger

Review finds behavior correct today, but extension risk rises — next rule change would need touching multiple layers or changing core code for policy-only updates.

### Refactoring

1. Run focused review pass, these rules:
   - layer dependency boundaries (no inward leaks from adapters/frameworks)
   - validation ownership + consistency (single authoritative boundary per invariant)
   - test lifecycle coverage (unit + integration + acceptance wired intentionally)
   - policy configurability (hard-coded thresholds/tiers extracted behind config/policy objects)
2. Classify findings by severity (`VIOLATIONS`, `WARNINGS`, `GOOD PRACTICES`) to prioritize work.
3. Refactor only highest-impact guardrail breaks first, preserve behavior with regression tests.
4. Encode recurring findings in catalog entries so future reviews faster + consistent.

### Structure after refactoring

- Clear adapter/application/domain boundaries, explicit dependency direction.
- Invariants enforced once at deliberate boundary, adapters map errors consistently.
- CI lifecycle intentionally includes chosen test layers.
- Business policy represented as configurable input or dedicated policy abstraction.

### Tests

- Keep use case tests for rule behavior.
- Keep controller tests for API contract + validation mapping.
- Ensure acceptance tests either wired in CI or explicitly documented as manual.
- Add regression tests when moving validation/policy ownership to prevent behavioral drift.

### Example

**Hotel Room Allocation API**

- Validation overlap found between controller input validation + domain constructor.
- Acceptance test exists but excluded from CI lifecycle.
- Premium threshold hard-coded in use case instead of configurable.

---

## Formatting logic in domain entities

### Smell

Domain entity has methods formatting data for display (e.g., `displayLabel()`,
`formattedAddress()`, `shareText()`). Methods assemble human-readable strings using
domain fields but serve no domain invariant/business rule. Couples domain to
presentation concerns — locale, label conventions, abbreviation rules — that change for
UI reasons, not business reasons.

### Trigger

Second display context appears (e.g., share sheet alongside detail screen) needing
different format from same entity. Entity accumulates formatting variants, or same
conditional logic (e.g., which address field to prefer) appears in multiple methods.

### Refactoring

1. Identify methods on domain entities producing display strings (not enforcing invariants).
2. Create `*TextFormatter` object/class in presentation layer.
3. Move formatting methods to formatter as static/companion functions taking entity
   as parameter: `OrderTextFormatter.summaryLine(order)`.
4. Extract shared derivation logic as private helper inside formatter.
5. Update all call sites in presentation/ViewModel code.
6. Move tests from domain test directory to presentation test directory.

### Structure after refactoring

- Domain entity: only invariant enforcement, domain queries, equality. Zero string
  formatting.
- `*TextFormatter` in presentation: owns all display-string assembly. One place to change
  when label conventions evolve.
- Dependency direction preserved: formatter depends on domain, not reverse.

### Tests

- Formatter tests live in presentation test directory, exercise each formatting method
  with boundary inputs (missing optional fields, equal fields, differing fields, etc.).
- Domain entity tests only cover invariants + domain behavior.
- All existing assertions preserved — only call site changes.

### Example

**E-commerce order system**
`Order.summaryLine()`, `Order.shippingLabel()`, `Order.receiptText()` were display-string
methods on domain entity. Moved to `OrderTextFormatter` in presentation layer. The
`recipientName()` helper became private function in formatter. Domain `Order` kept
only `total()`, `isShippable()`, invariant enforcement.

---

## Shotgun surgery

### Smell

Change to one concept (adding parameter, renaming field, changing type) needs
identical edits in many files. Knowledge of how to construct/configure that concept spread
across every call site instead of living in one place.

Common forms:

- Same constructor call (same args) copy-pasted across 5+ test methods or
  multiple test files.
- Data class gains field, dozens of call sites need updating.
- Factory/formatter instantiated inline in every ViewModel test with identical config.

### Trigger

Signature change touches 3+ files with identical edits, or doing
find-and-replace across test files to add same parameter everywhere.

### Refactoring

1. **Before feature change**, scan call sites for constructor/method being modified.
2. If 3+ sites use identical construction, extract shared helper first:
   - Test fixtures: `aFestival(...)`, `testCardFormatter()`, `successState()`
   - Test render helpers: `renderDetailScreen(uiState = ...)`
   - Production factories: Koin module, companion factory method
3. Verify all tests still pass (extraction pure refactor — no behavior change).
4. **Then** make actual feature change — lands in one place (helper), not N call sites.

"Make the change easy (this might be hard), then make the easy change." — Kent Beck

### Structure after refactoring

- One shared helper/fixture per repeated construction pattern.
- Call sites specify only what differs from defaults.
- Adding new parameter to underlying constructor = editing one helper, not N files.

### Tests

- Extraction step green-to-green: all existing tests pass before + after.
- Feature change modifies only helper — existing tests continue pass/fail based
  on own assertions, not incidental constructor noise.

### Example

**WaWo Android app**
`FestivalCardFormatter(untilTemplate = "Bis %s", endsTodayLabel = "Endet heute")` was
constructed identically in 6 ViewModel test files. Adding `festivalImage` needed touching
all 6. Extracted `testCardFormatter()` fixture — `festivalImage` parameter change became
one-liner.

---

## Duplicated sealed-class dispatch composable

### Smell

`when` block dispatching on sealed class/interface copy-pasted across multiple
composable functions. Each copy renders same variants with same parameters (image URL,
placeholder resource, content scale, error drawable) but slightly different modifier
context. Adding new variant or changing shared behavior (e.g., adding `placeholder`
parameter to `AsyncImage`) needs identical edits in every copy.

### Trigger

Same sealed-class `when` dispatch appears in 3+ composable files, or change to one
variant (e.g., adding error-placeholder to `Remote`) must replicate across all copies.

### Refactoring

1. Extract shared `@Composable` function taking sealed type + `Modifier`,
   dispatches via `when`, renders each variant.
2. Shared composable owns all variant-specific details (placeholder drawable, content
   description, content scale default).
3. Replace every inline `when` block with call to shared composable, pass only
   sealed value + caller-specific modifier.
4. If caller needs non-default `contentScale`, expose as optional parameter with
   sensible default.

### Structure after refactoring

- One `FestivalImageView(source: FestivalImageSource, modifier, contentScale)` composable in
  `presentation/common/`.
- Callers pass `source` + `modifier` only.
- Adding new `FestivalImageSource` variant compiles-fails in one place, not N.

### Tests

- Existing Compose UI tests continue pass unchanged (extraction pure refactor).
- No new unit tests needed — shared composable thin rendering wrapper, tested
  transitively by each screen's UI tests.

### Example

**WaWo Android app**
`when (imageSource) { Remote -> AsyncImage(...); Placeholder -> Image(...) }` was duplicated
in `FestivalSearchCard`, `FestivalDetailHeroSection`, `FestivalMapInfoSheet`, `HomeScreen`.
Extracted to `FestivalImageView` in `presentation/common/`. All four callers
now delegate to shared composable. Adding `placeholder = painterResource(...)` to the
`Remote` branch needed changing one file instead of four.

---

## Guard clauses

### Smell

Function uses nested `if/else` blocks or assigns local variable through conditional
branches before reaching "real work." Happy path buried inside indentation, reader
must mentally track which conditions lead to which outcomes. Error/edge-case
handling interleaved with main logic instead of dispatched up front.

### Trigger

Function has 2+ nesting levels for validation/precondition checks, or main
logic sits inside `if` block whose `else` is error/throw.

### Refactoring

1. Identify each precondition/invalid-state check in function.
2. Invert condition, return/throw immediately (early exit).
3. Remove `else` branch — rest of function _is_ happy path.
4. Function now reads top-down: guards first, then linear main logic at base
   indentation level.

### Structure after refactoring

- Each guard clause is one-liner `if (!condition) throw/return`.
- Guards appear at top of function, cheapest-to-check first.
- Main logic follows at same indentation level — no nesting.

### Tests

- No behavioral change — tests stay green throughout.
- Each guard maps to test triggering that specific early exit.

### Example

**Diagnostics report schema versioning (TypeScript)**
`parseDiagnosticsReportEnvelope` originally used nested `if/else` blocks for validation.
Refactored to sequential guard clauses:

```typescript
if (!isPlainObject(parsed)) throw new Error(MALFORMED);
if (!hasNumericSchemaVersion(parsed)) throw new Error(MALFORMED);
if (isCurrentSchemaVersion(parsed)) return extractV1UserAgents(parsed);
throw new UnsupportedSchemaVersionError(parsed.schemaVersion);
```

Each line self-contained decision. Function reads as checklist.

---

## Extract named conditions

### Smell

Boolean expression appears inline in `if` statement or ternary. Expression uses
low-level checks (`typeof`, `in`, `===`, `instanceof`) whose combined intent isn't obvious
without reading every operand. Readers must reverse-engineer meaning of condition
from how it's computed.

### Trigger

`if` condition spans multiple lines, combines 2+ operators, or needs comment
to explain purpose. Or same compound check appears in more than one place.

### Refactoring

1. Extract condition into named function/variable stating business
   or structural intent (e.g., `hasNumericSchemaVersion`, `isExpired`, `isEligibleForDiscount`).
2. Use type-narrowing return type when language supports it (TypeScript `is`, Kotlin
   smart cast) so subsequent code benefits from narrowed type.
3. Replace inline expression with call to named function.
4. If condition used once + short, `const` with descriptive name sufficient;
   function preferred when type narrowing needed or reuse likely.

### Structure after refactoring

- Each `if` reads as domain/structural assertion: `if (!isPlainObject(parsed))`.
- Named predicates live as private helpers near function that uses them.
- Type guards carry narrowing information so callers don't need follow-up casts.

### Tests

- Pure refactor — existing tests stay green.
- If extracted predicate non-trivial, consider focused unit test.

### Example

**Diagnostics report schema versioning (TypeScript)**

```typescript
// Before
if (!('schemaVersion' in record) || typeof record.schemaVersion !== 'number') { ... }

// After
function hasNumericSchemaVersion(
  record: Record<string, unknown>,
): record is Record<string, unknown> & { schemaVersion: number } {
  return 'schemaVersion' in record && typeof record.schemaVersion === 'number';
}
if (!hasNumericSchemaVersion(parsed)) { ... }
```

`if` now reads as structural assertion. Type narrowing flows into subsequent code.

---

## Compose method

### Smell

Function long, mixes multiple abstraction levels: low-level mechanics (parsing,
casting, null-checking) alongside high-level decisions (dispatching on version, selecting
strategy). Reader must constantly shift gears between _what_ function does
and _how_ it does each step. Function hard to scan — outline (sequence of steps)
buried in implementation details.

### Trigger

Function exceeds ~15 lines, or 2+ distinct "phases" identifiable within it (validate,
transform, dispatch) each involving own low-level logic.

### Refactoring

1. Identify high-level steps function performs (e.g., parse → validate → dispatch).
2. Extract each step into named private function describing _what_ it does, not
   _how_: `extractV1UserAgents(record)`, `assertWellFormed(condition)`.
3. Composed function becomes short sequence of calls at single abstraction level —
   table of contents for algorithm.
4. Each extracted function owns one concern, understood independently.
5. Keep extracted helpers private/local unless reuse proven.

### Structure after refactoring

- Public function reads like pseudocode: 5–10 lines, each named step.
- Private helpers contain mechanical details (type checks, casts, error construction).
- Each helper has clear input → output contract.

### Tests

- No behavioral change — existing tests stay green.
- Helpers tested indirectly through composed function; extract dedicated test only
  when helper has complex branching worth pinning independently.

### Example

**Diagnostics report schema versioning (TypeScript)**
`parseDiagnosticsReportEnvelope` refactored from single function with inline validation
into composed method:

```typescript
export function parseDiagnosticsReportEnvelope(json: string): UserAgentChecks {
  const parsed: unknown = JSON.parse(json);
  if (!isPlainObject(parsed)) throw new Error(MALFORMED);
  if (!hasNumericSchemaVersion(parsed)) throw new Error(MALFORMED);
  if (isCurrentSchemaVersion(parsed)) return extractV1UserAgents(parsed);
  throw new UnsupportedSchemaVersionError(parsed.schemaVersion);
}
```

Each helper (`isPlainObject`, `hasNumericSchemaVersion`, `isCurrentSchemaVersion`,
`extractV1UserAgents`) owns single concern. Main function readable checklist.

---

## Comment as a missing name

### Smell

Comment explains _what_ next block of code does, meaning of boolean expression, or
role of value. Code itself mechanical; comment is translation key. Readers
keep both code + comment in head, comment drifts out of sync as code changes.

Distinct from _why_ comment (non-obvious constraint, external bug worked
around, deliberate divergence from contract). _Why_ comments survive refactoring; _what_
comments usually missing name in disguise.

### Trigger

About to type `// ` to explain:

- what next 3–10 lines do,
- what boolean expression represents in business language,
- why magic number chosen,
- which "phase" of function starting (`// validate`, `// transform`, `// dispatch`).

Or reading code where each block preceded by such comment, function reads
like heavily annotated script.

### Refactoring

1. For each _what_ comment, ask: would named variable/function replace it?
2. **Comment over value/expression → Extract Variable.** Replace
   `// retention check\nif (date.before(now.minusDays(plan.retentionDays)))` with
   `const isWithinRetention = ...; if (isWithinRetention)`. Drop comment.
3. **Comment over block → Extract Method.** Replace
   `// fold rows into per-window totals\nfor (...) { ... }` with `const summary = summarize(rows)`.
   Drop comment. (See _Compose method_ for multi-step variant.)
4. **Magic number with `// 16 = paddingXS`** → either substitute token directly, or extract
   `const TOKEN_PADDING_XS = 16` with single-line comment if no real token reachable.
5. After extraction, surviving comments should all answer "why this code at all" —
   external behavior worked around, deliberate trade-offs, references to ticket/contract.
   Can't answer "why" → delete comment.

### Structure after refactoring

- Function body reads top-to-bottom as named steps + named expressions.
- Surviving comments short, sit on single helper, explain non-obvious _why_.
- Block comments summarizing next N lines gone — were missing function name.

### Tests

- Pure refactor — existing tests stay green.
- Named extractions occasionally unit-testable if encode non-trivial rule worth
  pinning on own (rare; usually stay private, exercised through caller).

### Example

**AI Discoverability KPI adapter (TypeScript)**

Before — block comments narrate each step:

```ts
async findKPIs(request) {
  // ...
  try {
    // run the consolidated analytics query plus the postgres total-pages lookup
    const [analytics, totalPages] = await Promise.all([...]);

    // fold per-crawler rows into the high-level numbers used by the KPI builders
    let distinctCrawlersCurrent = 0;
    let ...
    for (const row of analytics.data) { ... }

    // we have comparable history iff earliest activity exists and predates dateFrom
    const hasHistoryBeforeCurrentWindow = earliestActivity !== null && earliestActivity < dateFrom;

    // ClickHouse returns min(time) over no rows as '1970-01-01' — treat as null
    const earliestActivity = ...;
    // ...
  }
}
```

After — names carry explanation; only surviving comment is _why_ about ClickHouse:

```ts
async findKPIs(request) {
  // ...
  const [analytics, totalPages] = await Promise.all([...]);

  const summary = summarize(analytics);
  const hasHistory = hasHistoryBefore(summary.earliestActivity, dateFrom);

  return Ok({
    crawlersDetected: crawlersDetectedFrom(summary, hasHistory),
    pageCoverage:     pageCoverageFrom(summary, totalPages, hasHistory),
    errorRate:        errorRateFrom(summary, hasHistory),
  });
}

// ClickHouse returns min(time) over no rows as '1970-01-01 00:00:00' rather than NULL.
function parseEarliestTime(value: string | null | undefined): Date | null { ... }
```

`summarize`, `hasHistoryBefore`, three `*From` helpers replaced four blocks of narrating
comments. Remaining comment on `parseEarliestTime` survives — documents
external ClickHouse behavior reader can't infer from code.

See also _Compose method_ for case where smell is single long function with multiple
phases each deserving own extracted helper.

---

## Comment that restates a test or cross-references foreign code

### Smell

Comment explains _behavior_ (not just what next line mechanically does) **already
pinned by a test**, or narrates **how some other part of codebase works**. Two recurring
shapes:

1. **Restates a test.** Comment asserts rule code obeys — "only tracked crawlers count
   toward detected", "`chatgpt` aliases `gptbot` so it is excluded" — and named test already
   locks that exact rule (`it('counts gptbot and chatgpt as one crawler')`). Test is the
   living specification: fails when behavior changes. Comment doesn't — silently
   drifts until actively misleading. Behavior documented twice, only one
   copy enforced.
2. **Cross-references foreign code.** Comment describes sibling/foreign module —
   "this is how we do it in the private-cache", "mirrors the throttling in the SQS consumer". It
   has no anchor: when referenced code moves/changes, nothing here fails, so note rots
   in place, sends next reader to description no longer matching reality.

This is layer beneath _Comment as a missing name_. There fix was better name; here
comment may be grammatically fine _what/why_ sentence — redundant because **test**
already says it, or unanchored because it points at **code this file doesn't own**.

### Trigger

Reading diff/file, land on comment where one of these true:

- Test already exists (or obviously should) whose name is same sentence as comment. The
  `chatgpt`/`gptbot` alias comment is canonical case — test name _is_ documentation.
- Comment's subject is another file/module/service, not the code it sits above.
- Comment explains _consequence_ of rule ("so the alias is not double-counted") rather
  than external fact reader genuinely can't infer (vendor quirk, worked-around bug,
  ticket reference).

### Refactoring

1. **Restated by a test → delete comment; make test the documentation.** If rule not
   yet tested, write test instead of comment — test name carries explanation
   and enforces it. At most, leave one-line pointer to test when rule genuinely
   surprising at call site (`// invariant pinned by AIDiscoverabilityKPIAdapter spec`); never
   re-explain rule prose.
2. **Cross-reference → delete.** If two places must stay in lockstep, encode link in _code_
   (shared constant, shared helper, type) so change forces both to move, or assert it in
   test touching both. Prose "see also" that nothing checks is worse than silence.
3. After pass, every surviving comment answers "why this code at all" with fact local to
   _this_ file no test could express: external/vendor behavior, deliberate trade-off,
   ticket/contract reference. If comment fails that test, test suite (or name) should
   carry it instead.

### Structure after refactoring

- Behavioral rules live in named tests; code free of prose re-asserting them.
- No comment describes internals of module it doesn't own.
- Surviving comments local *why*s about external facts — exactly the set _Comment as a
  missing name_ also leaves standing.

### Tests

- Pure documentation change — existing tests stay green.
- Where deleted comment described _untested_ rule, add test comment was standing
  in for. Only production-adjacent change this refactor produces, strengthens
  suite rather than just thinning comments.

### Example

**AI Discoverability KPI adapter (TypeScript)** — block above crawler-folding loop read
_"Only tracked crawlers count toward 'detected'. `chatgpt` aliases `gptbot` (same OpenAI bot) and
is excluded from `TRACKED_AI_CRAWLERS`, so …"_. Both sentences already enforced by tests
(`it('counts gptbot and chatgpt as one crawler')` + tracked-crawler filtering specs). The
comment deleted: test names are the specification, unlike comment fail when
aliasing rule changes. Surfaced in PR #3471 review.

---

## Feature envy → Move method

### Smell

Function/method reads multiple fields of _another_ type, contributes nothing of own.
It "envies" other type's data — every call looks like `doX(other.a, other.b, other.c)` or
`doX(other, extraArg)`. Function lives in module scope (or different class) only because
type started life as plain data bag.

Visible cue: free function whose first parameter is `summary`, `report`, `order`, `user`,
etc., whose body reads ≥2 fields of that parameter to produce derived value. Ask "could
method live on data?" — answer "yes, nothing lost" means data is natural owner.

### Trigger

Moment of recognition much earlier than obvious "three free functions in a row"
signal. Every time operating on data belonging to someone else — reading two+
fields of another object to compute/decide something — pause, ask whether that owner
should hold behavior instead. Waiting for obvious signal means fixing scattered code,
not avoiding it.

**Point is coupling, not aesthetics.** When rule lives with data, changing data
shape (or rule) is single-file edit, blast radius bounded. When rule
lives elsewhere, every change ripples — every function signature carries data type, every
call site selects right free function, every new rule adds new function threads it
through every caller.

Concrete questions at moment of writing:

- Reading two+ fields of same parameter to derive output?
- If that parameter's shape later changes (rename, split, new invariant), how many call sites
  break? More than one means rule on wrong side of line.
- Could caller construct that parameter with values contradicting what about to
  derive? If yes, derivation belongs in type's constructor/methods so contradiction
  becomes unrepresentable.

Late, undeniable signals — two/three `do(thing, ...)` functions in a row; type
exported as plain record with no methods; call sites reading as `doX(thing, extras)`
instead of `thing.x(extras)` — confirmations you missed early trigger, not
trigger itself.

### Refactoring

1. Convert plain data type into class (or, in functional codebases, module-level
   namespace owning operations + factory).
2. Make constructor private; expose named static factory (`fromAnalytics`, `fromRequest`,
   `parse`) performing whatever construction logic was previously in free function. This
   blocks callers from constructing type in inconsistent state.
3. Move each "envious" function onto class as method. Drop leading `<data>:` parameter —
   `this` is data now. Other parameters from elsewhere (request-time inputs,
   cross-cutting context) stay as method parameters.
4. Where multiple related fields always travel together (current/previous windows, request/response
   sides), bundle into small inner record so class isn't flat heap of `*Current` /
   `*Previous` fields. Symmetry existing in domain should be visible in type.
5. Orchestrator that threaded data through free functions now reads as sequence
   of method calls on single object.

### Structure after refactoring

- Class (or namespaced module) holding data + methods deriving from it.
- Private constructor + named static factory; callers can't bypass invariants.
- Methods short + self-contained — each derived value has one place to look.
- Orchestrator (use case / adapter / controller) shrinks: composition lives at call site
  via method calls, not free-function plumbing.

### Tests

- Pure refactor — existing tests stay green.
- If new class encodes non-trivial rules (constructor invariants, derived methods that
  branch), unit-test directly. Orchestrator's tests usually need no changes; already
  exercising rules indirectly.

### Example

**AI Discoverability KPI adapter (TypeScript)**

Before — three free functions all envy `Summary`:

```ts
type Summary = {
  distinctCrawlersCurrent: number;
  distinctCrawlersPrevious: number;
  totalRequestsCurrent: number;
  errorRequestsCurrent: number;
  totalRequestsPrevious: number;
  errorRequestsPrevious: number;
  trackedSeenInCurrent: ReadonlySet<TrackedAICrawler>;
  distinctCrawledPagesCurrent: number;
  distinctCrawledPagesPrevious: number;
  earliestActivity: Date | null;
};

function summarize(analytics): Summary { ... }
function hasHistoryBefore(earliest, boundary): boolean { ... }
function crawlersDetectedFrom(summary, hasHistory) { ... }
function pageCoverageFrom(summary, totalPages, hasHistory) { ... }
function errorRateFrom(summary, hasHistory) { ... }

// findKPIs:
const summary = summarize(analytics);
const hasHistory = hasHistoryBefore(summary.earliestActivity, dateFrom);
return {
  crawlersDetected: crawlersDetectedFrom(summary, hasHistory),
  pageCoverage:     pageCoverageFrom(summary, totalPages, hasHistory),
  errorRate:        errorRateFrom(summary, hasHistory),
};
```

After — data + behavior together, current/previous bundled into `WindowMetrics`:

```ts
type WindowMetrics = {
  distinctCrawlers: number;
  totalRequests: number;
  errorRequests: number;
  distinctCrawledPages: number;
};

class DiscoverabilityKPISummary {
  static fromAnalytics(analytics): DiscoverabilityKPISummary { ... }
  private constructor(
    private readonly current: WindowMetrics,
    private readonly previous: WindowMetrics,
    private readonly trackedSeenInCurrent: ReadonlySet<TrackedAICrawler>,
    private readonly earliestActivity: Date | null,
  ) {}

  hasHistoryBefore(boundary: Date): boolean { ... }
  crawlersDetected(hasHistory: boolean): CrawlersDetectedKPI { ... }
  pageCoverage(totalPages: number, hasHistory: boolean): PageCoverageKPI { ... }
  errorRate(hasHistory: boolean): ErrorRateKPI { ... }
}

// findKPIs:
const summary = DiscoverabilityKPISummary.fromAnalytics(analytics);
const hasHistory = summary.hasHistoryBefore(dateFrom);
return {
  crawlersDetected: summary.crawlersDetected(hasHistory),
  pageCoverage:     summary.pageCoverage(totalPages, hasHistory),
  errorRate:        summary.errorRate(hasHistory),
};
```

Orchestrator becomes table of contents; rules live on data; private
constructor blocks inconsistent states. Symmetry between current/previous now visible
in type (`WindowMetrics` × 2) rather than buried in pairs of `*Current` / `*Previous`
fields.

See also _Anemic domain model to rich model_ — feature envy is lightweight, module-local
sibling of same idea applied to domain entities. Same fix shape (move behavior to
data); different scope.

---

## Verb-prefixed query methods (Command-Query naming)

### Smell

Side-effect-free method returning value carries verb prefix naming _how_ answer
produced — `calculate*`, `compute*`, `build*`, `derive*`, `resolve*`, `get*`. Name leaks
implementation strategy (calculation? stored field? cached lookup? aggregation?) to every
caller. Code reading call site must mentally translate verb back to noun method actually
represents. If implementation later changes (cached → recomputed, stored → derived),
verb becomes lie, renaming now breaking change for every caller.

Corollary: method whose name reveals nothing about implementation lets implementation
evolve freely.

### Trigger

Method on entity/value object/data structure has all of:

- no side effects, no `void` return, no thrown exception path callers handle as control flow,
- input only `this` state (or small parameter narrowing what part of `this` to query),
- name starts with verb such as `calculate`, `compute`, `build`, `derive`, `resolve`, `get`,
  `make`, `produce`,
- noun method returns is entity's own concept (`statusLevel`, `total`, `summary`, …).

### Refactoring

1. Identify side-effect-free query methods on entities/value objects.
2. Strip verb; method becomes noun it returns.
   `calculateStatusLevel()` → `statusLevel()`, `getTotalAmount()` → `totalAmount()`,
   `buildSummary()` → `summary()`.
3. If `get*` prefix conveyed meaningful nuance (collections often distinguish `findById` vs
   `getById` for null vs throw), keep verb only when distinction part of contract.
4. Mutations/commands/methods throwing on failure keep verb prefix. Naming itself
   should make Command-Query Separation visible at call site.

### Structure after refactoring

- Query methods read like properties: `order.totalAmount()`, `report.summary()`.
- Command methods keep verbs: `order.cancel()`, `report.markCompleted()`.
- Implementation can switch between stored field, cached property, on-demand derivation without
  touching call sites.

### Tests

- Pure rename: existing tests update only call site.
- No behavior change.

### Example (pseudocode)

```
# Before — the verb leaks "I am a calculation"
class Order
  computeTotal() -> Money
    sum each line item

caller:
  amount = order.computeTotal()

# After — the caller asks for the noun, the class chooses the strategy
class Order
  total() -> Money
    sum each line item

caller:
  amount = order.total()

# A later optimization can store `total` as a memoized field,
# fetch it from a precomputed projection, or recompute every call.
# The name does not change. Callers do not change.
```

---

## Pass-through Layer (Middleman)

### Smell

Class adds no behavior. Receives call, hands to single collaborator, returns
result. Renaming arguments/return types isn't behavior. Class exists only to
preserve symmetry with other layers ("every entity needs a Service and a UseCase"), or as
speculative seam that never accumulated logic.

### Trigger

Any of these patterns enough to flag:

1. A **use case** whose `run()` body is one port call + a `return`. No orchestration of multiple ports, no policy, no invariant checks.
2. A **service** wrapping single repository method + forwarding call (`getUser(id)` → `userRepository.findById(id)`).
3. A **controller** whose handler calls another controller (or another service doing same).
4. Class with **only one method**, doing **one delegation**, no extra value at call site.

### Refactoring

1. Identify underlying collaborator middleman forwards to.
2. Inject collaborator directly at call site (e.g., into controller).
3. Delete middleman class + its tests.
4. If middleman was only thing standing between two layers and rename it performed had
   meaning, **rename the collaborator** instead of preserving wrapper. (Example: read-side
   port called `*Repository` whose only consumer was pass-through UseCase — rename port
   to `*Query`. See _Read-side port named "Repository"_ below.)
5. Update tests to use collaborator (or its fake) directly. Controller integration specs that
   previously mocked middleman now drive collaborator's fake via `seed(...)` / `failWith(...)`.

### Structure after refactoring

- One fewer layer between controller (or other caller) and collaborator.
- Collaborator's name carries intent middleman tried to convey.
- Fake for collaborator is seam integration tests use.

### Tests

- Tests of deleted middleman go away — only proved forward call worked.
- Tests of caller (controller, etc.) now drive collaborator's fake directly. Behavior
  surface covered unchanged; level asserted at moves closer to real boundary.

### When NOT to refactor

Keep middleman only if it represents **stable seam about to acquire policy** — e.g.,
authorization, caching, rate limiting, projection assembly — and work in flight/
imminent. Document upcoming reason in class header. Don't keep speculative seams "in
case" they grow.

### Example (pseudocode)

```
# Before — the use case is a one-line forward.
class ListActiveUsersUseCase
  ctor(owners: ActiveUsersPort)
  run() -> Result<list<UserId>, Failure>
    return this.owners.listActive()

class Controller
  ctor(listActive: ListActiveUsersUseCase)
  GET /active-users
    return await this.listActive.run()
```

```
# After — middleman deleted; controller injects the port directly.
class Controller
  ctor(activeUsers: ActiveUsersPort)
  GET /active-users
    return await this.activeUsers.listActive()
```

Controller integration test switches from mocking use case to driving fake of
collaborator. Behavior covered same; level moves closer to real boundary.

---

## Read-side port named "Repository"

### Smell

Port called `*Repository` whose only methods are read-shaped: `findAll`, `count`, `findBy*`,
`list*`. No `save`, no `delete`, no aggregate-shaped operations. "Repository" suffix promises
consistency boundary port doesn't actually enforce.

**Not a smell:** Repository with **both** `save` _and_ a `findByX` that loads aggregate
whole by its identity. Loading aggregates is part of Repository's job (Vernon). Smell is
purely read-only ports borrowing Repository name.

### Trigger

Review port's surface area, notice every method a read, no method changes state.
Port doesn't load aggregates for mutation, doesn't participate in any write transaction.
Exists purely to answer queries.

### Refactoring

1. Rename port from `*Repository` to **`*Query`** (one name — not Finder/Reader/Report; this
   project standardizes on `*Query` for every read-side port).
2. Rename symbol/injection token to match (`*_REPOSITORY` → `*_QUERY`).
3. Rename method if `Repository`-style verb prefixes leaked in (`listActiveUserIds` →
   `findAll` if port is active-users query).
4. Move port file from `domain/models/<aggregate>/` (write-side location) to `domain/query/`
   (read-side peer folder). Read-side ports don't belong inside aggregate's package.
5. Update adapter's `implements` clause + method body — body usually doesn't change,
   only method name + signature.
6. Update fakes (rename file, class, method).
7. Update contract test (rename function, file). Scenarios stay.
8. Often co-occurs with _Pass-through Layer (Middleman)_ — when removing pass-through UseCase
   from above read-side port, rename happens in same refactor.

### Structure after refactoring

- Port named `*Query`, located in `domain/query/`.
- Symbol named `*_QUERY`.

### Tests

- Contract tests rename but keep same scenarios.
- Adapter integration test renames `describe` heading + contract-invocation function.
- Fake spec renames file + contract import.
- No behavioral change.

### Example (pseudocode)

```
# Before
interface IntegratedDomainOwnersRepository
  listActiveUserIds() -> Result<list<UserId>, LookupFailure>

token INTEGRATED_DOMAIN_OWNERS_REPOSITORY
```

```
# After
interface ActiveUsersQuery
  findAll() -> Result<list<UserId>, LookupFailure>

token ACTIVE_USERS_QUERY
```

Adapter's `implements` clause changes from old name to new one; body of
read method unchanged.

---

## Read-side Query that mirrors the aggregate (pass-through View)

### Smell

`*Query` port returns `*View` whose shape is `{ ...aggregate, derivedProp: aggregate.method() }`,
and its adapter reads same single row, by same primary key, that Repository writes.
View duplicates aggregate's fields, only "extra" is materializing aggregate's
own derivation methods as properties. Split adds parallel read model, real adapter, fake,
contract spec, mapper — all behaving identically to Repository read with renamed fields.

This is read-side counterpart of _Pass-through Layer (Middleman)_: instead of extra class
between layers, it's extra port between controller and same data Repository already
exposes.

### Trigger

Any of:

- `*View` type's fields equal `aggregate`'s fields plus methods-as-properties
  (`view.status = aggregate.statusLevel()`, `view.triggers = aggregate.triggers()`).
- `*QueryAdapter` reads same table, by same primary key (`user_id`, `order_id`, …)
  that `*RepositoryAdapter` writes.
- Controller would work identically if injected Repository and called
  `findByUserId(...)`, then projected aggregate via DTO mapper.

### Refactoring

1. Add `findByX(...)` to `*Repository` interface, returning aggregate (Vernon: Repositories
   load aggregates whole by identity).
2. Implement `findByX` on `*RepositoryAdapter` (often one-line knex/sql query).
3. Update HTTP DTO mapper to take aggregate (not View), call aggregate's
   derivation methods (`aggregate.statusLevel()`, `aggregate.triggers()`) inline at projection
   time.
4. Update controller to inject Repository, call `findByX(...)`. Map null → 404.
5. Delete `*Query` port, its `*View` model, contract spec, fake, Postgres
   (or other) adapter.
6. Delete any failure types only consumed by Query (`*LookupFailure`) — unless
   Repository also needs them.

### Structure after refactoring

- One port (`*Repository`) for aggregate, with both `save` + `findByX`.
- One adapter.
- One fake.
- One contract spec, exercising roundtrip.
- HTTP DTO mapper runs derivation methods on aggregate inline.

### Tests

- Repository contract gains `findByX` scenarios (returns aggregate; returns null when absent;
  per-user isolation).
- Old Query contract scenarios disappear; coverage subsumed by Repository
  contract.
- Controller integration spec switches from `FakeQuery.seed(view)` to
  `fakeRepository.save(aggregate)`. HTTP response assertions stay — projection still
  produces same payload.

### When NOT to refactor (when to keep the Query split)

Keep Query split when read genuinely diverges from aggregate:

- Filters on columns aggregate doesn't carry (`WHERE status = 'critical'` over many rows —
  wants denormalized indexed column).
- Joins across aggregates (`OrderSummaryView { order, customerName, lineItemCount }`).
- Pagination/list shapes (`PageOf<UserListItem>`).
- Projection-only data (counts, sums, denormalized facts).
- Read load heavy enough eager pre-computation at write time pays off.

If none apply, Query is middleman — collapse it.

### Example (pseudocode)

```
# Before — Query reads the same row the Repository writes, returns a View
# that just lifts the aggregate's methods into properties.

interface DiscoverabilityStatusQuery
  findByUserId(userId) -> Result<DiscoverabilityStatusView?, LookupFailure>

type DiscoverabilityStatusView = {
  status: aggregate.statusLevel()    # derived
  triggers: aggregate.triggers()     # derived
  evaluatedAt
  domains: aggregate.domainIssues    # renamed
}

class Controller(query: DiscoverabilityStatusQuery)
  GET /status -> toResponseDto(query.findByUserId(...).value)
```

```
# After — Query / View / QueryAdapter / FakeQuery / Query-contract all deleted.
# Repository has both save and findByUserId; DTO mapper takes the aggregate.

interface DiscoverabilityStatusRepository
  save(status)
  findByUserId(userId) -> DiscoverabilityStatus?

class Controller(repo: DiscoverabilityStatusRepository)
  GET /status:
    let s = repo.findByUserId(...)
    if s == null: 404
    return { status: s.statusLevel(), triggers: s.triggers(), evaluatedAt: s.evaluatedAt, domains: s.domainIssues }
```

---

## Misplaced projection in a foreign write-side table (orphaned ownership)

### Smell

Derived, read-oriented value — a _projection_ of events happened in another context — is
materialized as column on **operational write-side table**, job keeping it fresh
lives in service owning **neither source of truth nor destination table**. Three
failures stack:

1. **Misplaced data.** Value is read model (lookup/reporting projection), yet sits
   inside table whose reason to exist is operational writes. Every reader of that table now
   couples to column unrelated to table's purpose, table's owner can no longer reason
   about who writes it or when.
2. **Misplaced behaviour.** Context actually producing underlying event does **not**
   write value — frequently because writing it inline would overwhelm database (millions of
   writes/day, connection exhaustion). Write exiled to batch/queue job in _third_
   service reading from one store (analytics/event log), writing into table it does not own.
3. **No owner.** Because data + behaviour split across contexts each disown them,
   nobody owns failure mode. Job breaks in production, stays broken: event producer
   says "not my table", table's owner says "not my job", job's host service says "not my
   data". Chronic, unowned failure is diagnostic symptom smell has set in.

Tell: _"we keep the projection on `<operational_table>` because that's where readers already
look, and a job in `<other_service>` backfills it from `<analytics_store>` because writing it at the
source would blow up the DB."_

### Trigger

- Column on operational table written **only** by job in different service — never by
  code owning table, nor code owning event value summarizes.
- That backfill/projection job fails repeatedly in production, no single team treats it as theirs.
- Justification for not writing value at source is scaling concern ("too many
  writes"). That's exactly the signal value wants to be **async projection with
  deliberate owner**, not column smuggled into someone else's table.

### Refactoring

1. **Name source of truth.** Identify context owning event (e.g. delivery to
   crawlers → delivery/cache context, whose events already land in analytics store).
2. **Make projection first-class read model with one owner.** Move it out of operational
   table into store read side owns (own projection table / materialized view / read
   cache). Operational table goes back to holding only what its own writers own.
3. **Give maintaining behaviour to owner of source of truth.** Producing context
   publishes event (stream / outbox / domain event); read side subscribes, updates own
   projection. Scaling problem that forced batch hack now solved by async projection
   with backpressure — not by exiling write to foreign job.
4. **Delete orphan job, or relocate into owning context** as explicit, owned projector
   with own alerting + SLO. Either way gains single team owning its failures.
5. **Point readers at projection**, not column bolted onto write table.

### Structure after refactoring

- Operational table: only fields its writers own; no foreign-maintained columns.
- Projection store: owned by read side, fed by events from source-of-truth context.
- Exactly one projector, clear home, explicit ownership, own monitoring.
- **Data + behaviour maintaining it share single owner** — this is the fix; data-placement
  change without ownership change just moves the orphan.

### Tests

- Projector test: given source events, projection updates correctly — idempotent, tolerant of
  replay + out-of-order delivery.
- Reader test: reads come from projection, defined behaviour when stale/missing.
- Regression test proving operational table no longer needs (or carries) foreign column.

### Example

**`last_delivered_at` on operational `resources` table.** Value represents last time
resource delivered to consumer — read-only projection consumed by many. Lived as column
on `resources` (operational write table). Delivery context, owning delivering to
consumers, deliberately did **not** write it inline, since doing so would issue millions of writes
a day, exhaust DB connections. Instead job in separate queue service (`LastDeliveredJob`) read
delivery data from analytics store, wrote projection back into `resources` — table it
doesn't own. Result: misplaced read projection, behaviour with no real owner, job that
fails consistently in production, no team treating it as theirs. Fix is ownership: delivery
context owns + publishes delivery events; projection becomes read model owned by
its readers, fed from those events; orphan job retired or rehomed into owning context
with own alerting.