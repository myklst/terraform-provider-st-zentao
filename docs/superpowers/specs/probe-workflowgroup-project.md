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

## 6. Addendum (2026-05-20): the sibling `workflowgroup-product` catalog

ZenTao exposes **two** workflow-group catalogs, distinguished by the
`zt_workflowgroup.type` column — NOT to be confused with the per-row
`projectType` field (see §4):

| Catalog | Endpoint | `type` column | Factory rows |
|---|---|---|---|
| 项目流程 (project flow) | `GET workflowgroup-project.json` | `project` | 10 (scrum/waterfall/… × product/project) |
| 产品流程 (product flow) | `GET workflowgroup-product.json` | `product` | 1 (默认流程) |

`type` (product | project) is the **catalog/endpoint selector**; the
`st-zentao_workflow_group` data source surfaces it as the required `type`
input. `project_model` / `project_type` only further-filter within the
**project** catalog.

### Real `workflowgroup-product.json` response (2026-05-20, factory Max 8.x)

Same stringified-`data` envelope and `groups`/`pager` shape as the project
catalog, so `CtrlResp.DecodeData` and the existing `WorkflowGroup` struct parse
it unchanged:

```json
{"status":"success","data":"{\"title\":\"产品流程列表\",\"groups\":{\"1\":{\"id\":1,\"objectID\":0,\"type\":\"product\",\"projectModel\":\"\",\"projectType\":\"project\",\"name\":\"默认流程\",\"code\":\"productproject\",\"status\":\"normal\",\"main\":1,\"vision\":\"rnd\",\"deleted\":0}},\"pager\":{\"recTotal\":1,\"pageTotal\":1,\"methodName\":\"product\"},\"browseType\":\"all\"}","md5":"..."}
```

The single factory product row:

| id | code | type | projectModel | projectType | name |
|---|---|---|---|---|---|
| 1 | productproject | product | `""` (empty) | project | 默认流程 |

Key consequences for the client:

- **`projectModel` is empty for product-flow rows.** The original
  `FindWorkflowGroup` hard-required a non-empty `projectModel`; the product
  catalog has none, which is why the data source's `project_model` /
  `project_type` are now Optional (forbidden when `type=product`, required when
  `type=project`).
- The product catalog ships exactly one row. `ListProductWorkflowGroups`
  returns it; if an admin adds more, the data source reports an ambiguity error
  (no narrowing knob exists for the product catalog).
