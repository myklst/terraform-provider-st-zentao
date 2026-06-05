# Probe: ZenTao Project Controller surface

**Date:** 2026-05-16
**Server:** ZenTao Max 8.x at `${ZENTAO_URL}` (lek-ws.sige.la:8080)
**Tool:** raw `curl` via `direnv exec .`
**Cleanup:** all probe-created project ids deleted (verified by re-view returning `已被删除`).

> Source of truth for migrating `zentaoAPI/project.go` from V2 (`/api.php/v2/projects`) to the Controller transport (PATH_INFO `.json`). The existing V2 surface is functional but the codebase has converged on Controller for write-heavy entities (`program`, `product`, `user`, `group`) — this probe is the basis for bringing `project` into that family.

## 1. Endpoint summary

| Endpoint | Status | Notes |
|---|---|---|
| `GET project-view-{id}.json` | works (read primitive) | Wraps `data.project` JSON; alive project returns 75 fields verbatim. Side effect: `module/project/ext/control/view.php` runs `UPDATE zt_project SET workflowGroup=... WHERE id={id}` — benign on project-type rows where the column has a valid value; **crashes with `SQLSTATE[23000]` if id refers to a non-project row** (sprint/program/execution sharing `zt_project`). Don't feed cross-type ids. |
| `GET project-edit-{id}.json` | works | Returns the same inner `.project` plus heavy UI prefill metadata (`linkedProducts`, `branchPairs`, `workflowGroups`, `productPlans`, …). Equivalent for `GetProject`; we prefer `project-view` for the leaner payload. |
| `POST project-create.json` | works | Form-urlencoded body; returns `{result, message, id, load?}` — id inline, no separate lookup needed. |
| `POST project-edit-{id}.json` | works | Form-urlencoded body; returns `{result, message, load?}`. **Non-PATCH** — name-only POST fails `productsBox` validation, so M-Z merge must replay `products[]` (and all other writeable fields) from baseline. |
| `GET project-delete-{id}.json` | works | Bare form succeeds — unlike `program-delete` which requires the positional `-yes` suffix, `project-delete` accepts both. Idempotent: missing-id and already-deleted both return `{result:success, closeModal:true, load:"..."}`. |
| `GET project-delete-{id}-yes.json` | works | Equivalent to the bare form. |

## 2. form.php field set (create)

Empirical required set (validated against live Max 8.x; upstream source review not done in this probe):

| Field | Required | Notes |
|---|---|---|
| `name` | **yes** | trimmed |
| `begin` | **yes** | date |
| `end` | **yes** | date |
| `model` | **yes** | `scrum` / `waterfall` / `kanban` |
| `workflowGroup` | **yes** | int — must match an existing workflow group id (see `project-edit GET` → `workflowGroups[]`) |
| `acl` | **yes** | `open` / `private` / `custom` |
| `products[]` | **yes (≥1)** | PHP array — `products[]=1&products[]=2`. Synthetic validator `productsBox` fires when array is empty/missing |
| `hasProduct` | recommended | observed as `1` in all live rows |
| `multiple` | optional, **create-only** | "迭代开关". **Wire is HTML checkbox semantics — form value `on` (checked) / `off` (unchecked), NOT `1`/`0`/`yes`/`no`.** Probe matrix: `multiple=on` → stored `1` ✓; `multiple=off` → stored `0` ✓; `multiple=1` / `multiple=yes` → stored `0` (form filter only matches the literal `"on"`); omitted → `0`. Persisted as int `0`/`1` in `zt_project`. **Settable only on `project-create`; `project-edit` POST does not change it** (per project owner). Wrapper needs a custom `boolToOnOff` serializer for `toForm()` (program's `boolToIntStr` won't work). TF resource attribute should have `RequiresReplace` so a flip triggers destroy+create. |
| `parent` | optional | parent **program** id (zt_project is a hierarchy: program > project > sprint). `parent=0` for top-level project; positive int attaches under that program. Server recomputes `path` and `grade` |
| `PM` | optional | PM username |
| `desc` | optional | rich-text body |
| `budget` / `budgetUnit` | optional | budget="0.00" budgetUnit="CNY" by default |
| `whitelist[]` | optional | only meaningful when `acl=custom` |

### Create response shapes

Success:
```json
{"result":"success","message":"保存成功","id":70}
```

Validation failure (heterogeneous shapes):
```json
{"result":"fail","message":{"end":"『计划完成』不能为空。"}}
{"result":"fail","message":{"workflowGroup":["『项目流程』不能为空。"]}}
{"result":"fail","message":{"productsBox":"最少关联一个产品"}}
{"result":"fail","message":{"products[0]":"最少关联一个产品"}}
```

The `message` map's value can be **string OR `[]string`**, and the field key can be **`field` OR `field[N]`** (with array index). The existing `CtrlSimpleResponse.FieldErrors()` only decodes `map[string][]string` — it must be extended to also accept `map[string]string`, or per-field flat-string values get silently dropped.

## 3. Controller GET shape (`project-view-{id}.json`)

For id=28 (real project) the inner `.project` returned the full `zt_project` row:

```json
{
  "id": 28, "project": 0, "isTpl": 0, "charter": 0,
  "model": "scrum", "type": "project", "category": "", "lifetime": "",
  "budget": "0.00", "budgetUnit": "CNY", "attribute": "",
  "percent": "0.00", "milestone": 0, "output": "", "auth": "extend",
  "storyType": "", "parent": 4, "path": ",1,4,28,", "grade": 3,
  "name": "LB - Maintenance Project", "code": "",
  "hasProduct": 1, "workflowGroup": 2,
  "begin": "2015-01-01", "end": "2099-12-31",
  "firstEnd": "", "realBegan": "", "realEnd": "", "days": 0,
  "status": "wait", "subStatus": "", "pri": 1, "desc": "",
  "version": 1, "parentVersion": 1, "planDuration": 0, "realDuration": 0,
  "progress": "0.00", "estimate": "0.00", "left": "0.00", "consumed": "0.00",
  "teamCount": 0, "market": 0,
  "openedBy": "admin", "openedDate": "...", "openedVersion": "",
  "lastEditedBy": "admin", "lastEditedDate": "...",
  "closedBy": "", "closedDate": "", "closedReason": "",
  "canceledBy": "", "canceledDate": "", "suspendedDate": "",
  "PO": "", "PM": "", "QD": "", "RD": "", "team": "...",
  "acl": "private", "whitelist": "",
  "tplAcl": "open", "tplWhiteList": "",
  "order": 140, "stageBy": "product",
  "displayCards": 0, "fluidBoard": 0, "multiple": 0, "parallel": 0,
  "enabled": "on", "linkType": "plan", "taskDateLimit": "auto",
  "colWidth": 264, "minColWidth": 200, "maxColWidth": 384,
  "coverExecutionPriv": 1, "vision": "rnd", "frozen": "", "deleted": 0
}
```

Numeric columns mix native ints (`id`, `parent`, `multiple`, `deleted`, …) and stringified decimals (`budget`, `progress`, `estimate`, …) — `json.Number` locals handle both.

**Fields the wrapper should surface** (writeable + identity + key system fields):
`ID`, `Name`, `Model`, `Type`, `Begin`, `End`, `Parent`, `Products` (read from join — see §5), `WorkflowGroup`, `Multiple`, `ACL`, `PM`, `Desc`, `Deleted`.

**Fields to OMIT** (audit / derived / presentation internals — same reasoning as program/product):
`project`, `isTpl`, `charter`, `category`, `lifetime`, `attribute`, `percent`, `milestone`, `output`, `auth`, `storyType`, `code`, `hasProduct`, `path`, `grade`, `firstEnd`, `realBegan`, `realEnd`, `days`, `status`, `subStatus`, `pri`, `version`, `parentVersion`, `planDuration`, `realDuration`, `progress`, `estimate`, `left`, `consumed`, `teamCount`, `market`, `openedBy`, `openedDate`, `openedVersion`, `lastEditedBy`, `lastEditedDate`, `closedBy`, `closedDate`, `closedReason`, `canceledBy`, `canceledDate`, `suspendedDate`, `team`, `whitelist`, `tplAcl`, `tplWhiteList`, `order`, `stageBy`, `displayCards`, `fluidBoard`, `parallel`, `enabled`, `linkType`, `taskDateLimit`, `colWidth`, `minColWidth`, `maxColWidth`, `coverExecutionPriv`, `vision`, `frozen`, `budget`, `budgetUnit`.

Reasoning per memory `feedback_use_state_for_unknown_hazard`: server-derived columns (`percent`, `progress`, `teamCount`, `path`, `grade`, `realBegan`, …) would trip the inconsistent-after-apply trap if surfaced as Computed in Terraform. `code` is UI-only. `status` belongs to a separate lifecycle controller path (`project-start` / `project-suspend` / `project-close`). `whitelist`/`tplAcl` aren't needed by current consumers.

## 4. Response envelope shapes

### GET success

```json
{"status":"success","data":"{\"title\":\"...\",\"project\":{...full row...},\"workflowGroup\":{...},...}"}
```

`data` is a JSON-encoded string; `CtrlResp.DecodeData` unwraps it. Only inner `.project` is consumed.

### GET missing id (HTTP 200, NOT 404)

```json
{"result":"success","load":{"alert":"您无权访问该项目！","locate":"\/zentao\/project-browse.json"}}
```

Distinctive: `status` field is absent, top-level shape uses `result`. Wrapper detects this and surfaces `ErrNotFound`.

### GET soft-deleted id (HTTP 200)

```json
{"result":"fail","load":{"alert":"抱歉，您访问的项目已被删除。","locate":"\/zentao\/project-browse.json"}}
```

Different `result` value (`fail` vs `success`) and `alert` text from missing, but the same semantic answer: `ErrNotFound`. Wrapper should match the alert text (`已被删除` OR `无权访问该项目`) to collapse both to `ErrNotFound`.

### POST create / edit success

```json
{"result":"success","message":"保存成功","id":70,"load":"\/zentao\/project-browse.json"}
```

`id` is present on create, absent on edit. `load` is sometimes a string redirect, sometimes a structured object — `CtrlSimpleResponse.Load` is `json.RawMessage` and we don't consume it.

### POST validation fail

See §2 — mix of `{field: "str"}` and `{field: ["str"]}` requires extending `FieldErrors()`.

### DELETE success / idempotent

```json
{"result":"success","closeModal":true,"load":"\/zentao\/project-browse.json"}
```

Returned for: actually-deleted, already-deleted (re-delete), and missing-id. Server-side idempotency is total; wrapper passes through.

## 5. `products[]` association

`products` is **not** a column on `zt_project` — it's a many-to-many via `zt_projectproduct`. The Controller surface still exposes it via `project-view` / `project-edit`, just not in the inner `.project` payload:

- On **read**, `project-view-{id}.json` returns the linked products as a map at **`.data.products`** (top-level, sibling of `.data.project`). Map key = product id (string), value = full product row joined from `zt_product`. `project-edit-{id}.json` GET exposes the same data as **`.data.linkedProducts`** (different key, same content). Verified by creating a project with `products[]=1` only — both endpoints returned a single-element map `{"1": {...}}`.
- On **create**, `products[]=1&products[]=2` form input populates the join table.
- On **edit (POST)**, omitting `products[]` triggers `productsBox` validation error (the synthetic validator runs against the join table state under the implicit "must have ≥1" rule). M-Z merge must read the products list from baseline (`.data.products`) and replay it on every edit.

**`GetProject` reads via `project-view` alone — one request, one envelope.** The earlier worry about needing a follow-up `project-edit` call to fetch products was based on misreading the inner `.project` payload; `.data.products` is the authoritative source.

## 6. Code path for the wrapper (recommended sketch)

```go
// GetProject → doController("project", "view", {id})
//   1) parse envelope, detect missing/soft-deleted via load.alert
//   2) DecodeData → top-level .project (row fields) + top-level .products
//      (linked-product map) → Project struct via UnmarshalJSON

// CreateProject → doControllerForm("project", "create", nil, form)
//   - require Name/Begin/End/Model/WorkflowGroup/ACL/Products non-nil
//   - parse {result,message,id} response, surface id, re-GET
//   - on result=fail with heterogeneous message map, classifyCtrlSimple
//     after extending FieldErrors() to accept map[string]string too

// UpdateProject → fetch baseline (GetProject + linkedProducts), merge, POST
//   - mergeProjectBaseline pattern mirroring mergeProductBaseline
//   - re-GET after success

// DeleteProject → doController("project", "delete", {id})
//   - no -yes suffix needed (bare form accepted)
//   - any 200 with envelope IsSuccess() OR not-found reason → return nil
```

## 7. Open questions / follow-ups

1. ~~Multi-execution toggle~~ — **resolved**: form value is `on`/`off`, see §2. Memory updated.
2. **`linkedProducts` shape under multi-branch products**: probe-project-controller didn't cover branch-aware product binding (the `branchPairs` field hints at it). If TF resource needs to support branched products, follow-up probe required.
3. **Lifecycle methods** (`project-start` / `project-suspend` / `project-close` / `project-activate`) are out of scope for this refactor — keep the `status` field excluded from the wrapper struct, document in the resource that lifecycle transitions need a separate Terraform action (or are simply unsupported).

## 8. Probe cleanup audit

Created during probe: `tfp-probe-*` series ids 70, 72, 74, 76, 78, 80.
All deleted (verified by post-cleanup `project-view-{id}` returning `已被删除`).
