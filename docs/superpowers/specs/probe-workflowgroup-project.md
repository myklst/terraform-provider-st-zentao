# Probe: ZenTao Workflow Group catalog (workflowgroup-project)

**Date:** 2026-05-20
**Server:** ZenTao Max 8.x at `${ZENTAO_URL}`
**Tool:** raw `curl` via `direnv exec .`

> Source of truth for `zentaoAPI/workflow_group.go`. The data source
> `st-zentao_workflow_group` resolves a `(project_model, project_type)` pair
> to the numeric `workflow_group` id consumed by `st-zentao_project`. This
> probe is the basis for replacing the original two-endpoint enumeration with
> a single catalog read.

## 1. Why this endpoint (and not the old two)

The original implementation enumerated workflow groups via a brittle pair:

| Old endpoint | Role | Problem |
|---|---|---|
| `POST project-create.json` (empty body) | enumerate ids from the form's `workflowGroupPairs` map | relies on "empty POST renders the form" side effect; no full rows |
| `GET workflowgroup-view-{id}.json` | fetch one row by id | N+1: one request per id after enumeration |

`GET workflowgroup-project.json` returns the **entire** catalog in **one**
request, with full rows — so it replaces both. Old endpoints retired.

## 2. Endpoint summary

| Endpoint | Status | Notes |
|---|---|---|
| `GET workflowgroup-project.json` | works (read primitive) | Controller route, auth via `?zentaosid=`. Returns the full catalog keyed by id. `browseType:"all"` — includes both product-typed and project-typed rows. |

## 3. Real response (2026-05-20, factory Max 8.x)

Controller success envelope; `data` is a **stringified** JSON body (legacy
shape — `CtrlResp.DecodeData` handles it):

```json
{"status":"success","data":"{\"title\":\"项目流程列表\",\"groups\":{\"2\":{\"id\":2,\"objectID\":0,\"type\":\"project\",\"projectModel\":\"scrum\",\"projectType\":\"product\",\"name\":\"敏捷式产品研发\",\"code\":\"scrumproduct\",\"desc\":\"\",\"status\":\"normal\",\"vision\":\"rnd\",\"deleted\":0}, ... ,\"11\":{...}},\"pager\":{\"offset\":0,\"recTotal\":10,\"recPerPage\":20,\"pageTotal\":1,\"pageID\":1,\"moduleName\":\"workflowgroup\",\"methodName\":\"project\"},\"orderBy\":\"\",\"browseType\":\"all\"}","md5":"..."}
```

Factory catalog is 10 rows (ids 2–11):

| id | code | projectModel | projectType |
|---|---|---|---|
| 2 | scrumproduct | scrum | product |
| 3 | scrumproject | scrum | project |
| 4 | waterfallproduct | waterfall | product |
| 5 | waterfallproject | waterfall | project |
| 6 | agileplusproduct | agileplus | product |
| 7 | agileplusproject | agileplus | project |
| 8 | waterfallplusproduct | waterfallplus | product |
| 9 | waterfallplusproject | waterfallplus | project |
| 10 | kanbanproduct | kanban | product |
| 11 | kanbanproject | kanban | project |

Each `(projectModel, projectType)` pair is unique in the factory set, so
`FindWorkflowGroup` normally returns exactly one match; the ambiguity branch
only fires if an admin creates a duplicate.

## 4. Field semantics

- `groups`: `map[idString]record`. Per-row fields used by the client:
  `id`, `code`, `name`, `projectModel`, `projectType`, `vision`, `deleted`.
- **`type` is NOT `projectType`.** Every row here has `type:"project"` (the
  `zt_workflowgroup.type` column, set by the listing method), while
  `projectType` is the product/project discriminator we actually filter on.
- `deleted`: soft-delete flag (`0`/`1`). The client drops `deleted == 1`.
- `status`: observed `"normal"` for all factory rows. The disabled-row value
  was **not probed**, so the client does **not** filter on `status`.

## 5. Pagination decision (deferred)

`pager.recPerPage` defaults to 20; factory catalog is 10, so realistic
catalogs fit one page. The PATH_INFO paging-parameter format was **not
probed**.

Decision: `ListWorkflowGroups` issues a single default-page request and reads
back `pager.pageTotal`. If `pageTotal > 1` it **returns an error** rather than
silently truncating (which would make `FindWorkflowGroup` falsely report
`ErrNotFound` for groups on later pages). Implementing real pagination is
deferred until someone actually grows the catalog past one page — at which
point the paging URL form must be probed first.
