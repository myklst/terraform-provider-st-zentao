# Plan: `st-zentao_project` resource + data source

**Date:** 2026-05-09
**Branch:** `feat/project-resource`
**Status:** approved (post grill-me); pending probe execution
**Out of scope:** sprint resource, execution resource, lifecycle hooks (close/suspend/start), `force_destroy` flag, model-only fields like `vision/days/auth`, `custom`-acl whitelist write path.

## 1. Goal

Add `st-zentao_project` (resource + data source) to the provider, backed by ZenTao V2 RESTful API. Mirrors the shape of `st-zentao_program` and `st-zentao_product`, with project-specific deltas:

- `model` is the methodology selector (Required, enum, RequiresReplace) — **not** `type`.
- `type` is a row-discriminator on the shared `zt_project` table (`project | sprint | program | ...`); this resource locks it to `"project"` on the wire and rejects rows of other types on Read.

## 2. Endpoints (from user-supplied docs)

| Surface | Path | Doc |
|---|---|---|
| Create | POST `/api.php/v2/projects` | 2156 |
| Update | PUT  `/api.php/v2/projects/{id}` | 2157 |
| List   | GET  `/api.php/v2/projects` | 2158 |
| List under program | GET `/api.php/v2/programs/{programId}/projects` | 2159 |
| Delete | DELETE `/api.php/v2/projects/{id}` | 2252 |

> **Missing from docs:** `GET /api.php/v2/projects/{id}` (single Read). Must be probed before relying on it.

## 3. Decision log

Decisions reached during the 2026-05-09 grill-me session.

### 3.1 Read path
- **Decision:** Probe whether `GET /api.php/v2/projects/{id}` exists. If yes, use it (mirrors product/program). If no, fall back path is **TBD post-probe** (likely list-filter or controller `project-view`).
- **Rationale:** ZenTao docs frequently omit endpoints that exist in production. Verifying with curl is cheaper than designing around an absence.

### 3.2 Parent program field
- **Decision:** Expose as `program` (TF schema) → `parent` (likely wire field, confirm at probe), `Optional + Computed`, no static default, type `Int64`.
- **Rationale:** Mirrors `resource_product.program` exactly ([resource_product.go:93-99](../../../zentao/resource_product.go#L93-L99)). ZenTao auto-assigns a default program when unset; static default would risk apply-time drift.

### 3.3 `model` (methodology)
- **Decision:** **Required**, `enum` validator, `RequiresReplace` plan modifier.
- **Enum:** `scrum / waterfall / kanban / agileplus / waterfallplus / cmmi` (Max-targeted full set; probe-verified, not pre-filtered to public-edition subset).
- **Rationale:** ZenTao's methodology selector drives the entire view + state machine; PUT-time changes are silently ignored on most versions, so plan-time RequiresReplace is correct.

### 3.4 `type` (row discriminator)
- **Decision:** **Not exposed in TF schema.** Wire body always sends `type: "project"`. Read returns `ErrNotFound` if server returns `type != "project"`.
- **Rationale:** `zt_project` table holds projects + sprints + programs distinguished by `type`. Resource name `st-zentao_project` should manage exactly `type=project` rows. Sprint support is a future `resource_sprint`; programs already have `resource_program`.

### 3.5 Required field set
- **Decision:** `name + model + begin + end` are Required.
  - `begin` / `end`: `^\d{4}-\d{2}-\d{2}$` regex, mirrors program ([resource_program.go:30](../../../zentao/resource_program.go#L30)).
  - kanban edge case (begin/end optional under kanban model): **deferred** — first version applies uniformly.

### 3.6 `code`
- **Decision:** Server-managed (Computed-only, `json:"-"`), mirrors `Product.Code` ([product.go:33](../../../zentaoAPI/product.go#L33)).

### 3.7 `acl`
- **Decision:** `Optional + Computed`, no static default, enum = `open / private / custom`.
- **Note:** `custom` does not pull a whitelist — first version exposes the enum only; whitelist write path deferred.

### 3.8 Roles & description
- **Decision:** `pm / po / qd / rd` are `Optional + Computed` with no static default (mirrors product role fields, [resource_product.go:130-156](../../../zentao/resource_product.go#L130-L156)). Wire JSON keys: `PM / PO / QD / RD`.
- **Decision:** `description` is `Optional + Computed` with `static default ""`, wire key `desc` (mirrors product/program convention).

### 3.9 Server-managed read-only fields
- **Decision:** All Computed-only with `json:"-"` on the API struct:
  - `status`, `parent`, `code`, `opened_by`, `opened_date`, `last_edited_by`, `real_began`, `real_end`, `progress`, `team_count`, `budget`, `budget_unit`.

### 3.10 `lifetime` (Max-specific)
- **Decision:** Computed-only (read-only). Do not let users write it in the first version; observe server behavior and re-evaluate post-probe.

### 3.11 `days / auth / vision`
- **Decision:** Do **not** expose in the first version. Re-evaluate when there is a real user need.

### 3.12 DELETE semantics
- **Decision:** No pre-flight cleanup. Errors from server pass through verbatim. Idempotent on missing rows: HTTP 404 *and* `HTTP 200 + status=fail + "does not exist"` both treated as success (mirrors [product.go:249-271](../../../zentaoAPI/product.go#L249-L271)).
- **No `force_destroy` flag** — explicitly avoided as premature design.

### 3.13 Branch
- **Decision:** `feat/project-resource` (overrides repo's `zentao/task-NNNN` convention per user's global feedback memory).

### 3.14 Probe workflow
- **Decision:** Spec-first.
  1. Write integration probe tests under `//go:build integration`.
  2. Run them against a real ZenTao instance (env vars provided by user).
  3. Capture results in `docs/superpowers/specs/probe-project-v2.md`.
  4. Reconcile §3.1–§3.11 decisions against spec; revise schema where probe contradicts.
  5. Then, and only then, write production code.

### 3.15 Probe items (9)
1. `GET /api.php/v2/projects/{id}` existence + envelope shape.
2. `POST /projects` without `program`/`parent` → which program does the row land in?
3. `model` enum acceptance for each value (`scrum/waterfall/kanban/agileplus/waterfallplus/cmmi`).
4. `type` default when unset; whether `type:"project"` is accepted explicitly.
5. `acl: "custom"` without whitelist — server response.
6. PUT changing `model` — does server actually mutate the row, or silently ignore?
7. DELETE on already-deleted / nonexistent id — exact response shape.
8. `lifetime` field values surfaced in GET.
9. Wire field name for parent program: `parent` vs `program`.

### 3.16 PR & commit strategy
- **Single PR** (matches existing repo convention).
- **Parallel agents** for steps 6a/6b/6c after the API client is in place:
  - 6a: `resource_project.go` + tests
  - 6b: `data_source_project.go` + tests
  - 6c: `provider.go` registration

## 4. Execution order

```
1. git checkout -b feat/project-resource
2. Add probe tests to zentaoAPI/client_integration_test.go (//go:build integration)
3. Run probes; capture results to docs/superpowers/specs/probe-project-v2.md
4. Reconcile §3 decisions against probe results; update this plan if needed
5. zentaoAPI/project.go + project_test.go (TDD; httptest only)
6. Parallel: resource_project.go + data_source_project.go + provider.go reg (3 agents)
7. Acceptance tests (TF_ACC=1) — manual run
8. make generate-docs
9. Open PR
```

## 5. File layout

New files:
```
zentaoAPI/project.go
zentaoAPI/project_test.go
zentao/resource_project.go
zentao/resource_project_test.go
zentao/data_source_project.go
zentao/data_source_project_test.go
docs/superpowers/specs/probe-project-v2.md
docs/data-sources/project.md          # via tfplugindocs
docs/resources/project.md             # via tfplugindocs
```

Modified files:
```
zentao/provider.go                    # +2 lines: NewProjectResource, NewProjectDataSource
zentaoAPI/client_integration_test.go  # +probe tests
```

## 6. Open questions that block execution

1. **Env vars for probe:** awaiting `ZENTAO_URL / ZENTAO_ACCOUNT / ZENTAO_PASSWORD` from user (or shell already set in `.envrc`).
2. **Existing test program/product IDs** for probe context (creating a fresh project requires a parent program id — spec needs to know which).

Once both unblock, execution begins at step 1.
