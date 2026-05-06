# Controller Extension Stage 1 — Design Contract

**Date:** 2026-05-06
**Branch:** `feat/zentao-controller-extension`
**Status:** Stage 1 implemented (commits `e2b279f` → `187fa39`).
**Supersedes parts of:** `docs/superpowers/specs/2026-05-03-zentao-provider-design.md` §4.4–§4.6, §5.1, §5.3.
**See also:** `docs/superpowers/specs/probe-controller-auth.md` (auth probe verdict), `docs/superpowers/plans/2026-05-06-zentao-controller-extension.md` (full plan).

---

## What Stage 1 changed

The original MVP (commits `66019ef`/`85ac8b0`) authenticated via the V2 single-shot `POST /api.php/v2/users/login` and used `Token` headers on every V2 REST call. That stayed inside ZenTao's V2 surface, which covers only a handful of entities (`product`, `program`, …).

The auth probe ([probe-controller-auth.md](probe-controller-auth.md)) showed two facts that drive Stage 1:

1. **V2 login is API-only.** Its session cannot authenticate ZenTao's PATH_INFO Controller routes (which is where every UI action — and therefore every entity not on V2 — lives).
2. **The legacy v1 two-step login *is* a strict superset.** Its sessionID drives BOTH Controller calls AND V2 endpoints.

Stage 1 therefore replaces the V2 login with the v1 two-step flow and adds a Controller transport layer alongside the existing V2 path. Both transports flow through the same auth/refresh/retry pipeline.

## Auth — β-unified flow

```
NewClient
  └─► Login()
        ├─► step 1: GET /api-getsessionid.json
        │     server: bootstraps PHP session, sets zentaosid cookie,
        │             returns sessionID in string-encoded `data`
        │     client: cookiejar captures cookie automatically
        │
        └─► step 2: GET /user-login.json?account=&password=&zentaosid=<sid>
              server: validates credentials; on success repeats the
                      (possibly rotated) sessionID under `token`
              client: cookiejar updates; c.token = response.token
```

Per-request flow:

```
doRequest
  ├─► send (V2 path or Controller path — URL composition is the only diff)
  │     - http.Client.Jar attaches zentaosid cookie automatically
  │     - http.Client.CheckRedirect = ErrUseLastResponse so 302→login
  │       is visible (not silently followed)
  │     - Token: <sid> header set defensively (V2 still respects it,
  │       Controller ignores it benignly)
  │
  └─► isSessionExpired(status, body, location)
        ├─► HTTP 401                                        → expired
        ├─► HTTP 302/301/303 + Location contains user-login → expired
        └─► HTTP 200 + envelope status≠success + reason
            matches isLoginRedirectReason                    → expired
```

`refreshSession` mu+double-check semantics carry over verbatim from the V2-only design — concurrent expiries still trigger exactly one Login.

## Two transports, one pipeline

```
*Client
 ├── doRequest(method, path, query, body)              ← shared transport
 │     └── send (auth headers + cookie + retry + redirect-noop)
 │
 ├── product/program V2 wrappers          → doRequest with v2 paths
 │   (api.php/v2/products/{id}, etc.)
 │
 └── Controller transport                 → doRequest via doController
      └── doController(module, method, pathArgs, query, body)
            └── controllerPath: <module>-<method>[-<arg>...].json
                  body nil → GET, else POST + Content-Type: application/json
```

Public escape hatch: `(c *Client) CallController(...)` mirrors `doController`'s signature, exported for cases not yet wrapped by a typed method. Marked **EXPERIMENTAL** — its surface may change as typed wrappers come online.

## Envelopes — two coexisting shapes

| Envelope | Where used | Shape |
|---|---|---|
| `ZentaoResponse` | V2 endpoints (`product.go`, `program.go`) | flat: `{status, error/message/reason, ...resource fields at top level}` |
| `CtrlEnvelope` | Controller endpoints (future entity files; today only `CallController` callers) | nested: `{status, error/message/reason, data: <JSON-encoded string OR direct object>, md5}` |

These are deliberately **NOT** merged. Their shapes diverge enough that a unified type would require special-casing every site of use. Both reuse the internal `zentaoFailReason(error, message, reason)` helper.

`DecodeData(env CtrlEnvelope, target any)` unwraps the dual `data` shape (string-encoded JSON OR direct object/array OR null/empty).

## Errors

Same sentinels (`ErrNotFound`, `ErrUnauthorized`) and same `*APIError` struct cover both transports. Two envelope-specific classifiers coexist:

- V2 path: `apiError(status, body)` (in `product.go`).
- Controller path: `classifyCtrlError(status, env, body)` (in `types.go`).

`ErrUnauthorized` is reserved for **clear bad-credentials** signals. Other auth-related failures (malformed envelope, network glitch during login) bubble up as `*APIError` so `errors.Is(err, ErrUnauthorized)` reliably means "the credentials really are no longer accepted."

Reason-matching helpers in `types.go`:

| Helper | Recognises |
|---|---|
| `isNotFoundReason` | "not exist", "not found" |
| `isLoginRedirectReason` | "please login", "session expired", "请重新登录", "请登录" |
| `isUnauthorizedReason` | "wrong"/"incorrect"/"invalid", "密码错误", "登录失败", "认证" |

## Deferred extraction — the `callCtrl` template

When typed wrappers for `user`, `project`, `execution` arrive in Stage 2, each one will repeat the same envelope-decode + status-check + `DecodeData` + error-classify plumbing — likely 5–10 lines per method. **This is intentional.**

The plan commits to extracting that plumbing into a private `callCtrl` template helper **only after at least two typed entities have landed** (current target: user + project). Reason: the abstraction's correct shape becomes obvious from two real call sites, not from one. If we extract it now from the single user case, the abstraction will warp around user-specific concerns (password handling, possible form-encoding fallback) and force project to break it.

Future readers who see verbose Controller wrappers in Stage 2 should not "clean them up" with a shared helper before project (or whichever second entity) lands. Once two are in place, the refactor is mechanical and safe.

## Things deliberately deferred to Stage 2 or beyond

- Typed wrappers for `user`, `project`, `execution`.
- Audit pass over `product`/`program` for Controller-only actions (close/activate/link/etc.).
- Form-encoded body fallback (`doControllerForm`) — add only if the first write-class Controller (likely `user-create.json`) rejects JSON. Probe didn't verify writes.
- Multilingual reason coverage beyond the strings already observed. As more locales surface in Stage 2 testing, append to `isNotFoundReason` / `isLoginRedirectReason` / `isUnauthorizedReason`.
- Resource-layer (`zentao/`) work for new entities — separate plan after typed wrappers land.

## Verification at end of Stage 1

| Layer | Command | Result |
|---|---|---|
| Unit (zentaoAPI) | `go test -race -cover ./zentaoAPI/...` | green; pkg coverage 84.6%; core helpers ≥94% |
| Acceptance (zentao) | `TF_ACC=1 go test -race ./zentao/...` (Phase B end) | green in 52.6s — proved v1 Login covers V2 endpoints unchanged |
| Integration (live) | `go test -tags=integration ...` | gate added; live re-run blocked at end of Phase D by environment outage on test instance (MySQL unreachable from app container, unrelated to branch) |
