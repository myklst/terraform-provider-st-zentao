# Probe — system (应用 / application) Controller transport

**Date:** 2026-05-22
**Server:** Max 8.1 (`systemMode: ALM`), `http://lek-ws.sige.la:8080/zentao`
**Transport:** Controller (PATH_INFO `.json`). `system` is controller-only on Max 8.x (V2 fallback returns `JSON_BUSINESS_FAIL`).

The `system` module's application entity (`browse` / `create` / `edit` / `active` / `inactive` / `delete`) — not the DevOps system-admin surface that shares the module name.

## Endpoint summary

| Action | Route | Method | Auth | Body | Notes |
|---|---|---|---|---|---|
| List all | `/system-showAll.json` | GET | `?zentaosid=` | — | `CtrlEnvelope`; `data.appList` map keyed by id. **Includes `deleted=1` rows.** |
| List by product | `/system-browse-{productID}.json` | GET | `?zentaosid=` | — | requires `productID` arg; `showAll` is the simpler list primitive |
| Read one | `/system-edit-{id}.json` | GET | `?zentaosid=` | — | `CtrlEnvelope`; `data.system` = full row, or `false` if id never existed |
| Find by name | `/system-getbyname-{base64(name)}.json` | GET | `?zentaosid=` | — | name is **base64-encoded** (PHP `base64_encode`); `CtrlEnvelope`; `data.appInfo` = full row, or `false` if no match. Matches by name only; does **not** filter tombstones. Custom endpoint. |
| Create | `/system-create-{productID}.json` | POST | `?zentaosid=` | form-urlencoded | `productID` is a **URL arg**, not a form key; bare `/system-create.json` errors `productID should pass value` |
| Update | `/system-edit-{id}.json` | POST | `?zentaosid=` | form-urlencoded | `CtrlSimpleResponse`; **not PATCH** — omitted form keys reset to default |
| Activate | `/system-active-{id}.json` | POST | `?zentaosid=` | — | sets `status=active` |
| Deactivate | `/system-inactive-{id}.json` | POST | `?zentaosid=` | — | sets `status=inactive` |
| Delete | `/system-delete-{id}.json` | POST | `?zentaosid=` | — | soft-delete (`deleted=1`); idempotent `result:success` on re-delete |

**Gotchas**
- JSON body to controller silently re-renders the form — **form-urlencoded only** for POSTs (confirmed pattern, not re-probed here).
- `showAll` / `edit-GET` return `text/html` content-type but a JSON body — decode regardless of content-type.
- **No hard 404.** Missing id → `data.system == false`; soft-deleted id → full row with `deleted=1`. Read not-found = `system==false OR system.deleted==1`.
- **`showAll` does not filter `deleted=1`.** Data source and any showAll-based read must drop `deleted==1` rows client-side.

## Fields

### Full row (`system-edit-{id}` GET → `data.system`)

```json
{"id":685,"name":"tfp-probe-edited","product":1,"integrated":0,"latestRelease":0,
 "latestDate":"","children":"","status":"active","desc":"hello-desc",
 "createdBy":"admin","createdDate":"2026-05-22 19:32:22","editedBy":"admin",
 "editedDate":"2026-05-22 19:32:37","deleted":0}
```

`showAll` row is slimmer: `{id,name,product,integrated,children,status,desc,deleted}`.

### Create form (POST `system-create-{productID}`)

| Key | Required | Observed |
|---|---|---|
| `name` | **yes** | create with `name` only succeeds; others default |
| `desc` | no | `''` |
| `integrated` | no | **server ignores it** — POSTing `integrated=1` left row at `integrated=0` |
| `children[]` | no | array form; stored as comma-string |

### Update form (POST `system-edit-{id}`)

| Key | Required | Observed |
|---|---|---|
| `name` | **yes** | |
| `desc` | no | resets to `''` if omitted |
| `children[]` | no | **resets to `""` if omitted** (M-Z merge required) |

`product` is **not** an edit-form key → immutable post-create (→ `RequiresReplace`). `integrated` and `status` are not mutated by the edit form.

## Field semantics

- **`product`** — int FK, set as create URL arg, immutable. → Required, RequiresReplace. Wire mechanism: URL path segment of `system-create-{productID}`.
- **`integrated`** — int 0/1. Server-owned: edit form cannot set it, and adding children did not flip it. → **Computed read-only**.
- **`children`** — comma-separated id string on the **parent** row (`"686"` for one child, `""` for none). Set via `children[]=<id>` array form keys. **Field-style FK on a shared parent column** (§6b-ter): edit-POST resets it when omitted, so resource_system's Update must M-Z-preserve it. Owned by the attachment resource.
- **`status`** — string enum. Observed: `active`, `inactive` (`wait` not observed). Toggled by `system-active` / `system-inactive`, **not** the edit form.
- **`name`** — Required. **Unique** across live *and* soft-deleted rows: create rejects a duplicate with `{"result":"fail","message":{"name":["...已经有...这条记录了..."]}}`. This makes `getbyname` (name-only) unambiguous, so post-create id lookup uses it instead of a full `showAll` scan. Because `getbyname` does not filter tombstones, a `deleted=1` hit or `appInfo:false` both read as ErrNotFound.
- **`desc`** — Optional; resets to `''` if omitted on edit.
- **`createdBy` / `createdDate`** — server, create-time, immutable.
- **`editedBy` / `editedDate`** — server, updated each edit.
- **`latestRelease` / `latestDate`** — server-derived (release linkage); not surfaced in v1.
- **`deleted`** — soft-delete tombstone; drives not-found detection, not surfaced.

## Response shapes (verbatim)

```text
create / edit / active / inactive / delete (CtrlSimpleResponse):
  {"load":true,"result":"success","message":"保存成功"}

read one (CtrlEnvelope):     {"status":"success","data":"{...\"system\":{...}...}"}
read one, missing id:        {"status":"success","data":"{...\"system\":false...}"}
list:                        {"status":"success","data":"{...\"appList\":{id:{...}}...}"}
```

`data` is a JSON-**string**; second-pass unwrap with `DecodeData`.

## Reconciliation table (plan → probe verdict)

| # | Plan decision | Verdict |
|---|---|---|
| 1 | Read = `system-edit-{id}` GET full row; `showAll` for DS | ✅ Confirmed. ➕ not-found = `system==false` OR `deleted==1`; `showAll` includes deleted rows. |
| 2 | `product` Required FK, RequiresReplace | ✅ Confirmed — URL arg `system-create-{productID}`, immutable. |
| 3 | `children` → attachment resource | 🔄 Field-style comma-string on parent. Edit-POST resets if omitted → main Update must M-Z preserve. Membership is **non-exclusive** (a child can sit in several parents' lists) and **additive** (probe: attaching the same child to two parents left both lists intact), so the attachment uses an **idempotent-adopt** Create (no refuse branch) rather than program's single-FK P3 guard. Modelled as `st-zentao_system_child_attachment(parent, child)`, both Required+RequiresReplace, id `{parent}-{child}`. |
| 4 | `integrated` Computed read-only | ✅ Confirmed — server ignores form `integrated`, not flipped by children. |
| 5 | `active` Optional+Computed toggle | 🔄 Wire column is **`status`** (string enum `active`/`inactive`). Per wire-name rule expose `status` String, not bool `active`. Toggle via active/inactive endpoints. **Re-grilled — see plan.** |
| 6 | name Required / desc O+C / dates Computed | ✅ Confirmed. + `created_by`/`edited_by` Computed. |
| 7 | DELETE semantics (was probe-deferred) | 🔄 Always `result:success` (idempotent); soft-delete `deleted=1`; no 404. Read on deleted → ErrNotFound. Release/build-refusal path unprobed (no associated rows); surface server `result:fail` message as-is if hit. |

## Implementation notes

- Decode `data` JSON-string with `DecodeData`; then unmarshal `system` / `appList`. `system` may be the literal `false` — decode into `*systemEditInner` (nil ⇒ not found) or check a sentinel before unmarshal.
- `json.Number` for `id`/`product`/`integrated`/`deleted` (mixed int/quoted shapes possible).
- `children` decode: comma-string → `[]string` of ids if surfaced; for resource_system keep the raw string baseline for M-Z preservation.
- Update must re-emit baseline `children[]` (each id as its own `children[]=` key) so the attachment is not wiped.
- Acc-test cleanup verification must filter `deleted==0` — `showAll` returns tombstones.

## Cleanup

Probe rows 685/686 soft-deleted (`deleted=1`). No hard-delete endpoint exists; tombstones remain in `showAll` by design.
