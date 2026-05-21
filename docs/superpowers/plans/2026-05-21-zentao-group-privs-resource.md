# Plan: `st-zentao_group_privs` resource

**Date:** 2026-05-21
**Branch:** `feat/group-privs-resource`
**Status:** planned (grill complete; Phase 4 probed 2026-05-21 — see `docs/superpowers/specs/probe-group-privs-controller.md`)
**Out of scope (v1):**
- Data source — relationship-style resource; priv sets are rarely referenced independently. Mirrors `resource_program_parent_attachment` (resource-only).
- Member management (`zt_usergroup`) — separate future `st-zentao_group_members`.
- Client-side validation of priv values against the live catalog.

## 1. Goal

Add `st-zentao_group_privs` (resource only) backed by the **Controller** transport.
The resource authoritatively owns a permission group's complete privilege set
(`zt_grouppriv`, a collection of `module-method` grants) via ZenTao's `managePriv`
action. See ADR [0001](../../adr/0001-group-privs-standalone-authoritative-resource.md).

## 2. Endpoints (probe-verified 2026-05-21 — `docs/superpowers/specs/probe-group-privs-controller.md`)

**One endpoint serves both scopes** — `projectID` derived from `GetGroup(groupID).project`
(`0` for system groups, `> 0` for project-scoped):

| Op | Controller path | HTTP | Body |
|---|---|---|---|
| Read | `/project-managePriv-{projectID}-{groupID}.json` | GET | none |
| Write | `/project-managePriv-{projectID}-{groupID}.json` | POST | `noChecked=1` + `actions[<module>][]=<method>` |

`group-managePriv-{groupID}.json` is **dead** on this version (117-byte empty shell) —
do not use it. System groups go through `project-managePriv-0-{groupID}`.

**Probe verdicts:**
- `selectedPrivList` (flat `["module-method",...]`) is the granted set → Read uses it directly.
- Write is replace-all via `actions[<module>][]=<method>`; **always send `noChecked=1`** or an empty POST re-renders instead of saving.
- Delete / empty set = POST `noChecked=1` alone.
- `managePriv` never 404s (bogus id → 200, empty list); not-found comes from `GetGroup` (`group:null` → `ErrNotFound`), which also yields `projectID`.

## 3. Decision log

Decisions reached during the 2026-05-21 grill session. CONTEXT.md gained the **Group Priv** term.

### 3.1 Standalone resource, not inline field
- **Decision:** New `st-zentao_group_privs` resource; never an attribute on `st-zentao_group`.
- **Rationale:** Consistent with the deferral already recorded in
  `docs/superpowers/specs/probe-group-controller.md:363` and
  `docs/superpowers/plans/2026-05-09-zentao-group-resource.md` §3.2. `managePriv` is a
  separate Controller surface from group CRUD. See ADR 0001.

### 3.2 Full-set authoritative ownership
- **Decision:** Create/Update submit the complete declared `privs` set (replace-all);
  Delete clears the set to empty. `privs = []` is a valid "no privileges" assertion.
- **Rationale:** `managePriv` is replace-all on the wire; additive semantics can't be
  reconciled against it. See ADR 0001.

### 3.3 Group scope — both, single endpoint (simplified by probe)
- **Decision:** Support both system (`project = 0`) and project-scoped (`project > 0`)
  groups. Resource takes only `group` (id); `projectID` is derived via
  `GetGroup(groupID).project` and fed into the **one** path
  `project-managePriv-{projectID}-{groupID}`.
- **Rationale (post-probe):** No endpoint branching needed — `group-managePriv` JSON
  surface is dead on this version; `project-managePriv` handles system groups via
  `projectID=0`. `GetGroup` does double duty (existence check + projectID source).

### 3.4 `privs` shape
- **Decision:** `Required`, `set(string)`, each item `module-method` (split on first `-`;
  module/method names contain no hyphen). Format validator `^\w+-\w+$`. Empty set allowed.
- **Rationale:** User confirmed `module-method` matches ZenTao's official priv identifier.
  Set = order-independent, deduplicated. No enum validation against the dynamic ~137KB catalog.

### 3.5 `group` field
- **Decision:** `Required`, `Int64`, `RequiresReplace`, positive-integer validator.
  Re-pointing to a different group is destroy+create.
- **Rationale:** The resource's identity is the group; mirrors the owning-side field of
  `resource_program_parent_attachment` (`RequiresReplace`).

### 3.6 `id`
- **Decision:** Computed string = group id, `UseStateForUnknown`.
- **Rationale:** 1:1 with the group; stable from create-time (safe per SKILL §6b-bis).

### 3.7 Read / out-of-band drift
- **Decision:** Read GETs the catalog, extracts the granted subset into `privs`. If the
  group is gone (`ErrNotFound`), `RemoveResource`.
- **Rationale:** UI-side priv edits surface as a normal plan diff against the declared set.

### 3.8 No data source
- **Decision:** Resource only.
- **Rationale:** Relationship-style; priv sets rarely referenced independently. Mirrors
  `resource_program_parent_attachment`.

### 3.9 Probe mode
- **Decision:** curl against the live server (`.envrc` `ZENTAO_URL`/`ACCOUNT`/`PASSWORD`).
- **Rationale:** Fastest verification of `managePriv` GET/POST wire shapes. SKILL hard rule
  "Probe before assuming."

## 4. Implementation sketch (Phase 6)

- **API client:** `zentaoAPI/group_priv.go` (+ `_test.go`), Controller transport. Primitives:
  - `GetGroupPrivs(ctx, groupID) ([]string, error)` — internal `GetGroup` for scope, GET, extract granted subset.
  - `SetGroupPrivs(ctx, groupID, privs []string) error` — internal `GetGroup` for scope, POST full set.
- **TF layer:** `zentao/resource_group_privs.go` (+ `_test.go`), register in `provider.go` Resources slice.
- **Docs:** `examples/resources/st-zentao_group_privs/resource.tf`, README section, `make generate-docs`.

## 5. PR shape (Phase 7)

Single PR on `feat/group-privs-resource`, 4 commits:
1. `docs: probe-driven design contract for st-zentao_group_privs` — this plan + ADR + spec.
2. `feat(zentaoAPI): GroupPrivs Get/Set via managePriv Controller action` — client + tests.
3. `feat(zentao): st-zentao_group_privs resource` — TF layer + tests + provider.go reg.
4. `docs: group_privs example + README section + regenerated tfplugindocs output`.
