# Probe: Controller Auth Behavior on ZenTao Max 8.1

**Date:** 2026-05-06
**Probe target:** `http://lek-ws.sige.la:8080/zentao/`
**Edition / Version:** Max 8.1 (`systemMode: ALM`)
**Probe operator:** Claude (automated curl probe)
**Verdict:** **β-unified** — single login via the v1 flow drives BOTH V2 and Controller paths via a cookie jar.

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

PATH_INFO routing means pretty URLs like `/zentao/product-all.json` are first-class; the query-string form `?m=&f=` exists but is rejected on auth (always 302 → login) regardless of cookie/token.

---

## Experiments

| # | Endpoint | Auth | Result | Conclusion |
|---|---|---|---|---|
| Login (V2) | `POST /api.php/v2/users/login` | account+password | 200; body has `token`; sets `zentaosid` cookie with same value | V2 login produces a token == cookie value |
| F | `GET /index.php?m=user&f=view&account=admin&t=json` | V2-login `zentaosid` cookie | **302 → /user-login** | V2 session does NOT authenticate Controllers |
| G | same | `Token` header only | **302 → /user-login** | Token header is ignored by Controller |
| H | same | none | **302 → /user-login** | (control) |
| I | same + `?zentaosid=` query param | V2 token via query | 200, but renders home page (not user view) — query-string Controller routing isn't honored on PATH_INFO server | Avoid query-string Controller URLs on this server |
| J | `GET /api-getsessionid.json` | none | 200, returns fresh `sessionID`; sets matching cookie | v1 session bootstrap still works |
| v1-2 | `GET /user-login.json?account=...&password=...&zentaosid=<sid>` | bootstrap cookie | 200, returns `{status:"success", token:<sid>, user:{...}}` (same shape as V2 login) | v1 apilogin works |
| v1-4 | `GET /product-all.json` | v1-login cookie | **200, JSON envelope `{status,data}`** | v1 session authenticates Controllers |
| L | `GET /api.php/v2/products/218` | v1-login cookie | 200, business response `{"status":"fail","message":"Product does not exist."}` (not 401) | v1 session ALSO authenticates V2 |
| L2 | same | `Token: <v1-sid>` header | identical 200 business response | v1 sessionID works as V2 token too |

---

## Conclusion

**v1 login is a strict superset of V2 login on this server.**

- V2 login → Token works for V2 endpoints; cookie does NOT work for Controllers.
- v1 login → cookie works for Controllers AND for V2 (the v1 sessionID also functions as a V2 token).

Therefore the client should:

1. Replace the current `POST /api.php/v2/users/login` with the **v1 two-step flow**:
   - `GET /api-getsessionid.json` (parse `sessionID` from string-encoded `data`).
   - `GET /user-login.json?account=…&password=…&zentaosid=<sid>` (verify `status:success`).
2. Maintain a `http.Client.Jar` (`net/http/cookiejar`) so subsequent requests (V2 OR Controller) auto-send `zentaosid`.
3. Continue setting `Token: <sessionID>` header on V2 requests (defensive — works the same).
4. `isSessionExpired` must now also recognise:
   - 302 redirect to `*/user-login*` (Controller's "please login" signal).
   - HTTP 200 + `{"status":"failed","reason":"please login"}` (legacy v1 envelope).
   - HTTP 401 (V2 envelope, unchanged).

## Observed envelope shapes (for response-parsing design)

- **V2 endpoints** (existing): flat JSON, fields at top level. `{"status":"success", ...resource...}` or `{"status":"fail","message":"..."}`.
- **Controller endpoints (PATH_INFO `.json`)**: nested. `{"status":"success","data":"<JSON-encoded string>","md5":"..."}` — the `data` value is a **string** that needs a second `json.Unmarshal` to obtain the resource shape. Sometimes also returned as a direct object, per ZenTao module.
- v1 login is an exception: the `user` object is a direct JSON object at top level (alongside `token`), NOT under `data`.

## Implications for design (feeds Q4–Q9)

- URL builder needs PATH_INFO form: `<webRoot>/<module>-<method>[-<arg1>[-<arg2>...]].<viewType>`. Separator `-` (server's `requestFix`). View type: `.json` for read; for writes, action (e.g. `product-create.json`) usually accepts form-encoded body via POST.
- Response parser needs a `decodeData` helper for the `string-encoded JSON in data` shape.
- Login flow becomes two HTTP round-trips. `Login()` and `refreshSession` must coordinate both (existing serialization via `refreshMu` carries over unchanged).
- Auth probe artefacts retained at `/tmp/probe-*` during the probe session; not committed.

---

## Addendum — V1 endpoint compat (probed 2026-05-08)

**Probe target:** same as above (`http://lek-ws.sige.la:8080/zentao/`, Max 8.1).
**Probe operator:** Claude (via terraform-provider-st-zentao /grill-me session).
**Trigger:** the original probe (above) tested whether the Controller-flavoured two-step login's sessionID drives V2; it did NOT test ZenTao's RESTful **API V1** surface (`/api.php/v1/...`, distinct from "v1 apilogin" naming used in the table above for the legacy two-step login). The 2026-05-08 probe filled that gap and led to the current implementation switching from the two-step flow to V1's documented `POST /api.php/v1/tokens`.

### Experiments

| # | Endpoint | Auth | Result | Conclusion |
|---|---|---|---|---|
| V1-1 | `GET /api.php/v1/products` | none | 403 Forbidden + `Set-Cookie: zentaosid=…` | V1 surface exists; expiry signal is **403**, not 401 |
| V1-2 | `POST /api.php/v1/tokens` | bad creds | 400 + `{"error":"登录失败，请检查您的用户名或密码是否填写正确。"}` | V1 login error envelope is `{"error":"<reason>"}`; classify via `isUnauthorizedReason` |
| V1-3 | `POST /api.php/v1/tokens` | good creds | 201 + `{"token":"<32-hex>"}` + `Set-Cookie: zentaosid=<32-hex>` | V1 login is one round-trip and emits both Token (body) and zentaosid (cookie) values |
| V1-4 | two-step sessionID as `Token:` header → `GET /api.php/v1/products` | two-step SID | 200 + `{"page","total","limit","products":[…]}` | **Controller two-step sessionID also authenticates V1** — the token store is shared |
| V1-5 | V1 token (from V1-3) as `Token:` header → V1 endpoint | V1 token | 200 + business JSON | V1 token works on its own surface (sanity) |
| V1-6 | V1 token as `Token:` header → Controller endpoint | V1 token | **302 → /user-login** | V1 token does NOT pass cookie-style auth on Controller |
| V1-7 | V1 token as `?zentaosid=<v1tok>` → Controller endpoint | V1 token via query | 200 | V1 token DOES authenticate Controller when passed via query — same as a sessionID |
| V1-8 | V1 token as `Token:` header → V2 endpoint | V1 token | 200 | V1 token works on V2 too |
| V1-9 | V1 token → V2 CRUD lifecycle (POST/PUT/DELETE on `/api.php/v2/products`) | V1 token | All 200 + success envelopes | V1 token works on V2 writes (only GET had been tested before) |

### Conclusion

**Controller-flavoured two-step sessionID and V1 token are interchangeable** — the strings differ (each login flow emits a fresh value) but ZenTao's token store is shared across all three transports. Either credential, passed through its transport-specific carrier (`Token:` header for V1/V2, `?zentaosid=` query for Controller), authenticates any of the three surfaces.

This unblocks the simplification adopted in `feat/zentao-controller-extension`:

1. `Login()` now uses **`POST /api.php/v1/tokens`** (one round-trip, documented contract) instead of the legacy two-step flow.
2. The single sessionID drives all three transports — no per-transport login is needed.
3. Per-transport expiry detection is independent: V1 uses 401/403, V2 uses 401, Controller uses 302→user-login or 200+please-login envelope.

The two-step flow is documented here for archival reasons but no longer exercised by the production client.
