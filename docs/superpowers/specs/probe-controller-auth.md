# Probe: Controller Auth Behavior on ZenTao Max 8.1

**Probe target:** `https://zentao.sige-test.com` (Max 8.1, K8s single-replica, HTTPS).
**Edition / Version:** Max 8.1 (`systemMode: ALM`)
**Probe operator:** Claude (automated curl probe)
**Verdict:** **two disjoint credentials** — the API token store and the PHP session store are separate; V1/V2 and Controller each need their own credential.

---

## Server config (from `/index.php?mode=getconfig`)

```
version       : max8.1
requestType   : PATH_INFO
requestFix    : "-"
moduleVar     : m
methodVar     : f
viewVar       : t
sessionVar    : zentaosid
sessionName   : zentaosid
expiredTime   : 43200    # 12h
systemMode    : ALM
```

PATH_INFO routing means pretty URLs like `/zentao/product-all.json` are first-class. Controller routes authenticate **exclusively** via `?zentaosid=` query — cookie + `Token:` header alone yield 302 → `/user-login`.

---

## Auth model

`POST /api.php/v1/tokens` registers an **API token** that drives V1 and V2, but does **not** write `$_SESSION['user']`. The Controller's `checkPriv` reads only `$_SESSION['user']`, so it is authenticated by a separate **PHP session** established via the two-step apilogin flow. The two surfaces require two disjoint credentials.

| Credential | Established by | Drives | Carrier | Expiry signal |
|---|---|---|---|---|
| `c.token` | `POST /api.php/v1/tokens` | V1 + V2 | `Token:` header | 401 (V2), 401/403 (V1) |
| `c.ctrlSID` | two-step apilogin (`GET /api-getsessionid.json` → `GET /user-login.json?account&password&zentaosid`) | Controller | `?zentaosid=` query | 302 → `/user-login` OR 200 + "please login" envelope |

Both share one cookie jar.

---

## Experiments

| # | Step | Credential | Result | Inference |
|---|---|---|---|---|
| S-1 | `POST /api.php/v1/tokens` | good creds | 201 + `{"token":"<32hex>"}` + `Set-Cookie: zentaosid=<same 32hex>` | token == cookie value |
| S-2 | V1 token as `Token:` header → `GET /api.php/v2/products` | V1 token | **200** | V1 token authenticates V2 ✓ |
| S-3 | V1 token as `?zentaosid=` → Controller | V1 token | **302 → /user-login** | V1 token does NOT authenticate Controller ✗ |
| S-4 | V1 token as `zentaosid` cookie → Controller | V1 token | **302 → /user-login** | cookie form fails too — not a carrier issue |
| S-5 | apilogin sid (`api-getsessionid` → `user-login` GET) → Controller via `?zentaosid=` | apilogin sid | **200** | apilogin sid authenticates Controller ✓ |
| S-6 | apilogin sid as `Token:` header → V1 / V2 | apilogin sid | **401** | apilogin sid does NOT authenticate V1/V2 ✗ |

---

## Key facts

- **Two independent credentials.** Neither authenticates the other's surface. The client establishes both eagerly in `NewClient` and refreshes them independently (`refreshSession` / `refreshControllerSession`, each with its own mutex). `doWithRefresh` takes a `credential{observe, refresh}` so the shared send→detect→refresh→replay loop serves both.
- **apilogin success = `status:success` AND a non-empty `user.account`.** The success body is flat: `{"status":"success","token":"<sid>","user":{"account":"admin",...}}`. A failed login / silent form re-render also returns `status:success` but with **no `user`** — so status alone is not the success signal. Plaintext password works (no rand/md5 needed for the GET apilogin form).
- **V2 must NOT receive `?zentaosid=` query** — Max 8.x mis-parses it on PUT as a record id, yielding `Unknown column` SQL errors. `sendHTTP`'s `injectZentaosid` flag guards this.
- **`CheckRedirect` is `http.ErrUseLastResponse`** so the 302→login signal stays visible to `isControllerSessionExpired` instead of being silently followed.

---

## Observed response shapes

- **V1 login** (`POST /api.php/v1/tokens`): 201 + `{"token":"<32-hex>"}`; error is 400 + `{"error":"<reason>"}`.
- **V2 endpoints**: flat JSON, fields at top level. `{"status":"success", ...resource...}` or `{"status":"fail","message":"..."}`. Expiry is 401.
- **Controller endpoints** (PATH_INFO `.json`): nested. `{"status":"success","data":"<JSON-encoded string>","md5":"..."}` — the `data` value is a **string** needing a second `json.Unmarshal`. Sometimes returned as a direct object, per module.
- **apilogin** (`GET /user-login.json`): flat envelope with `user` object at top level (alongside `token`), NOT under `data`.

## URL forms

- **V1/V2:** `<webRoot>/api.php/v1/...`, `<webRoot>/api.php/v2/...`.
- **Controller:** `<webRoot>/<module>-<method>[-<arg1>[-<arg2>...]].<viewType>`. Separator `-` (server's `requestFix`). `.json` for reads; writes (e.g. `product-create.json`) accept a form-encoded body via POST.
