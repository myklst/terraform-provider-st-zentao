# Probe: product Controller (PATH_INFO `.json`)

Probed against ZenTao Max 8.1 at `http://lek-ws.sige.la:8080/zentao` on 2026-05-10.

This spec captures the wire contract used by `zentaoAPI/product.go` for
the Controller-backed CRUD path. The V2 REST surface (`/api.php/v2/products`)
is **not** used — see §7 for why we moved off V2.

## 1. Endpoint summary

| Op | Route | HTTP | Body | Response shape |
|---|---|---|---|---|
| Read | `product-view-{id}.json` | GET | — | `CtrlEnvelope` (existing id) **or** `CtrlSimpleResponse`-shaped `{result, load.alert}` (missing id) |
| Create | `product-create.json` (or `product-create-{programID}.json`) | POST | `application/x-www-form-urlencoded` | `{result, message, id}` |
| Update | `product-edit-{id}.json` | POST | `application/x-www-form-urlencoded` | `CtrlSimpleResponse` |
| Delete | `product-delete-{id}.json` | GET | — | `CtrlSimpleResponse` |

All routes authenticate via `?zentaosid=<token>` query param per the
Controller auth contract (see `probe-controller-auth.md`).

## 2. Read — `product-view-{id}.json` GET

The view route returns **two distinct response shapes** — one for
existing ids, one for missing ids. The wrapper sniffs the discriminator
on `status` (present on shape #1) vs `result` + no `data` (shape #2).

### 2a. Existing id — `CtrlEnvelope` (shape #1)

Outer envelope (3 fields):

```json
{
  "status": "success",
  "data":   "<JSON-encoded string, ~14KB — see §2a.1>",
  "md5":    "<32-char hex digest of data>"
}
```

`data` is a **JSON-encoded string** (not a nested object — must be
`json.Unmarshal`-ed a second time). The `md5` field is the hash of the
inner string and is not consumed by the wrapper.

#### 2a.1 Inner object — top-level keys

After `data` is decoded as a string and re-parsed, the inner object has
**12 top-level keys**, of which the wrapper consumes only **one**:

| Key | Type | Wrapper uses? | Notes |
|---|---|---|---|
| `product` | object | **YES** — `productCtrlWire`, see §2a.2 | The row itself (~66 fields) |
| `title` | string | no | UI page title, e.g. `"LB \| Back Office-产品概况"` |
| `products` | object `{id: name}` | no | Sibling-product map for the UI's left-rail switcher; ~hundreds of entries on a busy server |
| `workflowGroups` | object `{id: name}` | no | Lookup table for the `workflowGroup` field |
| `actions` | object `{id: action}` | no | Audit log: opened/edited/closed events scoped to this product |
| `dynamics` | array of action objects | no | Recent activity feed (cross-resource, e.g. project events) |
| `users` | object `{login: display}` | no | Username → display-name lookup |
| `groups` | object `{id: name}` | no | Permission-group id → name lookup (used by the `groups` field on `product`) |
| `branches` | array | no | Branch list when `type=branch`/`platform`; `[]` for `type=normal` |
| `reviewers` | array/object | no | Reviewer roster scoped to this product |
| `members` | array/object | no | Team members scoped to this product |
| `pager` | object \| null | no | Always `null` on a single-row read |

#### 2a.2 The `data.product` object — field-by-field

The `product` object surfaces the full row from the `zt_product` table.
Unlike programs / projects / executions / sprints / stages / kanbans
(which all share the polymorphic `zt_project` table discriminated by
`type`), ZenTao stores products in their own dedicated `zt_product`
table. On
`product-view`, the row carries **66 fields**. All numerics may arrive
as native int OR as JSON-quoted strings (a ZenTao quirk) — the
wrapper's `productCtrlWire` uses `json.Number` to tolerate both.

##### Identity & lifecycle (15 fields)

| Field | Wire type | Meaning | Wrapper exposes? |
|---|---|---|---|
| `id` | int / string-int | Primary key in `zt_product` | ✅ `Product.ID int64` |
| `program` | int / string-int | Parent program id (foreign key into `zt_project` row of `type=program`) | ✅ `Product.Program int64` |
| `line` | int / string-int | Product-line id, or `0` | ✅ `Product.Line int64` |
| `name` | string | Display name | ✅ `Product.Name string` |
| `code` | string | Short code; auto-assigned by server, often `""` | ✅ `Product.Code string` (read-only) |
| `type` | string | `normal \| branch \| platform` | ✅ `Product.Type string` |
| `status` | string | `normal \| closed` | ✅ `Product.Status string` |
| `subStatus` | string | UI-only sub-status label, almost always `""` | ❌ ignored |
| `vision` | string | `rnd` (R&D) or `lite`; tenant-mode flag | ❌ ignored |
| `order` | int | Display sort order in the products list | ❌ ignored |
| `deleted` | int / string-int | `0` = live, `1` = soft-deleted | **internal**: `wire.Deleted == "1"` → `ErrNotFound` |
| `closedDate` | string | `"YYYY-MM-DD HH:MM:SS"` when `status=closed`, else `""` | ❌ ignored |
| `createdBy` | string | Username of creator | ✅ `Product.CreatedBy string` (read-only) |
| `createdDate` | string | `"YYYY-MM-DD HH:MM:SS"` | ✅ `Product.CreatedDate string` (read-only) |
| `createdVersion` | string | ZenTao version that created the row, e.g. `"max8.1"` | ❌ ignored |

##### Content (4 fields)

| Field | Wire type | Meaning | Wrapper |
|---|---|---|---|
| `desc` | string | Free-form description (HTML allowed) | ✅ `Product.Desc string` |
| `acl` | string | `open` (anyone with product-view can access) / `private` (PO/QD/RD/program-PM/whitelist only) | ✅ `Product.ACL string` |
| `groups` | string | Comma-joined permission-group ids, e.g. `"1,2,5"` (read shape; submitted as `groups[]=1&groups[]=2` per `filter:join`) | ✅ `Product.Groups []string` (split via `flexibleStringList`) |
| `whitelist` | string | Comma-joined usernames (same shape as `groups`) | ✅ `Product.Whitelist []string` |

##### Roles (7 fields)

| Field | Wire type | Meaning | Wrapper |
|---|---|---|---|
| `PO` | string (account) | Product Owner username | ✅ `Product.PO string` |
| `QD` | string (account) | QA Lead username | ✅ `Product.QD string` |
| `RD` | string (account) | Release Lead username | ✅ `Product.RD string` |
| `reviewer` | string | Comma-joined reviewer usernames | ✅ `Product.Reviewer []string` (split) |
| `PMT` | string | Comma-joined Product Management Team; column exists on the row but `form.php` does not surface it for write | ❌ ignored |
| `feedback` | string (account) | Account that handles inbound feedback tickets | ❌ ignored |
| `ticket` | string (account) | Account that handles inbound user tickets | ❌ ignored |

##### Workflow (3 fields)

| Field | Wire type | Meaning | Wrapper |
|---|---|---|---|
| `workflowGroup` | int / string-int | Workflow id (default `1` = `默认流程`); FK to `workflowGroups` lookup | ❌ ignored |
| `shadow` | int | Internal flag for shadow products (used by ZenTao's "ditto" feature); always `0` for normal products | ❌ ignored |
| `bind` | int | Internal binding flag for cross-product story links | ❌ ignored |

##### Derived counters (~30 fields, server-recomputed every read)

The `xxxEpics` / `xxxRequirements` / `xxxStories` / `xxxBugs` families
each have 7 status buckets (`draft`, `active`, `changing`, `reviewing`,
`finished`, `closed`, `total`). Plus `unresolvedBugs`, `closedBugs`,
`fixedBugs`, `totalBugs`, `plans`, `releases`. **All read-only,
recomputed by the server on every read** — they change whenever anyone
in ZenTao touches a story/bug/plan that points at this product, even
without touching the product row itself. None are exposed on the Go
struct or the Terraform schema (would cause perpetual drift).

##### View-only derived fields (10 fields)

These appear on `product-view` only — they're aggregates the UI uses
for the "product overview" page:

| Field | Wire type | Meaning |
|---|---|---|
| `programName` | string | Program's display name (joined from the parent program row) |
| `projects` | int | Number of related projects |
| `executions` | int | Number of related sprints/stages |
| `progress` | int / string-int | Computed completion percentage (0–100) |
| `storyDeliveryRate` | int | Closed-story / total-story ratio (0–100) |
| `bugs` | int | Outstanding bug count (UI shorthand for `unresolvedBugs`) |
| `cases` | int | Test-case count |
| `docs` | int | Linked-doc count |
| `stories` | object `{"":N, "draft":N, "reviewing":N, "active":N, "changing":N, "closed":N}` | Story-status histogram (different shape from the flat `xxxStories` counters!) |
| `builds` | int | Build count |

None are exposed on the Go struct or Terraform schema for the same
"perpetual drift" reason as the regular counters.

#### 2a.3 Multi-value field encoding

Fields backed by `form.php` `type:array, filter:join` (`reviewer`,
`groups`, `whitelist`, `PMT`) come back as **comma-joined strings on
read**, e.g. `"reviewer":"admin,PM"`. On write the wrapper submits them
as PHP-array form params (`reviewer[]=admin&reviewer[]=PM`); the server
applies `filter:join` and stores them comma-joined. The wrapper's
`flexibleStringList` decoder splits on `,` and trims whitespace, so
both `"a,b"` and `["a","b"]` round-trip into `[]string{"a","b"}`.

### 2b. Missing id — `CtrlSimpleResponse`-shaped envelope

When `{id}` does not exist, `product-view` switches envelope shape:

```json
{
  "result": "success",
  "load": {
    "alert": "对象不存在！",
    "locate": "/zentao/product-all.json"
  }
}
```

No `data` field, no `status` field — the wrapper uses
`probe.Status == "" && len(probe.Data) == 0` as the discriminator and
returns `ErrNotFound` immediately.

### 2c. Soft-deleted rows

`product-view` faithfully echoes soft-deleted rows in the shape #1
envelope with `"deleted":1` (no fallback substitution to a sibling
product). The wrapper checks `wire.Deleted == "1"` and surfaces these
as `ErrNotFound` so Terraform Read clears the resource from state.

## 3. Create — `product-create.json` POST

URL forms:
- `product-create.json` — server picks default program from session
- `product-create-{programID}.json` — bind to a specific program (the
  URL arg sets the form's default but is overridden by an explicit
  `program=` field in the body, so we always send `program=` and use
  the bare path for simplicity)

Required form fields (per `fields` map on the create form):
- `name` (string, `filter:trim`)

All other fields are optional; server falls back to `form.php`
defaults. Fields the wrapper writes: `program, line, PO, QD, RD,
reviewer[], type, status, desc, acl, groups[], whitelist[],
feedback, ticket, workflowGroup`.

Multi-value fields (`reviewer`, `groups`, `whitelist`) are
`type:array, filter:join` — submit as `field[]=v1&field[]=v2` and the
server stores them as comma-joined strings. The wire returns the
comma-joined string on read.

Response on success:

```json
{"result":"success","message":"保存成功","id":230}
```

`id` is the new row's autoincrement — **echoed directly**, no
post-create lookup needed (unlike `group-create`).

## 4. Update — `product-edit-{id}.json` POST (NON-PATCH)

Response on success:

```json
{"result":"success","message":"保存成功","load":"/zentao/product-view-230.json"}
```

### 4a. Non-PATCH semantic (CRITICAL)

A POST that omits a `form.php` writable field resets that column to
its `form.php` default. Verified empirically: editing a product with
only `name=newname` cleared `program → 0, PO → "", QD → "", RD → "",
desc → "", reviewer → ""` while preserving `acl, type, status` (which
happened to match defaults).

**Mitigation**: `UpdateProduct` GETs the baseline, copies all writable
fields, overrides only the caller-set non-zero values, then submits
the full form. Same M-Z merge as `program.go` (`mergeProgramBaseline`).

## 5. Delete — `product-delete-{id}.json` GET

Both `product-delete-{id}.json` and `product-delete-{id}-yes.json`
return success on this server (no confirmation prompt, unlike
`program-delete`). Use the bare form for simplicity.

Response on success:

```json
{"load":"/zentao/product-all.json","result":"success","message":"保存成功"}
```

The wrapper is idempotent on already-deleted ids: the server returns
the same success envelope for already-soft-deleted rows.

## 6. form.php field reference

From the `fields` map echoed by `product-create.json` GET (the same
form contract drives both create and update):

| Field | type | required | default | control | filter |
|---|---|---|---|---|---|
| name | string | true | — | text | trim |
| program | int | false | 0 / 1 | select | — |
| line | int | false | 0 | select | — |
| PO | account | false | "" | select | — |
| QD | account | false | "" | select | — |
| RD | account | false | "" | select | — |
| reviewer | array | false | "" | multi-select | join |
| type | string | false | normal | select | — |
| status | string | false | normal | select / hidden | — |
| desc | string | false | "" | editor | — |
| acl | string | false | private / open | radio | — |
| groups | array | false | "" | multi-select | join |
| whitelist | array | false | "" | multi-select | join |
| feedback | account | false | "" | select | — |
| ticket | account | false | "" | select | — |
| workflowGroup | int | false | 0 / 1 | select | — |
| changeProjects | string | false | "" | hidden | — |

`type` options: `"" | normal | branch | platform`.
`status` options: `normal | closed`.
`acl` options: `open | private`.

## 7. Why we moved off V2

V2 `PUT /api.php/v2/products/{id}` works for simple field updates but
lacks coverage for some fields the Controller exposes (e.g.
`workflowGroup`, `groups`, `whitelist` accept different shapes on
write). The Controller route is also the canonical write path used by
the ZenTao UI, so it gets first-class server validation and
filter-side processing (e.g. the `filter:join` semantics for
multi-select arrays). Standardising on Controller for product matches
the existing decision for program/group/user.
