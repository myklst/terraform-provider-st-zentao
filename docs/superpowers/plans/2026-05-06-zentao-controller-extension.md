# ZenTao Controller Extension — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-05-06
**Branch:** create new `feat/zentao-controller-extension` off `main`. Do not commit on `main`.
**Spec inputs:**
- `docs/superpowers/specs/2026-05-03-zentao-provider-design.md` (original MVP spec)
- `docs/superpowers/specs/probe-controller-auth.md` (auth probe — proves β-unified)
- This plan (synthesises the 2026-05-06 grill-me session decisions)

**Goal:** Extend `zentaoAPI` so it can call ZenTao Controller endpoints (PATH_INFO `.json`) for full module coverage — V2 API V2 alone covers too few modules. **Stage 1** of this plan delivers the low-level transport (`doController`) plus the public escape hatch (`CallController`) plus all shared response/error helpers; **Stage 2** is a placeholder for typed-wrapper rollout (user → program-audit → product-audit → project → execution) tracked by future plans.

**Strategy chosen (D from grill Q1):** two-stage delivery. This plan covers stage 1 only.

**Coexistence semantics (B from grill Q2):** V2 + Controller paths coexist per entity. Existing `product.go` / `program.go` (V2) are NOT rewritten; they continue to work and only become callable on the new auth flow.

---

## Confirmed design decisions

### Auth — β-unified
- Replace `POST /api.php/v2/users/login` with v1 two-step flow:
  1. `GET /api-getsessionid.json` → parse string-encoded `data.sessionID` from outer envelope
  2. `GET /user-login.json?account=&password=&zentaosid=<sid>` → verify `status:success`
- `http.Client.Jar = cookiejar.New(nil)` so `zentaosid` rides every subsequent request automatically.
- `http.Client.CheckRedirect = http.ErrUseLastResponse` so 302→login is visible to `isSessionExpired`.
- `Token` header continues to be set on V2 calls (defensive — v1 sessionID also functions as V2 token, proven by probe TEST L).
- `isSessionExpired` extended to recognise three "session-gone" signals:
  1. HTTP 401 (V2)
  2. HTTP 302 with `Location` containing `user-login`
  3. HTTP 200 + body has `status:fail` + reason matches `please login` / similar (use `isLoginRedirectReason`)

### URL & method
- PATH_INFO with hard-coded `.json` viewType: `<webRoot>/<module>-<method>[-<arg>...].json`.
- Position-arg signature (no name lookup), `query map[string]string` retained for rare extras, JSON body default.
- `body == nil` → GET; otherwise POST.
- Delete's `?confirm=yes` quirk lives in typed wrappers, NOT in `doController`.

### Response & errors
- New `CtrlEnvelope{Status, Error, Message, Reason, Data json.RawMessage, MD5}` for Controller path. Coexists with existing `ZentaoResponse` (V2 path) — they are NOT merged.
- New `DecodeData(env CtrlEnvelope, target any)` handles three shapes: string-encoded JSON in `data`, direct object in `data`, null/empty.
- Shared helpers in `types.go`: `zentaoFailReason`, `isNotFoundReason`, `isLoginRedirectReason`, `isUnauthorizedReason`.
- New `classifyCtrlError(httpStatus, env, raw)` for Controller failure mapping. Coexists with V2's `apiError`. Both reuse `zentaoFailReason`.
- `ErrUnauthorized` strictly = clear bad-credentials. Other auth-related failures → `*APIError`.
- `APIError` fields unchanged. Call context comes via caller `fmt.Errorf` wrapping.
- No new `ErrSessionExhausted` sentinel; keep `*APIError("session refresh exhausted")`.

### API surface
- `CallController` exported with godoc EXPERIMENTAL warning. Same signature as `doController`.
- File-per-entity convention preserved. New entity files (stage 2) follow `product.go` / `program.go` shape.
- All methods hang on single `*Client`. Naming rule: `<Verb><Entity>` (`GetUser`, `CreateProject`, `CloseExecution`, `AssignToTask`, …).
- **Deferred extraction:** do NOT abstract a `callCtrl` template helper in stage 1. After stage 2 lands at least two typed entities (user + project), refactor recurrent envelope/status/decode plumbing into `callCtrl`. This commitment is part of the design contract.

### Tests
- Minimal `newMockZentaoServer` helper in `client_test.go` covers v1-login boilerplate only. Other handlers stay inline.
- One build-tag integration test gated on `ZENTAO_INTEGRATION_URL`. CI does NOT run it.
- Coverage target: package-level 85% (existing); core helpers (`DecodeData`, `isSessionExpired`, `classifyCtrlError`, `controllerPath`) ≥95%.
- Manual TF_ACC=1 product/program acceptance run is the v1-login regression gate before stage-1 close.

---

## File inventory (stage 1)

### Modified
- `zentaoAPI/client.go` — `cookiejar.New(nil)`; `CheckRedirect=ErrUseLastResponse`; `isSessionExpired` signature extended (httpStatus, body, location); `send` plumbs `Location` header through; new private `doController`.
- `zentaoAPI/auth.go` — `Login` rewritten to v1 two-step; `loginV1Wire` dedicated struct (NOT routed through `CtrlEnvelope`); `refreshSession` mu+double-check semantics retained verbatim.
- `zentaoAPI/types.go` — add `CtrlEnvelope`, `DecodeData`, `classifyCtrlError`; promote `isNotFoundReason` from `product.go`; add `isLoginRedirectReason`, `isUnauthorizedReason`, internal `zentaoFailReason(error,message,reason)`.
- `zentaoAPI/auth_test.go` — rewrite all login mocks to two-step v1; add Login-fail-classification table; refreshSession unchanged but probe through new `Login`.
- `zentaoAPI/client_test.go` — `newMockZentaoServer` factory (v1 login only); cookiejar persistence verified across requests; `CheckRedirect` wiring verified; isSessionExpired three-case table; concurrent N×302 single-refresh test (mirror existing V2 concurrent 401 test).
- `zentaoAPI/product_test.go` / `zentaoAPI/program_test.go` — login-fixture replacement (use `newMockZentaoServer`); business cases unchanged.
- `zentaoAPI/types_test.go` — add `isLoginRedirectReason` / `isUnauthorizedReason` tables.

### New
- `zentaoAPI/controller.go` — `doController`, `CallController`, `controllerPath`.
- `zentaoAPI/controller_test.go` — happy GET/POST; `controllerPath` boundary table; `DecodeData` five-shape table; `classifyCtrlError` three-branch table; `CallController` public-signature sanity.
- `zentaoAPI/client_integration_test.go` — single test `TestIntegration_V1Login_AndCallController`, build tag `//go:build integration`.

### Untouched
- `zentao/` package (provider/resources). β-unified is invisible to it.
- `zentaoAPI/product.go` / `zentaoAPI/program.go` — call sites unchanged.

---

## Phase A — Types, helpers, error model

- [ ] **A1.** Promote `isNotFoundReason` from `product.go` to `types.go`. Verify all current callers compile.
- [ ] **A2.** Add internal `zentaoFailReason(error_, message, reason string) string`. Refactor `ZentaoResponse.ZentaoFailReason` to delegate.
- [ ] **A3.** Add `isLoginRedirectReason(reason string) bool` (matches `please login` / `请重新登录` / similar Chinese localisations).
- [ ] **A4.** Add `isUnauthorizedReason(reason string) bool` (matches `wrong` / `incorrect` / `invalid` / `密码错误` / `认证`).
- [ ] **A5.** Add `CtrlEnvelope` struct with `Status / Error / Message / Reason / Data json.RawMessage / MD5`. Add method `(e CtrlEnvelope) ZentaoFailReason() string` delegating to shared helper.
- [ ] **A6.** Add `DecodeData(env CtrlEnvelope, target any) error`. Handle: empty `Data`, `null`, leading-quote string-encoded JSON, leading-`{` direct object, leading-`[` direct array. Return descriptive error otherwise.
- [ ] **A7.** Add `classifyCtrlError(httpStatus int, env CtrlEnvelope, rawBody []byte) error`. Maps not-found → `ErrNotFound`, unauthorized → `ErrUnauthorized`, default → `*APIError`.
- [ ] **A8.** Update `types_test.go`:
  - `DecodeData` table: 5 shapes (V1/V2/null/empty-string/non-JSON-error).
  - `isLoginRedirectReason` table.
  - `isUnauthorizedReason` table.
  - `classifyCtrlError` table: not-found / unauthorized / generic-fail.

**Verification (Phase A):** `go test -run 'Test(Decode|Classify|IsLogin|IsUnauth|IsNotFound)' ./zentaoAPI/...` green.

---

## Phase B — Auth rewrite to v1 two-step

- [ ] **B1.** Add `loginV1Wire` struct in `auth.go` (Status, Token, raw user blob unused).
- [ ] **B2.** Add `getsessionidWire` private struct (outer status + string-encoded data → `{sessionID}`).
- [ ] **B3.** Rewrite `Login(ctx)`:
  1. `GET <baseURL>/api-getsessionid.json` → parse outer; `DecodeData`-equivalent on inner data string → extract `sessionID`. Persist nothing yet — cookie jar already grabbed `zentaosid`.
  2. `GET <baseURL>/user-login.json?account=&password=&zentaosid=<sid>` → parse `loginV1Wire`. On `status!=success`, classify reason: unauthorized vs APIError. On success: `c.token = wire.Token` (under `tokenMu`).
  3. Both steps use the cookiejar-backed `c.http`.
- [ ] **B4.** Extend `isSessionExpired` signature to `(httpStatus int, body []byte, location string) bool`. Implement three-case match (401 / 302+login Location / 200+please-login reason).
- [ ] **B5.** Refactor `client.go:send` to capture `resp.Header.Get("Location")` and feed it to `isSessionExpired`. Backoff path returns location alongside status.
- [ ] **B6.** Refactor `client.go:doRequest` so the post-send dispatch matches the new isSessionExpired signature. Refresh+replay logic unchanged in shape.
- [ ] **B7.** Add cookie jar to `NewClient`:
  ```go
  jar, _ := cookiejar.New(nil)
  c.http = &http.Client{
      Timeout: 30 * time.Second,
      Jar:     jar,
      CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
  }
  ```
- [ ] **B8.** Rewrite `auth_test.go`:
  - `newMockZentaoServer` factory (under `client_test.go`) — exposes login handler that responds correctly to both `/api-getsessionid.json` and `/user-login.json`. Optional `loginFail` knob to inject failure shapes.
  - Login happy path (jar receives zentaosid; `c.token` populated).
  - Login fail at step 1: malformed envelope → `*APIError`.
  - Login fail at step 2: `status:fail` + bad-creds reason → `ErrUnauthorized`.
  - Login fail at step 2: `status:fail` + generic reason → `*APIError`.
  - Refresh: 302+login Location triggers refresh+single-replay → success.
  - Refresh: 200+please-login reason triggers refresh → success.
  - Refresh exhausted → `*APIError("session refresh exhausted")`.
  - Concurrent 10× simultaneous expiry → exactly one `Login` (use atomic counter on test handler).
- [ ] **B9.** Update `client_test.go` to verify cookiejar persistence across two consecutive `doRequest` calls + verify CheckRedirect prevents auto-follow.

**Verification (Phase B):** `go test -race ./zentaoAPI/...` green for all auth/client tests. `product_test.go` / `program_test.go` may be temporarily red — fixed in Phase D.

---

## Phase C — Controller transport

- [ ] **C1.** Add `controllerPath(webRootPath, module, method string, args []string) string` in `controller.go`. Behaviour:
  - Trailing-empty args trimmed; middle-empty args preserved as empty segments.
  - Output e.g. `/zentao/user-view-admin.json`, `/zentao/product-all-noclosed--25-1.json`.
  - Unit-tested against probe-observed forms.
- [ ] **C2.** Add private `doController(ctx, module, method string, pathArgs []string, query map[string]string, body any) (raw []byte, status int, err error)`:
  - Compose URL via `controllerPath`.
  - Apply `query` parameters (URL-encode).
  - If `body != nil`: JSON-marshal, `Content-Type: application/json`, `POST`. Else `GET`.
  - Reuse `doRequest`'s send/refresh pipeline by delegating: extract the lower-level helper if needed so V2 and Controller share identical retry+refresh semantics.
- [ ] **C3.** Add public `CallController(ctx, module, method, pathArgs, query, body) (raw, status, err)` — thin pass-through. Godoc:
  ```go
  // CallController is an EXPERIMENTAL escape hatch for invoking ZenTao Controller
  // endpoints not yet covered by typed wrappers. Prefer typed methods (GetUser,
  // CreateProject, …) when available; this surface may change without notice.
  ```
- [ ] **C4.** Write `controller_test.go`:
  - `controllerPath` table: zero-arg / one-arg / trailing-empty / middle-empty / pre-encoded chars.
  - `doController` GET happy: server asserts URL shape + sees no body; client decodes envelope.
  - `doController` POST happy: server asserts Content-Type `application/json` + JSON body equality; client decodes envelope.
  - `doController` query: server asserts query params present.
  - `doController` 302→login triggers refresh path (smoke-only — refresh covered fully in B8).
  - `CallController` public sanity: 1 happy path + 1 failure path.

**Verification (Phase C):** `go test -coverprofile=cover.out ./zentaoAPI/...`; `go tool cover -func=cover.out | grep -E '(controllerPath|DecodeData|classifyCtrlError|isSessionExpired)'` shows ≥95% per function.

---

## Phase D — Stabilise V2 path under new auth

- [ ] **D1.** Update `product_test.go` to bootstrap with `newMockZentaoServer` (v1 login fixture) instead of the old V2 login mock. Business test cases unchanged.
- [ ] **D2.** Same for `program_test.go`.
- [ ] **D3.** Run `go test -race -cover ./zentaoAPI/...`. Confirm package-level coverage ≥85% with `go tool cover -func=cover.out | tail -1`.
- [ ] **D4.** Add `client_integration_test.go`:
  ```go
  //go:build integration
  ```
  - One test `TestIntegration_V1Login_AndCallController`.
  - Reads `ZENTAO_INTEGRATION_URL`, `ZENTAO_INTEGRATION_ACCOUNT`, `ZENTAO_INTEGRATION_PASSWORD`. `t.Skip` if absent.
  - `NewClient` → succeeds (proves Login works end-to-end on real instance).
  - `CallController(ctx, "product", "all", nil, nil, nil)` → expects `status:success` envelope; sanity-decode `data` via `DecodeData` to a generic `map[string]any`.
- [ ] **D5.** **Manual gate before merge:** run product/program acceptance suite locally with `TF_ACC=1` against the test instance. Document outcome in PR description.

**Verification (Phase D):** Full `go test -race -cover ./zentaoAPI/...` green. Manual `TF_ACC=1` run green.

---

## Phase E — Documentation

- [ ] **E1.** Update `docs/superpowers/specs/2026-05-03-zentao-provider-design.md`:
  - Adjust §4.4–§4.6 to match β-unified Login + isSessionExpired three-case logic.
  - Add new §4.9 "Controller transport" describing `doController` / `CallController` / `CtrlEnvelope` / `DecodeData`.
  - Adjust §6.1 error taxonomy to mention `classifyCtrlError`.
- [ ] **E2.** Add a one-page note `docs/superpowers/specs/2026-05-06-controller-extension-stage1.md` summarising the design contract — most importantly the **deferred `callCtrl` extraction** commitment so future readers know why early Controller wrappers are verbose.
- [ ] **E3.** Update `README.md` Architecture section to describe two transport paths (V2 / Controller) and note `CallController` is EXPERIMENTAL.
- [ ] **E4.** Do NOT regenerate tfplugindocs in this stage — no Terraform-visible surface changes.

---

## Phase F — Pre-merge checklist

- [ ] **F1.** `go build ./...` clean.
- [ ] **F2.** `go vet ./...` clean.
- [ ] **F3.** `go test -race -cover ./zentaoAPI/...` green; package coverage ≥85%; core helpers ≥95%.
- [ ] **F4.** `go test -race ./zentao/...` green (unit only).
- [ ] **F5.** Manual `TF_ACC=1 go test ./zentao/...` against the integration instance green (D5).
- [ ] **F6.** Manual `go test -tags=integration -run TestIntegration_V1Login_AndCallController ./zentaoAPI/...` green.
- [ ] **F7.** PR description references both spec docs (design + probe) and this plan.
- [ ] **F8.** Commit hygiene: one commit per phase (A/B/C/D/E), squash optional. No commits on `main`.

---

## Out of scope (this plan)

The following are explicitly stage-2 or later — separate plan(s):

- Typed wrappers for `user`, `project`, `execution`.
- Audit pass over existing `product` / `program` to identify Controller-only actions worth wrapping (close, activate, link, etc.).
- `callCtrl` template extraction (deferred per design decision).
- Form-encoded body fallback (`doControllerForm`) — add only if user entity rejects JSON during stage 2.
- Resource-layer (`zentao/`) work for new entities — separate plan after typed wrappers land.

---

## Risks & open questions tracked

1. **Real-instance JSON-body acceptance for write-class Controllers.** Probe only verified read paths. First write attempt during stage 2 (likely `user-create.json`) will validate the JSON-body assumption. Mitigation: `doControllerForm` fallback documented above; flip switch is one new helper, no architecture change.

2. **Multilingual `please login` / not-found reasons.** This instance returns Chinese strings. `isLoginRedirectReason` / `isNotFoundReason` need to recognise common Chinese forms. Capture observed strings during stage 2 testing and extend the helpers.

3. **Cookie jar semantics across `Client` reuse.** A long-lived `Client` accumulates cookies; if ZenTao rotates `zentaosid` mid-session (observed once during probe — `Set-Cookie` after V2 login set TWO different `zentaosid` values), the jar may carry stale cookies. Verify by inspecting jar state in test B9; if rotation matters, swap to per-request jar reset on Login.
