# Probe: ZenTao Group (Controller transport)

**Probed against:** ZenTao Max 8.x at `lek-ws.sige.la:8080`
**Date:** 2026-05-09
**Driving plan:** `docs/superpowers/plans/2026-05-09-zentao-group-resource.md`
**Probe operator:** Claude Code, via direnv-wrapped curl using `.envrc` credentials.

---

## 0. Critical safety notes (read first)

1. **`GET /group-delete-<id>.json` is destructive on this server. NO `confirm=yes` is required.**
   The probe accidentally deleted the system `admin` group (id=1) by issuing this URL during exploration. Recovered by `POST /group-create.json` with the original fields, but **the new row received id=10000001**, orphaning any `zt_usergroup` rows that referenced id=1. The `admin` user retained super-user access (independent of group membership).
   - **Implication for code:** `DeleteGroup` will issue a GET; this matches the established `doController(ctx, "group", "delete", ...)` shape (no body → GET) and is correct. But the wrapper documentation MUST flag this as a write, and acceptance tests MUST NOT issue speculative `group-delete-*` requests outside an explicit teardown step.

2. **`POST /group-edit-<id>.json` on a non-existent id silently returns success.**
   No row is created, no error is surfaced. The envelope is the same `{load:true, closeModal:true, result:"success", message:"保存成功"}` as a successful update.
   - **Implication for code:** `UpdateGroup` MUST re-read after the POST and treat `group:null` as `ErrNotFound`. The success envelope alone is not proof of mutation.

3. **Project-scoped groups (project>0) do NOT appear in `/group-browse.json`; the system-wide listing only returns project=0 rows.**
   Two list endpoints are needed for the post-Create id lookup, routed by the new row's `project` value:
   - `project = 0` → `/group-browse.json`.
   - `project > 0` → `/project-group-<projectID>.json`.
   - **Implication for code:** `findGroupIDByName` switches list endpoints on the `projectID` argument; a single endpoint cannot serve both flavours.

---

## 1. Module location

The Controller module is **`group`** (system module). The user's initial pointer to `module/project/control.php` referenced the project-side _listing view_ (`project-group-<projectID>.json` action `group()` in project module), but the actual CRUD plumbing lives in `module/group/control.php` with a `project` field on the row that distinguishes system groups (project=0) from project-scoped groups (project>0). Both flavours share the exact same CRUD surface — the `project` value is just data on the row, not a different endpoint.

| Action | URL | HTTP | Body |
|---|---|---|---|
| List, system flavour | `/group-browse.json` | GET | none |
| List, project flavour | `/project-group-<projectID>.json` | GET | none |
| Create | `/group-create.json` | POST | form-urlencoded |
| Read (single) | `/group-edit-<id>.json` | GET | none |
| Update | `/group-edit-<id>.json` | POST | form-urlencoded |
| Delete | `/group-delete-<id>.json` | GET | none |

All routes authenticate via `?zentaosid=<sid>` query (Controller standard).

> Variants tried but absent on this version: `group-view`, `project-groupCreate`, `project-groupEdit`, `project-groupView`, `project-groupCopy`, `project-groupBrowse`, `groups-*`, `trash-*`. All return `module group has no Xxx method` or `module project has no Xxx method`.

---

## 2. Wire shapes

### 2.1 Create — `POST /group-create.json` (form-urlencoded)

**Form fields observed:**

| Field | Required | Notes |
|---|---|---|
| `name` | ✅ | display name |
| `project` | ✅ | 0 = system; >0 = project-scoped |
| `vision` | ✅ (validator) | one of `rnd`, `lite`, `rnd,lite`. Default to `rnd` if caller omits. |
| `role` | ❌ | free-text, bound to system role registry; `""` is accepted |
| `desc` | ❌ | text |
| `developer` | ❌ | 0/1; effective on Create, but ignored on Update (see 2.4) |

**Success response:**
```json
{"load":true,"result":"success","message":"保存成功"}
```

> **No ID echoed.** Caller MUST list to discover the new row's id.

---

### 2.2 List by project — `GET /project-group-<projectID>.json`

```json
{
  "status": "success",
  "data": "<stringified-json>",
  "md5": "..."
}
```

`data` is a **JSON string** that decodes to:

```json
{
  "title": "浏览分组",
  "globalDisableProgram": false,
  "groups": [
    {
      "id": 10000002,
      "project": 28,
      "name": "tf-probe-pg-1778265940",
      "role": "",
      "desc": "tf-probe project group desc",
      "acl": "",
      "developer": 0,
      "vision": "rnd",
      "users": ""
    }
  ],
  "project": { /* full project row, irrelevant here */ },
  "projectID": "28",
  "programID": "0",
  "groupUsers": [],
  "pager": null
}
```

**Quirks:**
- `acl` here is `""` (string). In `group-browse.json` the same field is `[]` (array). Use `json.RawMessage` if surfaced; v1 does not surface it.
- `users` is `""` (string) when no users; presumably comma-separated when populated. v1 does not surface it.
- `id` is a native integer here (not stringified — but probe did not exhaustively confirm across versions, so the wire decoder uses `json.Number` defensively, matching `userCtrlWire`).

---

### 2.3 Read single — `GET /group-edit-<id>.json`

```json
{
  "status": "success",
  "data": "<stringified-json>",
  "md5": "..."
}
```

`data` decodes to:

```json
{
  "title": "组织视图-编辑分组",
  "group": {
    "id": 10000002,
    "project": 28,
    "name": "tf-probe-pg-1778265940",
    "role": "",
    "desc": "tf-probe project group desc",
    "acl": [],
    "developer": 0,
    "vision": "rnd"
  },
  "pager": null
}
```

**Differences from list shape:**
- No `users` field here.
- `acl` here is `[]` (empty array) on a fresh group.

**Not-found shape:**

```json
{
  "status": "success",
  "data": "<stringified-{title:..., group:null, pager:null}>"
}
```

> HTTP 200, envelope `status:success`, **inner `group:null`**. The wrapper must treat this as `ErrNotFound`. Mirrors the `userEditInner` `User: null` handling.

---

### 2.4 Update — `POST /group-edit-<id>.json` (form-urlencoded)

**Success response:**
```json
{"load":true,"closeModal":true,"result":"success","message":"保存成功"}
```

**Mutability observed:**

| Field | Mutable on Update? |
|---|---|
| `name` | ✅ |
| `desc` | ✅ |
| `role` | ✅ (probed `""` ↔ valid string both directions) |
| `vision` | ✅ |
| `developer` | ❌ silently ignored (sent `1`, readback stayed `0`) |
| `project` | not probed; assumed immutable (RequiresReplace in TF schema) |

**Silent no-op on missing id:** `POST /group-edit-99999.json` returns the same success envelope with NO row created. Wrapper MUST re-read and surface `ErrNotFound` if the row is gone.

---

### 2.5 Delete — `GET /group-delete-<id>.json`

**Success response:**
```json
{"result":"success","message":"","load":"\/zentao\/group-browse.json"}
```

**Idempotency:**
- Re-delete on already-deleted id → same success envelope.
- Delete on never-existed id → same success envelope.

> The wrapper treats every successful envelope as success (idempotent on missing). Mirrors `DeleteUser`'s shape-C tolerance.

---

## 3. Reconciliation against plan

| Plan §  | Decision in plan | Verdict | Action |
|---|---|---|---|
| 3.1 | Controller transport, not V2 | ✅ Confirmed | — |
| 3.1 (impl-detail) | Module `project-group*` actions | 🔄 **Modified** | Module is `group`, not `project`. URL prefixes are `group-create`, `group-edit-<id>`, `group-delete-<id>` plus `project-group-<projectID>` for listing. |
| 3.2 | v1 scope: name/project/desc/role only | ✅ Confirmed | — |
| 3.3 | Read primitive = view first, fall back to edit | 🔄 **Modified** | `group-view` does NOT exist. Read primitive is `group-edit-<id>.json` GET unconditionally — no fallback chain. |
| 3.4 | `project` Required + RequiresReplace | ✅ Confirmed | — |
| 3.5 | `name` Required, mutable | ✅ Confirmed | — |
| 3.6 | `desc` Optional + Computed, mutable | ✅ Confirmed | Server returns `""` when unset; no surprising normalisation observed. |
| 3.7 | `role` Optional, free text, probe enum | ➕ **Surfaced** | Server accepts `""`; full enum not bounded by probe (would need to enumerate every valid system role). v1 keeps `role` as `Optional` String with no validator — caller's responsibility to use a valid role. |
| 3.8 | DELETE idempotent on missing | ✅ Confirmed | Plus: **DELETE is GET, no confirm required, destructive immediately.** Wrapper docstring + acceptance test cleanup discipline must reflect this. |
| 3.9 | Not-found shape | 🔄 **Modified** | NO HTTP 404; envelope `status:success` with **inner `group:null`** is the sole not-found marker. Read wrapper unwraps `data` then checks the `group` key. |
| 3.10 | ID lookup post-Create | 🔄 **Modified** | Confirmed Create does NOT echo ID. Lookup mechanism: list via `project-group-<projectID>.json` and filter by `name` (uniqueness assumed within a project; the only safe key the probe surfaced). |
| 3.11 | Schema table | ✅ Confirmed | — |
| 3.12 | Branch + 4 commits | ✅ Confirmed | — |
| 3.13 | Probe via direnv + curl | ✅ Confirmed | — |

---

## 4. Implementation contract for `zentaoAPI/group.go`

### 4.1 Public type

```go
type Group struct {
    ID      int    `json:"-"` // server-assigned; populated post-Create via list-and-filter
    Project int    `json:"project"`
    Name    string `json:"name"`
    Role    string `json:"role,omitempty"`
    Desc    string `json:"desc,omitempty"`
    // Server-managed; not surfaced to TF in v1, kept here so we can decode without losing data.
    Vision    string `json:"-"` // server defaults to "rnd"; we always send "rnd"
    Developer int    `json:"-"` // ignored by server on update
}
```

### 4.2 Wire type (mirrors `userCtrlWire`)

```go
type groupCtrlWire struct {
    ID        json.Number     `json:"id"`
    Project   json.Number     `json:"project"`
    Name      string          `json:"name"`
    Role      string          `json:"role"`
    Desc      string          `json:"desc"`
    ACL       json.RawMessage `json:"acl"`     // tolerates "" and []
    Developer json.Number     `json:"developer"`
    Vision    string          `json:"vision"`
}
```

### 4.3 Inner-form types

```go
// group-edit-<id>.json GET inner shape
type groupEditInner struct {
    Group json.RawMessage `json:"group"` // either {...} or null
}

// project-group-<projectID>.json inner shape
type groupListInner struct {
    Groups []groupCtrlWire `json:"groups"`
}
```

### 4.4 CRUD signatures

```go
func (c *Client) CreateGroup(ctx context.Context, g *Group) (*Group, error)
func (c *Client) GetGroup(ctx context.Context, id int) (*Group, error)
func (c *Client) UpdateGroup(ctx context.Context, g *Group) (*Group, error)
func (c *Client) DeleteGroup(ctx context.Context, id int) error

// Helper used by Create to discover the new row's id (no API echo).
func (c *Client) findGroupIDByName(ctx context.Context, projectID int, name string) (int, error)
```

### 4.5 Path helpers

```go
const groupCreatePath = "group-create.json"

func groupEditPath(id int) string { return controllerPath("group", "edit", []string{strconv.Itoa(id)}) }
func groupDeletePath(id int) string { return controllerPath("group", "delete", []string{strconv.Itoa(id)}) }
func groupListByProjectPath(projectID int) string { return controllerPath("project", "group", []string{strconv.Itoa(projectID)}) }
```

### 4.6 Required form fields

```go
func groupToForm(g *Group) url.Values {
    f := url.Values{}
    f.Set("name", g.Name)
    f.Set("project", strconv.Itoa(g.Project))
    f.Set("role", g.Role)
    f.Set("desc", g.Desc)
    if g.Vision != "" {
        f.Set("vision", g.Vision)
    } else {
        f.Set("vision", "rnd") // server validator requires this
    }
    if g.Developer != 0 {
        f.Set("developer", strconv.Itoa(g.Developer))
    }
    return f
}
```

### 4.7 Error mapping

| Wire condition | Wrapper return |
|---|---|
| GET `group-edit` envelope status≠success | `classifyCtrlError` |
| GET `group-edit` data inner `group:null` | `ErrNotFound` |
| GET `group-edit` HTTP 404 | `ErrNotFound` |
| POST `group-create` `result≠success` | `classifyCtrlSimple` |
| POST `group-edit` `result≠success` | `classifyCtrlSimple` |
| POST `group-edit` `result=success` AND post-read `group:null` | `ErrNotFound` (silent no-op detection) |
| GET `group-delete` `result≠success` | `classifyCtrlSimple` (envelope-fail) |
| GET `group-delete` `result=success` | `nil` (idempotent) |
| GET `group-delete` HTTP 404 | `nil` (idempotent) |

### 4.8 Test matrix

Required unit tests (mirror `user_test.go` discipline):

- `TestCreateGroup_BodyShape` — verifies form-urlencoded body has name/project/role/desc/vision; vision defaults to `rnd`.
- `TestCreateGroup_LookupAfterCreate` — Create → list `project-group-<projectID>.json` → returns id by name match.
- `TestCreateGroup_FailEnvelope` — `result:fail` is surfaced as `*APIError`.
- `TestGetGroup_FullFieldSet` — every field decoded from a probe-shaped response (ACL string AND array forms).
- `TestGetGroup_NotFound_NullInner` — `data: "{...group:null...}"` → `ErrNotFound`.
- `TestGetGroup_NotFound_HTTP404` — defensive (not observed; consistent with package convention).
- `TestUpdateGroup_PutPathAndRefetch` — POST then GetGroup composition; assertion asserts wire path of both calls.
- `TestUpdateGroup_SilentNoOpDetection` — POST returns success but post-read `group:null` → `ErrNotFound`.
- `TestUpdateGroup_FailEnvelope` — `result:fail` is surfaced.
- `TestDeleteGroup_Success` — `result:success` → `nil`.
- `TestDeleteGroup_HTTP404IsIdempotent` — defensive parity with user.
- `TestDeleteGroup_OtherFailureIsSurfaced` — `result:fail` → `*APIError`.

### 4.9 Acceptance test (TF layer) discipline

- All probe + acc-test rows MUST be created with name prefix `tf-acc-pg-` or `tf-imp-pg-` to be findable in the global cleanup script.
- DELETE is destructive without confirm — acceptance test cleanup MUST always run, even on test failure (TestMain-style cleanup hook or `t.Cleanup`).
- **NEVER touch system groups (project=0)** in any acc test. Always pin `project` to a known-test project (e.g. project=28 on the lek-ws.sige.la instance).

---

## 5. Cleanup verification

```bash
direnv exec . bash -c '
SID=$(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" -H "Content-Type: application/json" \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)
curl -sS "$ZENTAO_URL/project-group-28.json?zentaosid=$SID" | jq -r ".data" | jq "[.groups[] | select(.name | startswith(\"tf-probe-pg\") or startswith(\"tf-acc-pg\") or startswith(\"tf-imp-pg\"))]"
'
```

Expected: `[]`. Probe execution on 2026-05-09 left zero residue.

---

## 6. Open questions for follow-up resources (out of scope here)

- **Privs (`groupManagePriv`)**: probe `project-managePriv-<projectID>.json` returned a 137KB enumeration of all system privs grouped by module. The shape is documented but the write path (toggle priv on/off for a group) was NOT probed. A follow-up `st-zentao_group_privs` resource would need its own spec.
- **Members (`group-managePriv` users)**: not probed. Membership likely lives in `zt_usergroup`; probable Controller actions: `group-manageMember-<id>.json` (POST). Follow-up resource.
- **`actions` field on system groups**: present in `group-browse.json`, absent in `group-edit-<id>.json` and `project-group-<projectID>.json`. Not surfaced in v1; future expansion would need a new probe.
