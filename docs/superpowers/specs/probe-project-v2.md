# Probe: ZenTao V2 `/projects` surface

**Date:** 2026-05-09 (initial probe), addended 2026-05-09 with upstream-source corrections
**Server:** ZenTao Max 8.x at `${ZENTAO_URL}` (lek-ws.sige.la:8080)
**Tool:** raw `curl` via `direnv exec .`, plus upstream `easysoft/zentaopms` source review
**Session:** all probes use a single `POST /api.php/v1/tokens` sessionID; no refresh observed.
**Cleanup:** all probe-created project IDs deleted; final `GET /projects` filter on `tfp-*`/`tf-probe-*` returns `[]`.

> Source of truth for the schema and wire shape of `zentaoAPI.Project` and the `st-zentao_project` resource. Any code that contradicts this doc is wrong; if reality changes, update this doc *first*, then the code.

## 0. 2026-05-09 corrections (upstream source review)

The user pointed out two errors in §2/§3/§7/§8 below — the probe surfaced a `productsBox` validator and we **incorrectly generalized it to "products is required on create"**. Re-reading the upstream PHP reveals the truth:

- **`products` is NOT required on create.** Source: [`module/project/model.php#L2026`](https://github.com/easysoft/zentaopms/blob/main/module/project/model.php) — the create method does not enforce `products` as required at the model level. The `productsBox` validator we hit fires from the form-driven `_POST` flow (web UI), not from the V2 model-create path. A V2 POST that omits `products` entirely succeeds and creates a project with no product associations.
- **`multiple` is a first-class form field.** Source: [`module/project/config/form.php#L14-L32`](https://github.com/easysoft/zentaopms/blob/main/module/project/config/form.php) — toggles whether iterations (sprints) are enabled under the project: `"1"` = multi-iteration, `"0"` = single-iteration. Server-defaulted when unset. Not surfaced in the V2 docs; previously assumed Computed-only ("server-derived from products"), but it is actually a **user-settable input**.

**Schema impact** (overrides §7/§8 below):

| Field | Original (wrong) | Corrected |
|---|---|---|
| `products` | Required | **Optional+Computed** |
| `multiple` | Computed-only | **Optional+Computed** (BoolAttribute; resource layer translates `true`↔`"1"` and `false`↔`"0"`. Wire stays string per ZenTao; only the TF type changes for usability.) |

These corrections are reflected in `resource_project.go` and `data_source_project.go` as of 2026-05-09. The §2/§3/§7/§8 tables below are **kept as-is for probe history**; treat this §0 block as the authoritative current schema.

### Lessons recorded into [.claude/skills/zentao-feature-flow/SKILL.md]

1. **Read `module/<entity>/config/form.php` upstream BEFORE probing controller routes** — it is the structured field whitelist + required flags. Probing is for behavior; form.php is for shape.
2. **TF attribute names mirror ZenTao wire names.** `desc` not `description`; `multiple` not `enable_iteration`. Renaming forces a translation layer between spec and code; same vocabulary on both sides keeps the mapping one-to-one.

## 1. Endpoint summary

| Endpoint | Status | Notes |
|---|---|---|
| `POST /api.php/v2/projects` | works | Required body fields beyond docs: see §3 |
| `PUT  /api.php/v2/projects/{id}` | works | More forgiving than POST: see §4 |
| `GET  /api.php/v2/projects/{id}` | **EXISTS** (despite being absent from public docs) | Single-row read; envelope `{status, project: {...}}` |
| `GET  /api.php/v2/projects` | works | List endpoint, returns `{total, projects: [...]}` |
| `DELETE /api.php/v2/projects/{id}` | works | Pure envelope-based status (no HTTP 404) |

The single-GET being undocumented but functional is the same pattern as `GET /api.php/v2/products/{id}`. Treat ZenTao's missing-from-docs endpoints as the norm, not the exception.

## 2. Required vs Optional fields (POST)

The public V2 docs list `name / model / begin / end / parent / desc / PM / acl`. **Two server-enforced required fields are missing from those docs:**

| Field | Type on wire | Required? | Notes |
|---|---|---|---|
| `name` | string | yes | |
| `model` | string | yes | Enum verified: see §5 |
| `begin` | `YYYY-MM-DD` | yes (POST) | Probe 11: omitting only `end` flags only `end` — `begin` validation may differ. Treat as required for safety. |
| `end` | `YYYY-MM-DD` | **yes** | Probe 11: `{"result":"fail","message":{"end":["『计划完成』不能为空。"]}}` |
| **`products`** | `[]int` (≥ 1 element) | **yes** | **Not in docs.** Validator key in error is `productsBox`; the wire field is `products`. Empty array `[]` rejected on both POST *and* PUT. |
| **`workflowGroup`** | int | **yes** | **Not in docs.** Server accepts ANY integer (probe 14: 1, 2, 3, 99 all succeed); no real enum check. |
| `parent` | int | optional | Wire field is `parent`, **not `program`**. Defaults to `0` when omitted (NOT auto-assigned to a default program). |
| `type` | string | optional | Defaults to `"project"`. Sending `"project"` explicitly is accepted and recommended for our resource (defensive). |
| `acl` | string | optional | Default `"private"`. Enum: `open / private / custom`. `custom` accepted without whitelist (probe 7). |
| `desc` | string | optional | Default `""`. |
| `PM` | string | optional | Default to `""`. |

> The validator-name vs wire-name mismatch (`productsBox` in errors, `products` in body) is a ZenTao internal: the controller name comes from the form-field name in the web UI, while the JSON API accepts a flat `products` array.

## 3. Required vs Optional fields (PUT)

PUT is **more permissive** than POST:

| Field | Required on PUT? | Behavior |
|---|---|---|
| `begin` / `end` | **no** (probe 10) | PUT without dates succeeds, prior values retained |
| `products` | mostly no — but `[]` rejected (probe 16) | If body includes `products: []` → 400-style fail. Omit the field entirely to keep prior associations. |
| `workflowGroup` | no | Omit to retain |
| `parent` | no | Omitting retains; sending mutates (probe 9 confirmed `path` updates) |
| `model` | **yes-but-mutable** | PUT with new `model` mutates the row (probe 6: `scrum → waterfall` succeeded). **Server permits methodology change on update.** This contradicts the typical assumption; see §6. |
| `name` | yes (always sent in our wrapper) | |

## 4. Response response shapes

### POST success
```json
{"status": "success", "id": 95, "message": "保存成功"}
```

### POST validation fail (status-keyed)
```json
{"status": "fail", "message": {"productsBox": "最少关联一个产品"}}
{"status": "fail", "message": {"products[0]": "最少关联一个产品"}}
```

### POST validation fail (result-keyed — different envelope!)
```json
{"result": "fail", "message": {"end": ["『计划完成』不能为空。"]}}
```

> Probe 11 returned `result: "fail"` instead of `status: "fail"` — likely depends on whether validation failed at the JSON-decode stage vs. the model-validate stage. Decoder must accept both keys (see §7 implementation note).

### PUT success
```json
{"status": "success", "message": "保存成功",
 "load": "/zentao/zentao/index.php?m=project&f=view&t=json&projectID=97"}
```

### GET success
```json
{"status": "success", "project": { /* see §5 */ }}
```

### GET missing or deleted (HTTP 200, NOT 404)
```json
{"status": "fail", "message": "Project does not exist."}
```

### DELETE success
```json
{"status": "success", "closeModal": true,
 "load": "/zentao/zentao/index.php?m=project&f=browse&t=json"}
```

### DELETE missing or already-deleted (HTTP 200, NOT 404)
```json
{"status": "fail", "message": "Project does not exist."}
```

> **No HTTP 404 anywhere.** Both GET and DELETE return HTTP 200 and discriminate via the envelope. `Project does not exist.` is the canonical not-found reason. The existing `isNotFoundReason` helper in [errors.go](../../../zentaoAPI/errors.go) already matches `"does not exist"` substring — should work unchanged.

## 5. GET `/projects/{id}` full field set

Field set observed on a fresh project (id=95 created, queried, deleted):

```json
{
  "id": "95",                 "project": "0",
  "isTpl": "0",               "charter": "0",
  "model": "scrum",           "type": "project",
  "category": "",             "lifetime": "",
  "budget": "0.00",           "budgetUnit": "CNY",
  "attribute": "",            "percent": "0.00",
  "milestone": "0",           "output": "",
  "auth": "extend",           "storyType": "",
  "parent": "0",              "path": ",95,",
  "grade": "1",
  "name": "tfp-noparent-…",   "code": "",
  "hasProduct": "1",          "workflowGroup": "2",
  "begin": "2026-05-09",      "end": "2026-12-31",
  "firstEnd": "",             "realBegan": "",  "realEnd": "",
  "days": "0",                "status": "wait", "subStatus": "",
  "pri": "1",                 "desc": "",
  "version": "1",             "parentVersion": "1",
  "planDuration": "0",        "realDuration": "0",
  "progress": "0.00",         "estimate": "0.00",
  "left": "0.00",             "consumed": "0.00",
  "teamCount": "1",           "market": "0",
  "openedBy": "admin",        "openedDate": "2026-05-09 02:00:29",
  "openedVersion": "",        "lastEditedBy": "admin",
  "lastEditedDate": "2026-05-09 02:00:29",
  "closedBy": "",             "closedDate": "",  "closedReason": "",
  "canceledBy": "",           "canceledDate": "",
  "suspendedDate": "",
  "PO": "", "PM": "", "QD": "", "RD": "",
  "team": "tfp-noparent-…",
  "acl": "private",           "whitelist": "",
  "tplAcl": "open",           "tplWhiteList": "",
  "order": "140",             "stageBy": "product",
  "displayCards": "0",        "fluidBoard": "0",
  "multiple": "0",            "parallel": "0",
  "enabled": "on",            "linkType": "plan",
  "taskDateLimit": "auto",
  "colWidth": "264", "minColWidth": "200", "maxColWidth": "384",
  "coverExecutionPriv": "1",  "vision": "rnd",
  "frozen": "",               "deleted": "0"
}
```

All numeric/FK columns serialize as **JSON strings** (`"id": "95"`, not `95`). Use `json.Number` decoding (mirrors `productV2Wire` in [product.go](../../../zentaoAPI/product.go)).

### Server-set defaults observed
- `parent: 0` when not sent (server does NOT auto-assign to default program)
- `type: "project"` always (since this resource only POSTs `"project"`)
- `acl: "private"`
- `code: ""`
- `lifetime: ""`
- `vision: "rnd"`
- `multiple: "0"` initially (single-product mode); flipped to `"1"` after PUT with `products: [1, 2]` (probe 15)

## 6. `model` enum verification (probe 5)

All six methodology values create successfully on this Max 8.x instance:

| Value | Probe result |
|---|---|
| `scrum` | success |
| `waterfall` | success |
| `kanban` | success |
| `agileplus` | success |
| `waterfallplus` | success |
| `cmmi` | success |

PUT can change `model` between values (probe 6: `scrum → waterfall` mutated the row). **Server permits this**, but the project's internal state machine is tied to model — switching mid-project leaves the data in an inconsistent state from a UX perspective. We intentionally enforce `RequiresReplace` at the Terraform level so plan/apply produces a destroy+create instead of a silent in-place mutation.

## 7. Reconciliation against grilled decisions

Decisions from [`2026-05-09-zentao-project-resource.md`](../plans/2026-05-09-zentao-project-resource.md) §3 vs probe outcomes:

| § | Decision | Probe verdict | Action |
|---|---|---|---|
| 3.1 | Probe `GET /projects/{id}` existence | **EXISTS** | Use it directly, mirror product `GetProduct` |
| 3.2 | `program` (TF) → `parent` (wire), Optional+Computed | confirmed wire = `parent`; server defaults to `0` not "default program" | TF schema name `program`, wire JSON tag `parent`, default `0` documented |
| 3.3 | `model` Required + enum + RequiresReplace | enum 6 values verified; PUT can mutate but RequiresReplace is policy | Keep RequiresReplace at TF layer (defensive) |
| 3.4 | `type` not exposed, wire `"project"` | server accepts and defaults to `"project"`; explicit send accepted | Lock wire `type:"project"` |
| 3.5 | Required = `name+model+begin+end` | confirmed for POST (begin/end on POST, model always); PUT relaxes begin/end | TF schema marks all four Required (write-time) |
| 3.6 | `code` server-managed | confirmed `""` default, no write input | Computed-only (`json:"-"`) |
| 3.7 | `acl` Optional+Computed enum `open/private/custom` | confirmed; `custom` without whitelist OK | Mirror plan |
| 3.8 | Roles & description Optional+Computed | confirmed | Mirror product role pattern |
| 3.9 | Server-managed read-only set | confirmed; full list in §5 | Computed-only |
| 3.10 | `lifetime` Computed-only | confirmed default `""` | Computed-only string |
| 3.11 | `days/auth/vision` not exposed | server returns them in GET; not exposing is still fine | Skip first version |
| 3.12 | DELETE no pre-flight + idempotent | confirmed: HTTP 200 + envelope-fail on missing | Reuse product/program DELETE template |

### New schema additions surfaced by probe (NOT in original plan)

These two are server-required and were not anticipated:

| Field | TF schema decision |
|---|---|
| **`products`** (`[]int`, ≥ 1) | Required at create; updateable on PUT but cannot be set to empty. Schema: `products` ListAttribute of Int64, **Required**. |
| **`workflow_group`** (int) | Required at create. Server accepts any integer (no enum), so a `Required Int64Attribute` with no validator is correct. Document the Max-typical default value (`2`). |

## 8. Implementation notes flowing to code

1. **`zentaoAPI.Project` struct fields** must include `Products []int json:"products,omitempty"` and `WorkflowGroup int json:"workflowGroup,omitempty"`. POST sends them; PUT can omit if not changed (but if user changes, send the new list).
2. **`isNotFoundReason`** in [errors.go](../../../zentaoAPI/errors.go) already matches `"does not exist"` — works unchanged for `"Project does not exist."`.
3. **POST validation envelope**: decoder must handle both `{"status":"fail",...}` and `{"result":"fail",...}` shapes when classifying errors. Existing `ZentaoResponse.ZentaoFailReason()` may need a `result`-key fallback.
4. **`hasProduct` / `multiple`** are server-derived from `products` association; do not expose in TF schema (Computed-only).
5. **`workflow_group`** value `2` is the Max default (`ALM`) on this instance, but accepting it as Required (no default) is the correct schema choice — the user knows their workflow scheme.
6. **GET response numeric fields** are stringified — reuse the `json.Number` decode pattern from `productV2Wire`.

## 9. Probes considered but not run

- Probing other workflow groups via dedicated endpoint (e.g. `GET /workflows`) — out of scope for this resource; user can query their ZenTao admin UI.
- Probing project-cascade behavior on DELETE (executions, tasks under a project) — user explicitly opted out (grill Q11 b).
- Probing `model` PUT in cmmi/agileplus directions — assumed analogous to scrum→waterfall.
- Probing `acl=custom` + `whitelist=[...]` write path — deferred (grill Q10).
