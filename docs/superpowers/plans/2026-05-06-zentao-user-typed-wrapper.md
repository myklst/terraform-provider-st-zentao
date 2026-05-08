# Stage 2 Slice 1 — User Typed Wrapper Plan

> **2026-05-06 update:** Phase A probe completed — see [`probe-user-controller.md`](../specs/probe-user-controller.md) for raw findings. Several Q1–Q8 answers shifted what's testable; this plan section ↓ "Resolved by probe" reflects the actual scope. Live integration coverage is now limited to a `GetUser` round-trip; create/update/delete rely on httptest unit tests due to instance license cap and a verifyPassword sudo gate.

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

## Resolved by probe (2026-05-06)

| # | Question | Resolution |
|---|---|---|
| Q1 | Available actions | `edit-<id>` (GET = read, POST = write), `create` (POST), `delete-<id>?confirm=yes` (GET). `view` redirects to todocalendar (unusable). `browse`/`all`/`admin` don't exist on this controller. |
| Q2 | JSON or form body | **Form-urlencoded ONLY.** JSON body is silently ignored by `user-edit` (server re-renders the GET form). → `doControllerForm` is mandatory. |
| Q3 | Primary key | **`id` (int)** for both edit and delete. Account-keyed read needs a separate (untested) lookup; surface as `GetUser(ctx, id int)` for now. |
| Q4 | `view` arg | Moot — `view` is unusable. **Read primitive becomes `user-edit-<id>.json` GET** which returns the user inside the form-context envelope. |
| Q5 | Read envelope | `{status, data: "<JSON-string with .user inside>", md5}` — same Stage 1 shape A. The user object lives at `inner.user`, not at `inner` directly. |
| Q6 | Required fields | `account`, `password`, `realname`, `dept`, `gender`, `visions` ("rnd"/"lite") — confirmed by validation errors. Other fields optional. |
| Q7 | Error reasons | License-cap on create returns shape C `{result:fail, message:"...授权的上限..."}`. Validation errors use shape C with `message` as `map[fieldName][]string`. |
| Q8 | Delete idempotency | `?confirm=yes` honoured; missing-row response is `{status:success, data:"{...,user:false,...}"}` rather than an error. Caller must inspect `inner.user == false` to detect no-op. |

## New constraints surfaced by probe

| | |
|---|---|
| **License-cap on create** | This instance is full; live `CreateUser` cannot be tested. Httptest covers it. |
| **verifyPassword sudo gate** | `user-edit` POST and (likely) `user-delete` GET require a `verifyPassword` field. The hashing scheme (md5 plain / md5 salted / etc.) could not be cracked from probing alone. **Live update/delete testing not possible on this instance.** Wrapper exposes the field as a caller-supplied string for instances WITHOUT this gate. |
| **Third envelope shape (`CtrlSimpleResponse`)** | Write/form-submit responses use `{result:success\|fail, message:<string\|map>, load:<redirect>}`. The Stage-1 `CtrlEnvelope` cannot decode this. New type required. |
| **Read primitive is `edit GET`, not `view`** | `user-view-<x>` always 302s to todo-calendar. Read uses `user-edit-<id>.json` GET. |

---

## Phase A — Probe ✓ DONE

- [x] **A1.** Instance health confirmed (after MySQL recovery).
- [x] **A2–A6.** All actions probed; results in `docs/superpowers/specs/probe-user-controller.md`.
- [x] **A7.** Plan updated with resolutions (above) + new constraints.

---

## Phase B — Envelope C + form-encoded transport (prerequisites)

These are infrastructure pieces that must land before the user wrapper itself, because the user controller cannot be served by the existing Stage 1 transport alone.

- [ ] **B1.** Add `CtrlSimpleResponse` to `zentaoAPI/types.go`:
  ```go
  type CtrlSimpleResponse struct {
      Result  string          `json:"result"`            // "success" | "fail"
      Message json.RawMessage `json:"message,omitempty"` // string OR map[string][]string
      Load    string          `json:"load,omitempty"`    // post-action redirect URL
  }
  ```
  with helpers `(r CtrlSimpleResponse) IsSuccess() bool` and `(r CtrlSimpleResponse) FieldErrors() (string, map[string][]string)` that classifies the message as either flat-string or per-field map.
- [ ] **B2.** Add private `doControllerForm(ctx, module, method, pathArgs, query, formBody url.Values) ([]byte, int, error)` to `zentaoAPI/controller.go`. Same auth/refresh pipeline as `doController`; only difference is `Content-Type: application/x-www-form-urlencoded` and form-encoded body instead of JSON.
- [ ] **B3.** Unit tests in `controller_test.go` for `doControllerForm` (Content-Type assertion, body shape, query plumbing, redirect refresh smoke) and in `types_test.go` for `CtrlSimpleResponse` (string-message, map-message, success/fail predicate).

**Verification (Phase B):** `go test ./zentaoAPI/... -run 'CtrlSimpleResponse|DoControllerForm'` green.

## Phase C — `User` type and `userCtrlWire`

- [ ] **C1.** Define `User` struct in `zentaoAPI/user.go`:
  - Identity: `ID int`, `Account string`, `Realname string`.
  - Writeable: `Email`, `Phone`, `Mobile`, `Dept int`, `Role string`, `Gender string` ("m"/"f"), `Skype`, `QQ`, `Weixin`, `Address`, `Birthday`, `Visions string` (default "rnd"), `Nickname`, `Type string` ("inside"/"outside").
  - **Sensitive write-only:** `Password string` and `VerifyPassword string`. Both `json:"password,omitempty"` / `json:"verifyPassword,omitempty"` and a `// WARN: never logged.` comment. Read side leaves both empty. Caller fills `VerifyPassword` if their instance enforces sudo confirmation.
  - Read-only / server-managed: `Last string`, `Visits int`, `Locked string`, `Deleted string`. All `json:"-"`.
- [ ] **C2.** Define `userCtrlWire` mirroring `programV2Wire` — all numeric fields decoded as `json.Number` defensively.
- [ ] **C3.** `(w userCtrlWire) toUser() (*User, error)` conversion.
- [ ] **C4.** Unit-test `toUser` with the actual probe payload as fixture.

**Verification (Phase C):** `go test -run 'TestUserWire' ./zentaoAPI/...` green.

---

## Phase D — CRUD methods

- [ ] **D1.** `GetUser(ctx, id int) (*User, error)`:
  - GET via `doController("user", "edit", []string{itoa(id)}, nil, nil)` (read primitive — `view` is unusable, `edit GET` returns user inside form context).
  - Decode `CtrlEnvelope` → `DecodeData` into a struct with `User userCtrlWire` field → `toUser`.
  - On envelope `status != "success"` route through `classifyCtrlError`.
  - On success but `inner.user == false` (delete-style empty marker), return `ErrNotFound`.
- [ ] **D2.** `CreateUser(ctx, *User) (*User, error)`:
  - Pre-flight: require `User.Account` + `User.Password` + `User.Realname` non-empty + `User.Visions` ∈ {"rnd","lite"} (default "rnd" if empty).
  - `doControllerForm("user", "create", nil, nil, encode(*User))` — POST form-urlencoded.
  - Decode `CtrlSimpleResponse`. On `result:fail`, surface field-map message verbatim (license-cap text and per-field validation map both bubble through `*APIError`). On success, follow up with `GetUser` to obtain the new ID + authoritative state. (License-cap response carries no id, so success path implies id-discoverable.)
- [ ] **D3.** `UpdateUser(ctx, *User) (*User, error)`:
  - Pre-flight: require `User.ID != 0`. If caller's instance enforces sudo, `User.VerifyPassword` must be set; we don't enforce because the gate isn't universal.
  - `doControllerForm("user", "edit", []string{itoa(p.ID)}, nil, encode(*User))`.
  - On `result:success`, re-fetch via `GetUser` for authoritative state.
  - On `result:fail` with `verifyPassword` field-error, surface a clearer `*APIError` reason ("update requires verifyPassword; this instance enforces a sudo gate").
- [ ] **D4.** `DeleteUser(ctx, id int) error`:
  - GET via `doController("user", "delete", []string{itoa(id)}, map[string]string{"confirm":"yes"}, nil)`.
  - Probe showed missing rows return `{status:success, data:"...user:false..."}` rather than an error. Decode and inspect `inner.user` → `false`/null = treat as no-op success (idempotency).
  - Real-row delete envelope shape unknown; on success treat any non-fail envelope as deletion. If `result:fail` with a sudo-gate message, surface accordingly.
- [ ] **D5.** Unit tests in `zentaoAPI/user_test.go`, fixture-driven from `probe-user-controller.md`. Each method:
  - Happy path
  - `ErrNotFound` propagation (where the controller exposes a not-found shape)
  - License-cap fail (CreateUser only)
  - VerifyPassword fail (UpdateUser only) — assert `*APIError` carries the gate message
  - Pre-flight rejection on empty required fields

**Verification (Phase D):** `go test -race -cover ./zentaoAPI/...` green; per-function coverage ≥85%; package coverage ≥84%.

**NOTE — deferred extraction:** the four methods will share envelope/decode/classify plumbing. Do **not** extract a `callCtrl` helper here. The Stage 1 contract commits to extracting only after the *second* typed entity (project) lands. Verbose-but-explicit is the right state right now.

---

## Phase E — Integration test (read-only on live instance)

- [ ] **E1.** Append to `zentaoAPI/client_integration_test.go` (build tag `integration`):
  ```
  TestIntegration_GetUser_Admin
    - GetUser(ctx, 1) — admin
    - Assert Account == "admin", Realname non-empty, ID == 1
  ```
  This is the only round-trip we can run on the current instance. Create/update/delete deferred to instances without the license cap + sudo gate.
- [ ] **E2.** Document in commit message that live coverage of write paths is httptest-only on this branch and remains so until an instance permits it.

---

## Phase F — Documentation

- [ ] **F1.** README.md: append "User (API client only)" subsection under Resources, explicitly note the verifyPassword gate caveat.
- [ ] **F2.** Stage-1 spec — leave as-is.
- [ ] **F3.** Probe doc already published at `docs/superpowers/specs/probe-user-controller.md`; reference from Stage 2 commit.

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
