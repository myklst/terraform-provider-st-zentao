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
