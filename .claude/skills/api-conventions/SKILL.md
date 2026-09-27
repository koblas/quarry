---
name: api-conventions
description: HTTP / REST boundary rules — controller shape, REST URL design, request/response modeling, validation ownership, status codes, HTTP semantics, idempotency. Stack-agnostic. Invoke this skill when designing or reviewing a new HTTP endpoint, modeling request/response DTOs or error payloads, choosing HTTP methods or status codes, or drawing the line between input-format validation and domain rule enforcement.
---

Stack-agnostic rules for HTTP boundary. Independent of app architecture (Clean / Hexagonal / classic MVC) — cover wire.

Use when:
- Design or review new HTTP endpoint.
- Model request DTOs, response DTOs, error payloads.
- Pick HTTP methods, status codes, headers.
- Draw line between input-format validation vs domain rule enforcement.

## 1. Thin controllers

- Controller job: deserialize input → call use case (or read-side query) → map response. **No business logic** in controller: no domain calc, no conditional branch on domain state, no filter/sort of domain data use case should own.
- Prefer **one controller class per business action** (`DepositMoneyController`, `WithdrawMoneyController`) over fat controller (`AccountController.deposit()`, `AccountController.withdraw()`, …). Per-action controller make routing + test surface obvious.
- Controller methods stay short — few lines. Method past ~15 lines = logic belong deeper.

## 2. No domain leakage in API responses

- **Domain entities never returned directly from API.** Map to response DTO at boundary. HTTP response shape = contract API owe clients; coupling to domain entity drag every internal field rename into public API change.
- **Domain exceptions mapped to HTTP status codes at controller boundary.** Exception type stay in domain; controller (or shared exception filter) translate. Domain exceptions must not appear in response body.

## 3. REST URL conventions

- **Resource-oriented paths.** `/accounts/{id}/deposits` — not `/doDeposit`, not `/account/deposit`.
- **Plural nouns for collections.** `/accounts`, `/orders`, `/users`. Not `/account`.
- **No verbs in URLs.** HTTP method = verb. `POST /accounts/{id}/deposits` not `POST /accounts/{id}/depositMoney`.
- **Nested resources for relationships.** `/accounts/{id}/transactions` when transactions belong to one account; `/transactions?accountId=…` when queried independently.
- **Consistent naming style.** Pick `kebab-case` *or* `camelCase` for path segments. Stick to it across whole API. Don't mix.

## 4. Input validation scope

- **API layer validates format only**: JSON well-formed? Required fields present? Types parseable (number, ISO date, enum)?
- **Business rule validation belong in domain** — invariants enforced by entity constructor/factory, or by use case. DTO with `@Min(0)` / `@Pattern(...)` encoding business rule = violation: domain can't trust input that bypassed API layer (other microservice, test, CLI).
- Result: every entry point into domain hit same invariant enforcement.

## 5. Response modeling

- **Response DTOs contain only what client needs.** No internal IDs, no domain implementation details, no flags client can't act on. Each field on public response = contract you must support and version.
- **Error responses follow consistent structure** across every endpoint. Pick shape (e.g. `{ "error": { "code": "...", "message": "...", "details": [...] } }`) and use everywhere. Mixed shapes force every client to handle multiple parse paths.
- **HTTP status codes semantically correct**:
  - `200 OK` — successful read or update returning content.
  - `201 Created` — resource created (usually paired with `Location` header).
  - `204 No Content` — successful op with empty body (update with no return, delete).
  - `400 Bad Request` — malformed input, missing required field, type mismatch, **domain invariant violation** (e.g., negative amount).
  - `401 Unauthorized` — missing/invalid auth.
  - `403 Forbidden` — authenticated but not allowed to access resource.
  - `404 Not Found` — resource doesn't exist (GET/PUT/PATCH/DELETE of missing identifier).
  - `409 Conflict` — op conflicts with current state (concurrent update, duplicate creation).
  - `500 Internal Server Error` — unexpected infra/runtime failure. Use only when no more specific code fits.

## 6. HTTP semantics

- **Correct methods**:
  - `POST` for create (and non-idempotent actions when nothing else fits).
  - `GET` for read; must be side-effect-free and cacheable.
  - `PUT` for full replacement; idempotent.
  - `PATCH` for partial update; idempotent if patch is.
  - `DELETE` for removal; idempotent.
- **`Location` header on `201` responses** when applicable — points to created resource URL.
- **`Content-Type` set correctly** on all responses with body. `application/json; charset=utf-8` = default for JSON APIs; pick `application/problem+json` if using RFC 7807 problem details.
- **`Cache-Control` and `ETag`** when appropriate for read endpoints with heavy traffic. Out of scope for most internal APIs.

## 7. Idempotency and safety

- `GET`, `PUT`, `DELETE`, `PATCH` should be idempotent — same request twice = same result.
- `POST` = only non-idempotent verb by default. If POST endpoint safe to retry, document it (or use `Idempotency-Key` header pattern).
- `DELETE` of resource that doesn't exist should return `204` (idempotent) or `404` (strict). Pick one. Apply consistently.