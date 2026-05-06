# Stage 2 Slice 1 — User Typed Wrapper Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Date:** 2026-05-06
**Branch:** `feat/zentao-controller-extension` (continuing — typed wrappers stack on the Stage 1 transport without a new branch unless review demands it)
**Stage 1 contract:** [`2026-05-06-controller-extension-stage1.md`](../specs/2026-05-06-controller-extension-stage1.md)
**Order:** user → program-audit → product-audit → project → execution (frozen during 2026-05-06 grill, see plan-2026-05-06).

This is the first slice of Stage 2. It is intentionally narrow: it adds **only** the `user` entity wrapper at the `zentaoAPI` layer. Resource-layer / data-source work in `zentao/` belongs to a follow-up plan.

---

## Goal

Land typed methods on `*Client`:

```go
GetUser(ctx, account string) (*User, error)
CreateUser(ctx, *User)              (*User, error)
UpdateUser(ctx, *User)              (*User, error)
DeleteUser(ctx, account string)      error
```

Backed by ZenTao Controller PATH_INFO routes:

```
GET  /user-view-<account>.json
POST /user-create.json                  (or POST /user-edit-<id>.json)
POST /user-edit-<id>.json
GET  /user-delete-<id>.json?confirm=yes
```

(The exact route strings, primary-key choice — `account` vs `id` — and field set will be confirmed by probe before implementation.)

Definition of done:
- `zentaoAPI/user.go`: `User` struct, `userCtrlWire` shape, four typed methods.
- `zentaoAPI/user_test.go`: httptest table covering happy path + ErrNotFound + ErrUnauthorized leak guards + envelope failure passthrough for each method.
- One integration test extension to `client_integration_test.go`: round-trip `Create → Get → Update → Delete` against real instance.
- No regressions on existing `zentaoAPI` unit suite or `zentao` acceptance suite.

Out of scope:
- Group / role assignment (deferred — different controller).
- LDAP-backed users (server-side concern).
- Password change action (`user-changePassword`) (treat as separate `ChangePassword` method in a later slice if needed).
- Resource layer (`zentao/resource_user.go`, `zentao/data_source_user.go`).

---

## Open questions blocked on real-instance probe

These cannot be resolved while the test instance MySQL is down. Each is small enough to hold up only the relevant task, not the whole plan.

| # | Question | Why it matters | Default if probe blocked indefinitely |
|---|---|---|---|
| Q1 | Which actions does the user controller expose on Max 8.1? `view` / `create` / `edit` / `delete`, and what about `unbind`, `lock`, `restore`, `forbid`? | Determines wrapper surface | Implement only the four CRUD verbs |
| Q2 | Does `user-create.json` accept JSON body or only form-urlencoded? | Decides whether `doController` (JSON default) suffices or we must add `doControllerForm` | Try JSON; if probe shows form-only later, add fallback |
| Q3 | Primary key on edit/delete: `id` (int) or `account` (string)? Probe step 4 of stage-1 (`/api-getsessionid` + cookie) suggests routes like `user-edit-<id>.json`. | Affects path-arg encoding | Assume `id` numeric; surface `account` as separate lookup |
| Q4 | Does `view` accept `account` as the path arg, or also `id`? | Cosmetic but affects `GetUser(ctx, account string)` ergonomics | Use `account` (matches V2 login response shape) |
| Q5 | What does the `view` envelope look like? Is `data` the user object, or is the user nested under `data.user`? | Decides `userCtrlWire` parsing | Assume `data` is the user object directly; `DecodeData` handles the string-wrapped variant |
| Q6 | Required fields on `create`? (`account`, `password`, `realname`?) Optional: `email`, `dept`, `role`, `gender`, `phone`, etc. | Wrapper validation | Make only `account` + `password` required; everything else optional |
| Q7 | How does ZenTao return errors for `account already exists` and `password too weak`? | Maps to `*APIError` reasons; may need new `isXxxReason` matchers | Use `classifyCtrlError` default; add reason matchers iteratively |
| Q8 | Does `delete` honour `confirm=yes` or use a different idempotency mechanism? | Confirms Stage 1 design decision | Apply `?confirm=yes` query param; treat 404 + "not exist" as success |

A 30-minute probe covers all eight. The probe is itself a checklist task in Phase A below.

---

## Phase A — Probe (blocked on instance health)

- [ ] **A1.** Confirm ZenTao test instance is healthy (no MySQL connection-refused error).
- [ ] **A2.** Probe `user-view-admin.json` — capture envelope shape, field names, types for `userCtrlWire`. Save to `docs/superpowers/specs/probe-user-controller.md`.
- [ ] **A3.** Probe `user-create.json` with a unique account — verify JSON body accepted (Q2). Capture success and `account already exists` failure shapes.
- [ ] **A4.** Probe `user-edit-<id>.json` — verify path arg semantics (Q3) + diff field set vs create.
- [ ] **A5.** Probe `user-delete-<id>.json?confirm=yes` — capture success envelope (`{locate}` redirect-style? `status:success` only?) and missing-row response (Q8).
- [ ] **A6.** List Controller actions actually present (Q1) by inspecting either ZenTao's UI menus or the `user` controller class file. If only docs available, scan reference URL: https://www.zentao.net/book/api/2309.html (or instance-local `/api/zentaoapi/`).
- [ ] **A7.** Update this plan with the answers; freeze each Q with a one-line resolution.

**Verification (Phase A):** probe doc committed; this plan's "open questions" section replaced with a "resolved" table.

---

## Phase B — `User` type and `userCtrlWire`

- [ ] **B1.** Define `User` struct in `zentaoAPI/user.go`, mirroring the conventions used by `Product` / `Program`:
  - Identity: `ID int`, `Account string`, `Realname string`.
  - Writeable optional: `Email`, `Phone`, `Mobile`, `Dept`, `Role`, `Gender`, `Skype`, `QQ`, `Weixin`, `Address`, `Birthday`, `Visions` — final list pinned by probe Q6.
  - **Sensitive write-only:** `Password string` with `json:"password,omitempty"` and a `// WARN: never logged.` comment. Read-side never populates it.
  - Read-only / server-managed: `Status`, `Visits`, `Last`, `OpenedDate`, `CreatedBy` (where applicable). All `json:"-"`.
- [ ] **B2.** Define `userCtrlWire` mirroring `programV2Wire` style — every numeric field as `json.Number` (Controller envelope under `data` may serialise IDs as strings or numbers depending on action).
- [ ] **B3.** Add `(w userCtrlWire) toUser() (*User, error)` conversion.
- [ ] **B4.** Unit-test `toUser` (table-driven over the wire shapes captured in probe A2/A4).

**Verification (Phase B):** `go test -run 'TestUserWire' ./zentaoAPI/...` green.

---

## Phase C — CRUD methods

- [ ] **C1.** `GetUser(ctx, account string) (*User, error)`:
  - URL via `controllerPath("user", "view", []string{account})`.
  - Decode `CtrlEnvelope` → `DecodeData` → `userCtrlWire` → `toUser`.
  - On envelope `status != "success"` route through `classifyCtrlError`.
- [ ] **C2.** `CreateUser(ctx, *User) (*User, error)`:
  - POST `controllerPath("user", "create", nil)` with `User` as body (validates `Account` + `Password` non-empty pre-flight).
  - Server may return only the new ID; if so, follow with `GetUser` to surface authoritative state.
  - Capture `account already exists` failure and surface as `*APIError` (or new `ErrConflict` if pattern repeats — postpone until probe Q7 resolved).
- [ ] **C3.** `UpdateUser(ctx, *User) (*User, error)`:
  - Pre-flight: require `User.ID != 0`.
  - POST `controllerPath("user", "edit", []string{strconv.Itoa(p.ID)})` with body.
  - Re-fetch after success (`GetUser` by `Account` for authoritative state).
- [ ] **C4.** `DeleteUser(ctx, id int) error`:
  - GET `controllerPath("user", "delete", []string{strconv.Itoa(id)})` with `query={"confirm":"yes"}`.
  - Idempotent: `ErrNotFound` and `*APIError` whose reason matches `isNotFoundReason` both treated as success.
- [ ] **C5.** Unit tests in `zentaoAPI/user_test.go` modelled on `program_test.go`. Each method gets:
  - Happy path
  - `ErrNotFound` propagation (where applicable)
  - Envelope-failure passthrough as `*APIError`
  - Edge: empty/invalid input rejected pre-flight

**Verification (Phase C):** `go test -race -cover ./zentaoAPI/...` green; per-function coverage on new methods ≥85%; package coverage stays ≥84%.

**NOTE — deferred extraction:** the four methods will share envelope/decode/classify plumbing. Do **not** extract a `callCtrl` helper here. The Stage 1 contract commits to extracting only after the *second* typed entity (project) lands. Verbose-but-explicit is the right state right now.

---

## Phase D — Integration test extension

- [ ] **D1.** Append to `zentaoAPI/client_integration_test.go` (still under `//go:build integration`):
  ```
  TestIntegration_UserCRUD_RoundTrip
    - Create temporary user with timestamp suffix
    - Get → assert fields match
    - Update realname → re-Get → assert mutation
    - Delete → re-Get → ErrNotFound
  ```
- [ ] **D2.** Run with real env vars; capture pass/fail; mark as the live regression gate for slice 1.

---

## Phase E — Documentation

- [ ] **E1.** README.md: append "User" subsection under Resources (mark as "API client only — Terraform resource forthcoming").
- [ ] **E2.** Stage-1 spec → leave as-is (Stage 2 work doesn't supersede it).
- [ ] **E3.** Append Phase A probe outcomes to `docs/superpowers/specs/2026-05-06-controller-extension-stage1.md` "verification" table or create a sibling stage-2 doc.

---

## Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | Test instance MySQL outage prevents probe → blocks Phase A indefinitely | Document the instance-health requirement; confirm with instance owner that DB is restartable; otherwise pause slice 1 here |
| R2 | `user-create.json` rejects JSON body | Add `doControllerForm` private helper (Stage 1 plan already noted this fallback is permitted) |
| R3 | Edit / Delete primary-key mismatch (account vs id) | Probe Q3 settles before C2/C3 implementation; refactor surface if probe reveals account-keyed routes |
| R4 | `User.Password` accidentally logged or surfaced in errors | Verify `APIError.Error()` redaction (existing `passwordPattern` already handles `"password":"..."`); add a unit test asserting password never appears in any error string from user methods |

---

## Success criteria (slice 1)

1. `go build`, `go vet`, `go test -race -cover ./zentaoAPI/...` all green.
2. `TestIntegration_UserCRUD_RoundTrip` passes against real instance.
3. Existing TF_ACC=1 acceptance suite (`product` + `program`) remains green.
4. Branch ready for review; no unresolved open questions in this plan.
