# Plan: `st-zentao_group` resource + data source

**Date:** 2026-05-09 (rev. b — unified system + project flavours)
**Branch:** `feat/project-resource` (continued; appended on top of the four `st-zentao_project` commits)
**Status:** implemented; refactored 2026-05-09b to unify system and project group flavours after Phase 6 surfaced that the same row shape and CRUD surface serve both.
**Out of scope (v1):**
- Permission list management (`privs`) — to be added as a separate `st-zentao_group_privs` resource in a follow-up.
- Member management (`members`) — same: future `st-zentao_group_members`.
- The companion actions `groupCopy`, `groupManageView`, `groupManagePriv` — only the bare CRUD of the group entity is in scope here.
- `force_destroy` flag, role-template seeding.

## 1. Goal

Add `st-zentao_group` (resource + data source) to the provider, backed by ZenTao's **Controller** transport (PATH_INFO `.json` routing). API V1 and V2 do **not** expose group CRUD on Max 8.x, so we mirror the `user.go` Controller-based wrapper precedent rather than `project.go`'s V2 RESTful pattern.

The same `zt_group` row shape and Controller plumbing serve two flavours, distinguished only by the `project` column:

- `project = 0` → **system group** (org-wide RBAC; e.g. the built-in admin group).
- `project > 0` → **project-scoped permission group** (项目权限组).

Both flavours go through the same `module/group/control.php` actions; the `module/project/control.php` action of the same name is just a per-project listing view, not its own CRUD module. The unified `st-zentao_group` resource selects flavour via the `project` attribute (default 0).

## 2. Endpoints (probe-verified 2026-05-09 — see `docs/superpowers/specs/probe-group-controller.md`)

| Action | Controller path | HTTP | Body |
|---|---|---|---|
| List by project | `/project-group-<projectID>.json` | GET | none |
| Create | `/group-create.json` | POST | form-urlencoded (`project` is a body field, not a path arg) |
| Read (single) | `/group-edit-<id>.json` | GET | none |
| Update | `/group-edit-<id>.json` | POST | form-urlencoded |
| Delete | `/group-delete-<id>.json` | GET | none — **destructive without `confirm=yes`** ⚠️ |

**Key reconciliation finding:** The Controller module is `group` (system module), not `project`. The user-supplied pointer to `module/project/control.php` was at the project-side _listing_ view (`project-group-<projectID>` action `group()` in the project module), but CRUD plumbing lives in `module/group/control.php` with a `project` field on the row distinguishing system groups (project=0) from project-scoped groups (project>0).

## 3. Decision log

Decisions reached during the 2026-05-09 grill-me session.

### 3.1 Transport choice
- **Decision:** Controller transport (`doController` / `doControllerForm`) on the **`group` module** — not V2, not the `project` module.
- **Rationale:** User confirmed V1/V2 do not expose this entity on Max 8.x. Probe (2026-05-09) confirmed CRUD lives in `module/group/control.php`; the `project/group()` action exposed by the project module is a project-scoped listing view only. Mirrors the rationale that put `user.go` on Controller.

### 3.2 v1 scope slicing
- **Decision:** Minimal slice — manage only the group entity itself (name/project/desc/role). Privs and members are deferred to follow-up resources.
- **Rationale:** ZenTao's `groupManagePriv` is a separate Controller step on this version and the wire shape is unverified. Shipping CRUD on the group entity first establishes the test baseline; layering privs/members in a second PR keeps each PR's risk surface small. Mirrors the project's "ship product first, then program" evolution path.

### 3.3 Read path strategy
- **Decision (post-probe):** Read primitive is `group-edit-<id>.json` GET unconditionally. `group-view` does NOT exist on this version. Returns `{status:success, data: stringified-{title, group, pager}}`; not-found is signalled by **inner `group:null`** (HTTP 200, envelope `success`).
- **Rationale:** Same probe-first discipline that surfaced `user-view`'s 302 quirk; here it surfaced an outright missing action. Mirrors `userEditInner.User` null detection.

### 3.4 Project FK field (`project`)
- **Decision (post-refactor 2026-05-09b):** Expose as `project` (TF schema), `Optional + Computed`, default `0`, with `RequiresReplace`. Type `Int64`. `0` selects the **system flavour** (org-wide RBAC); positive integers select **project-scoped** groups.
- **Rationale:** Probe revealed that the same `zt_group` row shape carries both system groups (project=0) and project-scoped groups (project>0); CRUD plumbing is identical for both. Making `project` Required would force users to spell out `project = 0` for system groups; making it Optional with default 0 keeps the most-common project-scoped form ergonomic while still accepting the system flavour without a special-case attribute. RequiresReplace remains correct because moving across the system/project boundary or between projects is not a documented Controller operation.

> Earlier draft (pre-refactor): `Required + RequiresReplace`, no default, system groups out of scope. The unification dropped that constraint after the probe surfaced that system-group CRUD is the same surface as project-scoped CRUD.

### 3.5 `name` field
- **Decision:** `Required`, type `String`. Mutable (no RequiresReplace).
- **Rationale:** Standard.

### 3.6 `desc` field
- **Decision:** `Optional + Computed`, type `String`. Mutable.
- **Rationale:** ZenTao's `desc` is HTML on most pages but plain text on the group form (per upstream control.php). Optional+Computed lets the server seed defaults without producing apply-time drift; we will probe whether the server normalises whitespace / strips HTML before deciding plan-modifier shape.

### 3.7 `role` field
- **Decision:** `Optional`, type `String`, no static default. Probe enumerates the valid values; if there are exactly two or three options, we'll add an enum validator at implementation time.
- **Rationale:** The upstream `groupCreate` form lists role as a free-text field bound to ZenTao's built-in role table; some installs add custom roles. A whitelist would force users to override. Probe verifies which strings the server actually accepts.

### 3.8 DELETE semantics
- **Decision (post-probe):** `GET /group-delete-<id>.json` (no `confirm=yes`, no body) is the destructive action — **immediate effect, no two-step confirmation** unlike `user-delete`. Idempotent on missing rows: re-delete and never-existed both return `{result:success, message:"", load:"/zentao/group-browse.json"}`. Wrapper treats every successful envelope as `nil` (success).
- **Rationale:** Probe-verified shape. Critical safety note: the wrapper docstring MUST flag this as a destructive GET so future readers don't mistake it for a read.

> **Real-incident note:** During Phase 4 probe, this characteristic accidentally deleted the system `admin` group (id=1) when the operator assumed `confirm=yes` was required. Recovered by `POST /group-create.json` with original fields; new id is 10000001. Lesson encoded in spec §0 and in the eventual `DeleteGroup` doc comment.

### 3.9 Read primitive's not-found shape
- **Decision (post-probe):** Sole observed not-found marker is `{status:success, data: "{...,group:null,...}"}` (HTTP 200 + envelope success + inner `group:null`). Wrapper also defensively handles HTTP 404, the empty-marker `data:"...group:false..."` shape, and `null` for parity with `GetUser`. No 401/403 observed — auth failures redirect to `/user-login` (handled by `isControllerSessionExpired`).
- **Rationale:** Probe-verified. Defensive handling for shapes not observed costs nothing and keeps wrapper symmetric with `GetUser`.

### 3.9b Update silent-no-op on missing id (NEW — surfaced by probe)
- **Decision:** `UpdateGroup` MUST re-read after a successful POST and treat post-read `ErrNotFound` as `ErrNotFound` for the Update call. Silent success on missing id is the most surprising behaviour surfaced by the probe.
- **Rationale:** `POST /group-edit-99999.json` returns `{load:true, closeModal:true, result:success, message:"保存成功"}` even though no row was created and no row matched. Without re-read, callers would observe success on phantom updates.

### 3.10 ID assignment on Create
- **Decision (post-probe + post-refactor):** Create response is `{load:true, result:success, message:"保存成功"}` — **no id echoed**. Wrapper looks up the new id by listing and filtering `groups[]` by `name`. The list endpoint depends on the group's flavour:
  - `project = 0` → list via `/group-browse.json` (system view; project-scoped rows are excluded on Max 8.x).
  - `project > 0` → list via `/project-group-<projectID>.json`.
- **Rationale:** Probe-confirmed that the system-wide `/group-browse.json` does NOT include project-scoped (project>0) rows, so a single endpoint cannot serve both flavours. Routing on `project` value at lookup time keeps `CreateGroup` flavour-agnostic at the call site.

### 3.11 Resource and data-source schema

| TF attribute | Type | R/O | Notes |
|---|---|---|---|
| `id` | String | Computed | server-assigned (stringified numeric for Plugin Framework conventions) |
| `project` | Int64 | Optional + Computed + RequiresReplace | default `0` (system flavour); positive int → project-scoped |
| `name` | String | Required | mutable |
| `desc` | String | Optional + Computed | mutable; default `""` (server normalises empty) |
| `role` | String | Optional + Computed | mutable; free-text (no enum validator on this version) |

Data source: same fields, all Computed except `id` (Required — lookup key).

### 3.12 Branch + commits
- **Decision:** Stay on `feat/project-resource`; append the four project-group commits on top of the existing project commits.
- **Rationale:** User explicitly chose "use the current branch." Both features will ship in the same PR or two stacked PRs at the user's discretion.

Commits, in order:
1. `docs: probe-driven design contract for st-zentao_group`
2. `feat(zentaoAPI): Group type + Controller CRUD`
3. `feat(zentao): st-zentao_group resource + data source`
4. `docs: group examples + README section + regenerated tfplugindocs output`

### 3.13 Probe execution
- **Decision:** `direnv exec .` + `curl` + `jq`, same recipe as the `project` resource probe. `.envrc` already has `ZENTAO_URL` / `ZENTAO_ACCOUNT` / `ZENTAO_PASSWORD` populated.
- **Rationale:** Consistency with the prior probe; no new tooling needed.

## 4. Risks

- **`project-groupView` may return rendered HTML** instead of JSON envelope. ZenTao Controller routes occasionally do this for "view" actions. Mitigation: probe and fall back to `groupEdit` GET as the read primitive (see 3.3).
- **Permission group endpoints may be locked behind a separate priv check** that the integration test account doesn't hold. Mitigation: probe with the admin account (`.envrc` ZENTAO_ACCOUNT) which has groupCreate/groupEdit privs; if blocked, surface clearly and re-grill scope.
- **Project-scoped vs system-scoped group module ambiguity**: ZenTao has both `module/group/` (system RBAC) and project-scoped groups under `module/project/group*` actions. The user explicitly anchored to `module/project/control.php`, so the URL prefix is `project-groupXxx` (action prefix), not `group-xxx`. Probe verifies.
- **Numeric columns flip between native int and JSON string** (per the Controller probe-controller-auth.md addendum). Use `json.Number` for every numeric field on the wire, mirror `userCtrlWire`.

## 5. Probe checklist (Phase 4 input)

1. POST `/api.php/v1/tokens` to obtain sessionID.
2. List existing project groups (probe `project-groupBrowse-<projectID>.json` to learn the listing shape if it exists).
3. Read an existing group via `project-groupView-<id>.json` AND `project-groupEdit-<id>.json` — record envelope shape, status, inner key.
4. Create a probe group with minimum fields (`name`, `project`); record success envelope, whether ID is echoed.
5. Edit the probe group (rename) via `project-groupEdit-<id>.json` POST form-urlencoded.
6. Delete the probe group via `project-groupDelete-<id>.json?confirm=yes`; record envelope.
7. Read deleted group → record not-found shape (HTTP 404 vs envelope-fail).
8. Repeat delete on missing → record idempotency shape.
9. Probe each likely `role` value for accept/reject envelope.
10. Cleanup: list project groups, ensure no `tf-probe-*` leftovers.

Findings land in `docs/superpowers/specs/probe-group-controller.md`.
