# Probe: User Controller on ZenTao Max 8.1

**Date:** 2026-05-06
**Probe target:** `http://lek-ws.sige.la:8080/zentao/`, admin/123456
**Probe operator:** Claude (automated curl probe)
**Verdict:** User controller is partially testable on this instance — read works; write paths gated by license cap (create) and a verifyPassword sudo gate (update/delete) whose hashing scheme could not be reverse-engineered without source access.

---

## What works

| Action | Route | Method | Auth | Notes |
|---|---|---|---|---|
| Read user (any id) | `user-edit-<id>.json` | GET | `?zentaosid=<token>` | Returns `{status:success, data:"<JSON-string>"}` whose inner has `user` field with the full row plus form context (depts/groups/visions/companies). This is the ONLY way to read a user in this version — `user-view-<id\|account>.json` redirects to `user-todocalendar-<id>.json` for every account, so view is unusable as a read primitive. |

`user-edit-<id>` GET inner shape (relevant keys):

```
inner = {
  title, companies, depts, groups, userGroups, visions,
  pager, rand,            // CSRF nonce — unused by our flow
  user: {
    id, company, type, dept, account, password, role, realname,
    superior, pinyin, nickname, commiter, avatar, birthday, gender,
    email, skype, qq, mobile, phone, weixin, dingding, slack,
    whatsapp, address, zipcode, nature, analysis, strategy, join,
    visits, visions, ip, last, fails, locked, feedback, ranzhi,
    ldap, score, scoreLevel, resetToken, resetExpired, clientStatus,
    clientLang, jira, deleted
  }
}
```

The `password` field is the stored hash (`md5(plaintext)` for admin, observed: `e10adc3949ba59abbe56e057f20f883e` = md5("123456")).

## What is blocked

### Create — license cap

```
POST /user-create.json?zentaosid=<token>
  body (JSON or form): { account, password, realname, ... }
  → 200 {"result":"fail","message":"系统用户人数已达授权的上限，不能继续添加用户！","load":"/zentao/company-browse.json"}
```

The Max edition's licensed user count is full on this instance. Live create CANNOT be tested here. Httptest unit coverage will have to substitute.

### Update — verifyPassword sudo gate

`POST /user-edit-<id>.json` with form-urlencoded body returns:

```
{"result":"fail","message":{"verifyPassword":["验证失败，请检查您的系统登录密码是否正确"]}}
```

regardless of whether `verifyPassword` is sent as plaintext, `md5(plaintext)`, `md5(md5(pw)+rand)` (using the bootstrap rand), `md5(stored+formRand)`, or the session token. Without ZenTao source access (declined by user) the hashing scheme could not be reverse-engineered. **Live update on this instance is therefore not feasible**; httptest unit coverage will have to substitute.

The same gate likely applies to delete (untested — would need a non-self disposable user, which is also license-capped).

### Other actions probed and unavailable

```
user-browse.json   → 500: "the module user method does not exist"
user-all.json      → 500: same
user-admin.json    → 500: same
user-view-<x>.json → 302 → user-todocalendar-<x>.json (always)
user-create.json POST (JSON body)        → license fail
user-create.json POST (form body)        → license fail
user-edit-<id>.json POST (JSON body)     → form re-rendered, body ignored
user-edit-<id>.json POST (form body)     → verifyPassword gate
user-delete-99999.json?confirm=yes       → 200 with "user: false" (controller renders "missing row" success envelope)
```

The verifyPassword gate is **specific to the user controller**: probing `project-create.json` / `program-edit-1.json` GET on the same session works without any sudo gate.

## Three response shapes observed

The Stage 1 design knew about two (`ZentaoResponse` for V2, `CtrlEnvelope` for Controller). The user controller surfaced a third:

| Shape | Where | Example |
|---|---|---|
| **A.** Stage-1 known | Controller READ envelopes (`user-edit-<id>` GET, `product-all`, ...) | `{"status":"success","data":"<JSON-encoded>"}` |
| **B.** Stage-1 known | V2 endpoints | `{"status":"success","product":{...}}` (fields top-level) |
| **C.** **NEW** | Controller WRITE / form-submit responses | `{"result":"success\|fail","message":"..." or {field: [errs]},"load":"<redirect>"}` |

Shape C uses `result` (not `status`), and on validation failure the `message` field becomes a **map of field-name → array-of-error-strings** rather than a string. The Stage 1 `CtrlEnvelope` cannot decode this.

## Auth mechanism quirk surfaced

The probe also confirmed (and the stage-1 fix in commit `4b94e8c` records) that **Controller routes authenticate via `?zentaosid=<token>` query parameter exclusively**. Cookie + Token header alone yields 302 → `/user-login...`. V2 routes (`api.php/v2/...`) must NOT carry that query param on writes — Max 8.x mis-parses it as a record id in unique-check SQL on PUT.

## Direct implications for Stage 2 slice 1 plan

1. **Add `CtrlSimpleResponse` (or `CtrlWriteEnvelope`)** — a third envelope type for shape C, with `Result string` and `Message json.RawMessage` (so `message` can be either `string` or `map[string][]string`).
2. **Add a `doControllerForm` private helper** — write operations on user controller require form-urlencoded bodies; JSON is silently ignored.
3. **Read primitive shifts from `view` to `edit GET`** — `GetUser(ctx, id int)` calls `user-edit-<id>.json` (GET), then decodes inner `user` field. Account-keyed lookup (`GetUserByAccount`) probably needs a search call we haven't probed yet — defer.
4. **Live integration coverage** — only `GetUser` round-trip can be live-verified. `CreateUser`/`UpdateUser`/`DeleteUser` rely on httptest. The build-tag integration test for slice 1 should be limited to a Get against admin (id=1) followed by field assertions.
5. **`User.Password` write path** — must include `verifyPassword` as an additional write field whose value the caller has to supply (we expose the field; we don't know how to hash it server-side correctly, so the caller is responsible). On instances WITHOUT the sudo gate it can be omitted.
6. **License-cap awareness** — `CreateUser` must surface the `用户人数已达授权的上限` message cleanly so callers know to bump licensing rather than retry.

## Unresolved questions (carried forward)

- verifyPassword exact hashing scheme on Max 8.1 (probe failed; deferred to source-access opportunity or a different test instance).
- What `user-delete` returns when actually deleting a removable user (probe could only test against a missing id — got `data.user: false` "no-op" envelope). Real delete envelope shape is unknown.
- `userCtrlWire` field types: probe showed `id` as INT (e.g., `2`), `dept` as INT (`500`), `password` as string. But other actions may serialise these differently. Use `json.Number` defensively per existing convention.
