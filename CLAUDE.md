# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important

- Always reply in Simple Chinese

## Common commands

```bash
make go-lint             # golangci-lint run ./... — must be clean before committing
make go-test-unit        # unit tests, hermetic (httptest only)
make go-test-acc         # acceptance tests, sets TF_ACC=1, hits a real ZenTao
make install-local-custom-provider   # builds + installs into ~/.terraform.d/plugins
make generate-docs       # tfplugindocs via `go generate ./...`

# Run a single Go test
go test -race -run TestDoV2Request_HappyPath_TokenHeader ./zentaoAPI/...

# Live-server integration tests in zentaoAPI/ (build-tagged off the default run)
export ZENTAO_URL=http://your-zentao/zentao
export ZENTAO_ACCOUNT=admin
export ZENTAO_PASSWORD=...
go test -race -tags=integration -run TestIntegration ./zentaoAPI/...
```

The `.envrc.sample` shows the expected env-var names (`ZENTAO_URL`, `ZENTAO_ACCOUNT`, `ZENTAO_PASSWORD`) — direnv-friendly. The same vars are read by both the Terraform provider (under `TF_ACC=1`) and the Go integration tests.

## Architecture

Two top-level Go packages:

- **`zentao/`** — Terraform provider plugin: schema definitions, resource/datasource CRUD, Plugin Framework wiring (`provider.go` registers `st-zentao_product` and `st-zentao_program`).
- **`zentaoAPI/`** — HTTP client for ZenTao. The provider uses it; it has no Terraform dependencies and could be vendored independently.

### ZenTao has three API surfaces; we use all of them

The HTTP client supports all three because no single surface covers every entity:

| Surface | URL form | Auth carrier | Body | Expiry signal |
|---|---|---|---|---|
| **API V1** | `/api.php/v1/...` | `Token:` header | JSON | HTTP 401 or 403 |
| **API V2** | `/api.php/v2/...` | `Token:` header | JSON | HTTP 401 |
| **Controller** (PATH_INFO) | `/<module>-<method>-<args>.json` | `?zentaosid=` query | JSON or `application/x-www-form-urlencoded` | 302 → `user-login*` OR 200 + envelope reason "please login" / "请重新登录" |

**Critical invariants** (probed and documented in `docs/superpowers/specs/probe-controller-auth.md`):

- **Two independent credentials.** The API token store and the PHP session store are *separate*: `POST /api.php/v1/tokens` drives V1/V2 but its sessionID does **not** authenticate Controller routes (it never writes `$_SESSION['user']`). The client holds two credentials sharing one cookie jar:
  - `c.token` — `POST /api.php/v1/tokens`, carries V1/V2 (`Token:` header).
  - `c.ctrlSID` — two-step apilogin (`GET /api-getsessionid.json` → `GET /user-login.json?account&password&zentaosid`), carries Controller (`?zentaosid=` query). Apilogin success = `status:success` **and** a non-empty `user.account`; status alone is not enough (a form re-render also returns `status:success`).
  - Both are established eagerly in `NewClient` and refreshed independently (`refreshSession` / `refreshControllerSession`, each with its own mutex).
- V2 must **not** receive `?zentaosid=` query — Max 8.x mis-parses it on PUT as a record id, yielding `Unknown column` SQL errors. `sendHTTP`'s `injectZentaosid` flag guards this.
- Controller routes authenticate **exclusively** via `?zentaosid=` query (carrying `c.ctrlSID`) — cookie + Token header alone yields 302 → `/user-login`.
- The HTTP client's `CheckRedirect` is set to `http.ErrUseLastResponse` so the 302→login signal stays visible to `isControllerSessionExpired` instead of being silently followed.

### File layout in `zentaoAPI/`

Each transport owns its full request lifecycle. Shared plumbing lives in `client.go`:

```
client.go                  *Client struct (token + ctrlSID), NewClient, sendHTTP
                           (URL build + 5xx backoff), credential{observe,refresh},
                           doWithRefresh (observe-cred → send → detect-expiry → refresh → replay)
auth.go                    Login (POST /api.php/v1/tokens) + refreshSession (V1/V2 cred);
                           loginController (two-step apilogin) + refreshControllerSession
                           (Controller cred); credential lifecycle, transport-agnostic
errors.go                  ErrNotFound, ErrUnauthorized sentinels; *APIError
                           with password redaction; isNotFoundReason,
                           isUnauthorizedReason; zentaoFailReason
apiv1_transport.go         doV1Request + isV1SessionExpired (401/403)
apiv2_transport.go         doV2Request + isV2SessionExpired (401);
                           ZentaoResponse V2 envelope
controller_transport.go    controllerPath, doController, doControllerForm,
                           isControllerSessionExpired (302/please-login),
                           CtrlEnvelope, CtrlSimpleResponse, DecodeData,
                           classifyCtrlError, classifyCtrlSimple,
                           isLoginRedirectReason, CallController (escape hatch)

product.go / project.go    V2-backed typed wrappers (call doV2Request)
program.go                 Controller-backed typed wrappers (program-edit GET
                           is the read primitive — V2 echoes only ~24 fields,
                           controller surfaces the full ~70-field zt_project
                           row; see probe-program-controller.md)
user.go / group.go         Controller-backed typed wrappers
                           (V2 doesn't expose users / groups on Max 8.x)

client_integration_test.go //go:build integration — live server tests,
                           env-var-gated, off the default run
```

Test files mirror sources 1:1: every `*.go` has a co-named `*_test.go`. Shared test helpers (`newTestClient`, `mustParseURL`, the `v1Opts` / `newV1LoginServer` mock factory) live in `client_test.go` and `auth_test.go`.

### How the refresh loop works

`*Client.doWithRefresh(ctx, cred, isExpired, send)` is the shared "send → detect-expiry → refresh-once → replay" loop. Each transport's `doXRequest` is essentially a one-line dispatch into it, supplying the `credential` to use (`c.tokenCredential()` for V1/V2, `c.ctrlCredential()` for Controller), a transport-specific `isExpired` predicate, and a `send` closure that calls `sendHTTP` with the right body/contentType/`injectZentaosid` flag. The `credential{observe, refresh}` pair is what lets one loop serve both the V1-token and the apilogin sessionID without branching.

Concurrent expiry is serialised inside each credential's refresh func: `refreshSession` (guarded by `refreshMu`) re-runs `Login()`; `refreshControllerSession` (guarded by `ctrlRefreshMu`) re-runs `loginController()`. The first goroutine to acquire the lock performs the login; later goroutines re-check the credential under the lock and no-op if it's already been rotated.

### Path prefix constants

`apiV1PathPrefix` (in `apiv1_transport.go`) and `apiV2PathPrefix` (in `apiv2_transport.go`) are the canonical URL prefixes for the two REST surfaces. **All call sites that reference these surfaces must compose their paths from these constants** rather than hard-coding `"/api.php/v1/..."` / `"/api.php/v2/..."` strings:

- V1: `Login()` (`auth.go`) builds `apiV1PathPrefix + "tokens"`; future V1 wrappers should follow the same pattern.
- V2: `productsPath` / `programsPath` in `product.go` / `program.go` are derived as `apiV2PathPrefix + "products"` / `+ "programs"`; the per-id builders concatenate `"/<id>"` on top.

Tests assert on the exact wire path also via the constants (e.g. `gotPath != apiV1PathPrefix + "tokens"`), so a future change to either prefix only needs touching one line.

## Probe-driven development

When unsure how ZenTao behaves on a real server, write a curl probe and record findings in `docs/superpowers/specs/`. The current probe documents are the source of truth for "why the code is shaped this way":

- `probe-controller-auth.md` — auth flow + the two-credential model (token store vs PHP session store)
- `probe-user-controller.md` — User controller response shapes + verifyPassword sudo gate
- `2026-05-06-controller-extension-stage1.md` — design contract feeding the original Controller transport
- `2026-05-03-zentao-provider-design.md` — initial provider design

ZenTao Max 8.1 has several behavioural quirks that aren't documented upstream (e.g. `user-view` 302s to `user-todocalendar`, V2 `PUT` mis-parses `zentaosid`, `user-edit` POST with JSON silently re-renders the form). Always probe before assuming a surface works.

## Editing conventions specific to this repo

- **Don't reintroduce a generic `doRequest`** — the per-transport split exists deliberately so each surface owns its own URL prefix knowledge, body encoding, and expiry detector. A new transport gets a new `*_transport.go` file, not a flag added to a generic helper.
- **Rename impacts on tests are large** — the `c.token` / `c.ctrlSID` fields, `tokenMu`, `refreshMu`, `ctrlRefreshMu`, `backoffInitialInterval`, and `backoffMaxElapsed` are read directly by `newTestClient` and several tests. `newTestClient` seeds `token` and `ctrlSID` to the *same* value so Controller tests asserting `zentaosid=<token>` keep working; the `newV1LoginServer` factory also serves the apilogin routes (`api-getsessionid`, `user-login`) so `NewClient` completes. Renaming any of these touches multiple test files.
- **Integration test env vars** match `.envrc.sample` exactly — `ZENTAO_URL` / `ZENTAO_ACCOUNT` / `ZENTAO_PASSWORD` (no `_INTEGRATION_` infix). The same vars feed both Go integration tests and Terraform acceptance tests.
- **`CallController` is the escape hatch**, not the primary surface — typed wrappers should be added for any endpoint that gets called more than once.
- **Don't git push**
