# Probe: ZenTao Program Controller surface

**Date:** 2026-05-09
**Server:** ZenTao Max 8.x at `${ZENTAO_URL}` (lek-ws.sige.la:8080)
**Tool:** raw `curl` via `direnv exec .` + upstream `easysoft/zentaopms` source review
**Cleanup:** all probe-created program ids deleted; `program-browse` filter on `tfp-*` returns `[]`.

> Source of truth for the schema and wire shape of `zentaoAPI.Program` and the `st-zentao_program` resource. We migrated this resource from V2 to the Controller transport on 2026-05-09 because V2's `/api.php/v2/programs/{id}` only echoes ~24 fields, while the controller surfaces the full `zt_project` row (~70 fields).

## 1. Endpoint summary

| Endpoint | Status | Notes |
|---|---|---|
| `GET program-edit-{id}.json` | works (read primitive) | Wraps `data.program` JSON; `program: false` on missing id |
| `POST program-create.json` | works | Returns `{result, message, id, load}` — id inline, no separate lookup needed |
| `POST program-edit-{id}.json` | works | Form-urlencoded body; returns `{result, message, locate}` |
| `GET program-delete-{id}-yes.json` | works | Positional `-yes` suffix. Idempotent on missing rows |
| `GET program-view-{id}.json` | **302 → `program-product-{id}.json`** | Don't use; same trap as `user-view → user-todocalendar` |
| `GET program-browse.json` | works | Filters out soft-deleted rows |

## 2. form.php field set (writeable inputs)

Source: [`module/program/config/form.php#L1-L24`](https://github.com/easysoft/zentaopms/blob/main/module/program/config/form.php).

### create form

| Field | Type | Required (form.php) | Required (live) | Default | Notes |
|---|---|---|---|---|---|
| `parent` | int | no | no | 0 | parent program id |
| `name` | string | **yes** | yes | — | trimmed |
| `PM` | string | no | no | — | username |
| `budget` | float | no | no | — | float-as-string per ZenTao convention |
| `budgetUnit` | string | no | no | `CNY` | |
| `begin` | date | **yes** | yes | — | |
| `end` | date | no | **yes** | — | form.php declares Optional but live install rejects empty: `{"end":["『计划完成』不能为空。"]}` (probe 2026-05-09) |
| `desc` | string | no | no | — | editor control |
| `status` | string | no | no | `wait` | NOT exposed on the resource — state changes go through `close/activate/start` controllers we don't surface |
| `acl` | string | no | no | — | open / private / custom |
| `whitelist` | array (joined) | no | no | — | `filter=join` flattens to comma-joined string on the wire; only meaningful when `acl=custom` |

### edit form

Same fields as create except `status` is dropped — closing/activating uses dedicated controller actions (out of scope for this resource).

## 3. Controller GET shape (`program-edit-{id}.json`)

The probe target id=1 returned the full row (verbatim, trimmed):

```json
{
  "id": 1, "project": 0, "isTpl": 0, "charter": 0,
  "model": "", "type": "program", "category": "", "lifetime": "",
  "budget": "0.00", "budgetUnit": "CNY", "attribute": "",
  "percent": "0.00", "milestone": 0, "output": "", "auth": "",
  "storyType": "story", "parent": 0, "path": ",1,", "grade": 1,
  "name": "LB", "code": "", "hasProduct": 1, "workflowGroup": 0,
  "begin": "2025-01-01", "end": "2059-12-31", "firstEnd": "",
  "realBegan": "", "realEnd": "", "days": 0,
  "status": "doing", "subStatus": "", "pri": 1, "desc": "",
  "version": 1, "parentVersion": 1, "planDuration": 0, "realDuration": 0,
  "progress": "0.00", "estimate": "0.00", "left": "0.00", "consumed": "0.00",
  "teamCount": 0, "market": 0,
  "openedBy": "admin", "openedDate": "2026-05-09 17:21:51",
  "openedVersion": "", "lastEditedBy": "", "lastEditedDate": "",
  "closedBy": "", "closedDate": "", "closedReason": "",
  "canceledBy": "", "canceledDate": "", "suspendedDate": "",
  "PO": "", "PM": "", "QD": "", "RD": "", "team": "",
  "acl": "private", "whitelist": "",
  "tplAcl": "open", "tplWhiteList": "",
  "order": 5, "stageBy": "product",
  "displayCards": 0, "fluidBoard": 0, "multiple": 1, "parallel": 0,
  "enabled": "on", "linkType": "plan", "taskDateLimit": "auto",
  "colWidth": 264, "minColWidth": 200, "maxColWidth": 384,
  "coverExecutionPriv": 1, "vision": "rnd", "frozen": "", "deleted": 0
}
```

Numeric columns mix native ints (`id`, `multiple`, `deleted`, …) and stringified decimals (`budget`, `progress`, `estimate`, …). Each numeric in `programCtrlWire` rides through `json.Number` for forgiving unmarshalling.

The wrapper does NOT surface every field. The omitted ones are: `project`, `isTpl`, `charter`, `milestone`, `output`, `auth`, `planDuration`, `realDuration`, `market`, `openedVersion`, `tplAcl`, `tplWhiteList`, `stageBy`, `displayCards`, `fluidBoard`, `enabled`, `frozen`, `linkType`, `taskDateLimit`, `colWidth`, `minColWidth`, `maxColWidth`, `coverExecutionPriv`, `pri`, `version`, `parentVersion`, `subStatus`, `order`, `deleted`. Reasoning: most are presentation/template internals (`stageBy`, `displayCards`, …) or admin metadata Terraform shouldn't synchronise (`pri`, `version`). `deleted` is consumed internally — the wrapper turns `deleted=1` into `ErrNotFound`.

## 4. Response response shapes

### GET success

```json
{
  "status": "success",
  "data": "{\"title\":\"...\",\"charters\":[],\"charter\":0,\"pmUsers\":{},\"program\":{...full row...},\"parents\":{...}}"
}
```

`data` is a JSON-encoded string; `DecodeData` unwraps it. Only the inner `program` field is consumed.

### GET missing id (HTTP 200, NOT 404)

```json
{
  "status": "success",
  "data": "{\"title\":\"...\",\"program\":false,\"parents\":{...}}"
}
```

`program: false` is the empty-marker — wrapper surfaces `ErrNotFound`.

### POST create — success

```json
{"result": "success", "message": "保存成功", "id": 70, "load": "/zentao/program-browse.json"}
```

`id` arrives inline. Distinct from `user-create`, which doesn't echo an id.

### POST create — validation fail

```json
{"result": "fail", "message": {"name": ["『项目集名称』不能为空。"], "end": ["『计划完成』不能为空。"]}}
```

Per-field map. `classifyCtrlSimple` composes `name: ...; end: ...` into the `*APIError.Reason`.

### POST edit — success

```json
{"result": "success", "message": "保存成功", "locate": "/zentao/program-browse.json"}
```

Note `locate` (not `load`). Wrapper doesn't read it.

### DELETE — success (real row OR missing id)

```json
{"result": "success", "load": true}
```

Both flows return the same envelope, so the wrapper is naturally idempotent on missing.

### DELETE without `-yes` suffix

```json
{
  "result": "fail",
  "callback": "zui.Modal.confirm({...message: '您确定要删除\"…\"项目集吗？'}).then(...$.ajaxSubmit({url: '/zentao/program-delete-70-yes.json'}))"
}
```

The controller hands back JS for the web UI's confirmation modal. Sending `?confirm=yes` does NOT bypass it — only the positional `-yes` suffix does. This is different from `user-delete`, which uses `?confirm=yes`.

## 5. Soft-delete behaviour

`program-delete-{id}-yes.json` does **not** physically remove the row. Probe sequence (id=71):

```
create → deleted=0
delete → {result:success, load:true}
edit-GET → row still returned with deleted=1
browse → row filtered out
```

`GetProgram` therefore checks `deleted == "1"` and returns `ErrNotFound` so:
- Terraform `Read` clears state when a user manually deletes via ZenTao's UI.
- `program-browse`'s filter and our wrapper's filter agree on what's "alive".

## 6. Reconciliation with the previous V2 design

| Aspect | V2 era | Controller era |
|---|---|---|
| Read endpoint | `GET /api.php/v2/programs/{id}` (~24 fields) | `GET program-edit-{id}.json` (~70 fields) |
| Create endpoint | `POST /api.php/v2/programs` (JSON body) | `POST program-create.json` (form-urlencoded) |
| Update endpoint | `PUT /api.php/v2/programs/{id}` (JSON body) | `POST program-edit-{id}.json` (form-urlencoded) |
| Delete endpoint | `DELETE /api.php/v2/programs/{id}` | `GET program-delete-{id}-yes.json` |
| Auth | `Token: <sid>` header | `?zentaosid=<sid>` query (no header) |
| Idempotent missing-row delete | yes (200 + envelope-fail) | yes (200 + same success envelope) |
| Required fields | `name`, `begin`, `end`, `pm`, `desc` | `name`, `begin`, `end` (per form.php; `end` per live enforcement) |
| Optional writeable fields | only `pm`, `desc` | `parent`, `pm`, `desc`, `acl`, `budget`, `budget_unit`, `whitelist` |

## 7. Implementation notes flowing to code

1. **`program-view-{id}` is a 302 trap.** Use edit-GET as the read primitive (mirrors `user-view` pattern).
2. **Numeric/string mix.** Every numeric column on `programCtrlWire` rides through `json.Number`; the toProgram method runs `jsonNumberToInt` for ints and `Multiple/Parallel/Deleted.String()` for the bool-via-string fields.
3. **`Multiple` and `Parallel` are bool-on-the-resource.** Wire is "0"/"1" (sometimes 0/1); resource translates with `programWireBoolToTF` mirroring the `project.multiple` translation.
4. **Soft delete** must fold into `ErrNotFound` so Terraform Read clears state.
5. **Delete URL is `program-delete-{id}-yes.json`** (positional), NOT `?confirm=yes` query. This is different from `user-delete`.
6. **`whitelist` is a comma-joined string on the wire** (form.php `filter=join`). Resource exposes the joined string as-is. Splitting/joining for an array attribute would be nicer ergonomically; deferred until a user actually needs it because slicing semantics around `acl != custom` are non-trivial.
7. **Required vs Optional split** comes from form.php first (per the project-wide "form.php first" rule), then verified live. `end` was the only divergence — declared Optional in form.php but rejected as empty by the live install, so the schema marks it Required.

## 8. Addendum 2026-05-09 — `program-edit` POST is **not** PATCH-semantic

Probed while extracting `parent` into a dedicated attachment resource. Findings change how `UpdateProgram` and any future field-level wrapper must build their forms.

### F1 — Omitting `parent` resets the column to 0

```
POST program-edit-{id} with parent=N → zt_project.parent = N, path/grade recomputed
POST program-edit-{id} omitting parent entirely → zt_project.parent = 0, path/grade recomputed
POST program-edit-{id} with parent=0 explicitly → identical to omission (clears to 0)
```

There is **no way** to "POST edit while leaving parent unchanged" — the form handler treats absent `parent` as `0`.

### F2 — Same omission behaviour for every other form.php writeable field

Created a probe row with `desc=initial-desc-text, PM=admin, acl=open, budget=10000`, then POST-edited with **only** `name/begin/end`. Result:

| Field | Before | After omission | Behaviour |
|---|---|---|---|
| `desc` | `initial-desc-text` | `""` | reset |
| `PM` | `admin` | `""` | reset |
| `acl` | `open` | `""` | reset |
| `budget` | `10000.00` | `0.00` | reset |
| `budgetUnit` | `USD` | `USD` | preserved (form.php default fills in) |
| `whitelist` | `""` | `""` | (already empty) |
| `parent` | `0` | `0` | (already empty) |
| `vision` | `rnd` | `rnd` | preserved (not in form.php) |
| `multiple` | `1` | `1` | preserved (not in form.php) |
| `storyType` | `story` | `story` | preserved (not in form.php) |

So the rule is: **every form.php writeable field that is omitted gets reset to its form.php default** (which is empty/0 for most fields, `USD` for `budgetUnit`). Non-form.php columns are left alone.

### F3 — ZenTao does not reject self-attach or multi-level cycles

```
POST program-edit-A parent=A   → success; A.path = ",A,A,..." (corrupted)
POST program-edit-A parent=B (where B.parent already = A) → success; A.path includes A twice
```

ZenTao silently accepts the write, recomputes `path` to include the cycle, and returns `result: success`. It is **not safe** to rely on the server to reject illegal parent topologies.

### F4 — Implications for the wrappers

1. **`UpdateProgram` is not PATCH** — sending only the fields the user changed silently clears all other form.php fields. Any safe Update must:
   - Fetch the current row (`GetProgram(id)`)
   - Merge user-supplied non-zero fields onto that baseline (M-Z merge: zero/empty input = preserve baseline)
   - Submit a form containing **every** form.php field, even when the value is empty/0.
2. **`programToForm` must always-set every form.php field**, including `parent=0`, `desc=""`, `acl=""`, `budget=0`, `whitelist=""`. The `if X != "" { Set }` pattern is unsafe.
3. **A dedicated `SetProgramParent(child, parent)` wrapper** must use the same baseline-merge pattern (only `Parent` overridden); plus client-side guards because the server has none:
   - Reject `parent == child` (self-attach).
   - Reject `parent.path` containing `,child,` (would form a cycle).
4. **Detach semantics** are simply `parent = 0` on the full-form submit.

Cleanup: probe rows A=100, B=101, C=102 deleted at end of session.
