# Plan — st-zentao_system (应用 / application)

**Date:** 2026-05-22
**Status:** grilled + probed + reconciled. See [`probe-system-controller.md`](../specs/probe-system-controller.md) for the wire contract.
**Branch:** `feat/system-resource`
**Transport:** Controller (PATH_INFO `.json`) — `system` is controller-only on Max 8.x (V2 fallback returns `JSON_BUSINESS_FAIL`; see `probe-v2-vs-controller-coverage.md` bucket C).

## What it is

`system` = a ZenTao **application** owned by a **Product**. CRUD via the `system`
controller module. Distinct from the DevOps system-admin surface (backup /
upgrade / domain / OSS) that shares the module name — see CONTEXT.md flagged
ambiguity.

Upstream `module/system/config/form.php`:

| Field | create | edit | required | default |
|---|---|---|---|---|
| `name` | ✓ | ✓ | **yes** | — |
| `desc` | ✓ | ✓ | no | `''` |
| `integrated` | ✓ | — | no | `0` |
| `children` | ✓ | ✓ | no | `[]` |
| `createdDate` | ✓ | — | no | `now()` |
| `editedDate` | — | ✓ | no | `now()` |

## Decisions

| # | Decision | Rationale | Mirrors |
|---|---|---|---|
| 1 | **Read = `system-edit-{id}` GET (full row)**; `showAll` only for data source list | Canonical controller read primitive returns the complete row; more precise than list-filter | `program.go` edit-{id} GET |
| 2 | **`product` = Required FK, RequiresReplace** | `control.php create($productID)` scopes an application under a product | program parent-style Required FK |
| 3 | **`children` → standalone attachment resource** (`st-zentao_system_child_attachment`) | Self-referential system→system membership; avoids `for_each` self-reference cycle. Non-exclusive + additive → idempotent-adopt Create (no refuse). `(parent, child)` both Required+RequiresReplace, id `{parent}-{child}`. Edge ops (`AttachSystemChild`/`DetachSystemChild`) delegate the write to `UpdateSystem`. | `resource_program_parent_attachment.go` (§6b-ter) |
| 4 | **`integrated` = Computed read-only** | Server derives it from presence of children; user never sets it | program `parent`/`grade` server-derived |
| 5 | **`status` = Optional+Computed String** (`active`/`inactive`) + stringvalidator; Update calls `system-active-{id}` / `system-inactive-{id}` | Probe: wire column is `status` enum, not bool `active` → wire-name hard rule | project `multiple` user-toggle |
| 6 | **`name` Required; `desc` Optional+Computed; `id`/`created_date`/`last_edited_date` Computed** | form.php required flags + standard identity fields | product/program schema |
| 7 | **Branch `feat/system-resource`; curl probe** | repo convention; first-pass exploration | all prior `probe-*.md` |
| 8 | **Single PR; core first (4 commits); `child_attachment` as conditional 5th commit** | attachment shape is probe-deferred; defer to follow-up PR if probe overturns it | SKILL §7 commit shape |

## Schema sketch (post-probe)

| TF attr | Wire | Type | R/O/C | UseStateForUnknown | Notes |
|---|---|---|---|---|---|
| `id` | id | String | C | yes | server id |
| `product` | productID (create URL arg) | String | **Required**, RequiresReplace | — | int FK; immutable (not an edit-form key) |
| `name` | name | String | **Required** | — | |
| `desc` | desc | String | O+C | yes | resets to `''` if omitted on edit |
| `integrated` | integrated | Int64 | C (read-only) | yes (server-owned) | 0/1; edit form cannot set it |
| `status` | status | String | O+C | yes (user toggle) | enum `active`/`inactive`; toggled via active/inactive endpoints + stringvalidator |
| `created_by` | createdBy | String | C | yes | create-time, immutable |
| `created_date` | createdDate | String | C | yes | create-time, immutable |
| `last_edited_by` | editedBy | String | C | **no** | server recomputes on every edit (same-resource-update-derived → §6b-bis) |
| `last_edited_date` | editedDate | String | C | **no** | server recomputes on every edit (same-resource-update-derived → §6b-bis) |

`children` lives on the attachment resource, not here — but resource_system's Update **must M-Z-preserve** the baseline `children` (edit-POST wipes it when omitted). `latestRelease`/`latestDate`/`deleted` not surfaced (`deleted` drives not-found detection).

## Probe-resolved (was probe-deferred)

1. **`showAll`** ✅ exists; `CtrlEnvelope` → `data.appList` map. **Includes `deleted=1` rows** → DS must filter.
2. **`system-edit-{id}` GET** ✅ full row in `data.system`; `false` when id never existed.
3. **`product`** ✅ create URL arg `system-create-{productID}.json`; bare endpoint errors. Immutable.
4. **`children` edge** 🔄 comma-string on **parent** row (field-style). P3 collision guard applies; **re-grill attachment CRUD when building the 5th-commit attachment resource.**
5. **`status`** 🔄 wire column is `status` (enum `active`/`inactive`), not bool — expose `status` String.
6. **DELETE** 🔄 always `result:success` (idempotent); soft-delete `deleted=1`; no 404; read-on-deleted → ErrNotFound. Release/build refusal path unprobed; surface server message as-is.
7. **`integrated`** ✅ Int64 0/1, server-owned read-only.
8. **edit-POST** ✅ form-urlencoded; `children[]=<id>` array keys; omitted keys reset to default (M-Z merge required).

## Not-found detection

Read = `system-edit-{id}` GET. Treat as `ErrNotFound` when `data.system == false` **or** `data.system.deleted == 1`.

## Out of scope (v1)

- DevOps system-admin actions (backup / upgrade / domain / OSS / dashboard).
- `force_destroy` / cascade delete of releases/builds.
- Releases / builds owned by an application.
