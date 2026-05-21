# Probe: group privileges (`managePriv`) Controller surface

**Date:** 2026-05-21
**Server:** lek-ws.sige.la:8080 (ZenTao Max 8.x)
**Scope:** read + write path for a permission group's privilege set (`zt_grouppriv`).

## 1. Endpoint summary

A single Controller action serves **both** group scopes:

| Action | Path | HTTP | Notes |
|---|---|---|---|
| Read (catalog + granted) | `/project-managePriv-{projectID}-{groupID}.json` | GET | `projectID = 0` for system groups, `> 0` for project-scoped |
| Write (replace-all) | `/project-managePriv-{projectID}-{groupID}.json` | POST | form-urlencoded |

`projectID` is derived from `GetGroup(groupID).project` — the caller never passes it.

**`group-managePriv-{groupID}.json` is unusable on this version** — every variant
(`-`, `-bydept`, `-byModule`) returns a 117-byte empty shell `{"title":"","type":"N","pager":null}`
with no catalog and no granted list. The `project` module's `managePriv` is the only
JSON-capable surface; it handles system groups via `projectID = 0`
(`project-managePriv-0-2` returned the full 167KB catalog with `selectedCount: 216` for
the built-in 研发 group).

## 2. Read shape (GET)

Envelope: `{"status":"success","data":"<stringified-json>","md5":"..."}`. The inner
`data` (≈137–167KB) carries:

| Key | Shape | Use |
|---|---|---|
| `selectedPrivList` | flat array `["story-view","story-tasks"]` | **the granted set — Read uses this directly** |
| `groupPrivs` | nested map `{"story":{"view":"view","tasks":"tasks"}}` | redundant with selectedPrivList |
| `allPrivList` | flat array of all `module-method` strings (~454) | **assignable catalog — gates which privs the save will persist** |
| `groupID` / `projectID` | stringified numbers | echo of path args |

Each priv identifier is `module-method` (split on the **first** `-`; module/method
names contain no hyphen). Confirms the TF `set(string)` shape.

**No existence validation.** A bogus `groupID` returns HTTP 200, `status:success`,
`selectedPrivList: []`, echoing the bogus id. `managePriv` cannot signal not-found.
**Existence must come from `GetGroup` (group-edit `group:null` → `ErrNotFound`)**, which
also yields the `project` needed to build the path. Read composes: `GetGroup` (existence
+ projectID) → `project-managePriv` GET (selectedPrivList).

## 3. Write shape (POST) — replace-all

Save is triggered only when `$_POST` is non-empty; an empty POST body re-renders the
form (returns the GET catalog shape, **not** a save). Therefore **always send
`noChecked=1`**, even for an empty set.

- **Set privs:** `noChecked=1` + repeated `actions[<module>][]=<method>` per priv.
  e.g. `noChecked=1&actions[story][]=view&actions[story][]=tasks`.
- **Clear all (Delete / empty set):** `noChecked=1` alone → `selectedPrivList` empties.
- Replace-all semantics confirmed: posting `actions[story][]=view` after a two-priv set
  left exactly `["story-view"]`.

Save success envelope: `{"result":"success","message":"保存成功","load":"/zentao/project-group-{projectID}.json"}`
— the `CtrlSimpleResponse` (`result`) shape, not the `status` envelope. Wrong field
names POST successfully but silently no-op (`privs[]=...` returned 保存成功 yet changed
nothing) — the standard ZenTao silent-render trap; the `actions[module][]` shape is
load-bearing.

**Out-of-catalog privs are silently dropped.** A priv absent from this group's
`allPrivList` (e.g. `my-index` on a project-scoped group — the personal `my` module
isn't assignable there) POSTs as `保存成功` but never persists; the re-read
`selectedPrivList` excludes it. On a Required Terraform attribute this surfaces as the
opaque `Provider produced inconsistent result after apply: .privs: planned set element
… does not correlate`. **`SetGroupPrivs` must validate the requested set against
`allPrivList` before the save POST** and reject unknowns with an actionable error
naming them — catalog membership is the exact acceptance criterion (probe: 454-entry
catalog; `story-view`/`task-finish` present and persisted, `my-index` absent and dropped).

## 4. Reconciliation table

| Plan decision | Probe verdict |
|---|---|
| 3.3 branch endpoint on group scope | 🔄 **Simplified** — one endpoint `project-managePriv-{projectID}-{groupID}` for both; `group-managePriv` JSON surface is dead. projectID from GetGroup (0 for system). |
| 3.4 `privs` = set(string) `module-method` | ✅ Confirmed — `selectedPrivList`/`allPrivList` are exactly these strings. |
| 3.2 full-set replace + Delete clears | ✅ Confirmed — `actions[module][]` is replace-all; `noChecked=1` alone clears. |
| Read extracts granted subset | ✅ `selectedPrivList` is the flat granted set; no catalog filtering needed. |
| Not-found detection | ➕ **Net-new** — managePriv never 404s; reuse `GetGroup` → `ErrNotFound`. |
| Out-of-catalog priv handling | ➕ **Net-new** — server silently drops privs absent from `allPrivList`; `SetGroupPrivs` validates against the catalog pre-save to avoid inconsistent-after-apply. |

## 5. Implementation notes

- `GetGroupPrivs(ctx, groupID)`: `GetGroup` (→ project, ErrNotFound passthrough) →
  GET `project-managePriv-{project}-{groupID}` → decode `data.selectedPrivList`.
- `SetGroupPrivs(ctx, groupID, privs)`: format-validate (zero-cost) → `GetGroup`
  (→ project) → managePriv GET for `allPrivList` → reject any requested priv not in
  the catalog → build form (`noChecked=1` + split each priv on first `-` into
  `actions[module][]=method`) → POST. Empty `privs` ⇒ `noChecked=1` only. Three round
  trips (group-edit, catalog GET, save POST).
- Decode: inner `data` is a stringified JSON string (double-decode, like group-edit).
- `selectedPrivList` may be `[]` (jq saw `"array"`); decode into `[]string`.

## 6. Cleanup verification

Probe wrote only to project-scoped group **10000001** (Mobile PIC, project 28); baseline
was empty and was restored (`selectedPrivList: []`). System group 2 was **read-only**.

```bash
direnv exec . bash -c '
SID=$(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" -H "Content-Type: application/json" \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)
curl -sS "$ZENTAO_URL/project-managePriv-28-10000001.json?zentaosid=$SID" \
  | jq -r ".data" | jq ".selectedPrivList"'
```

Expected: `[]`.

**Acc-test discipline:** NEVER write system groups (`project = 0`). Use a known
project-scoped test group; always restore/clear in `t.Cleanup`.
