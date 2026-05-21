# Probe: ZenTao Workflow Group catalogs

**Date:** 2026-05-20
**Server:** ZenTao Max 8.x at `${ZENTAO_URL}`
**Tool:** raw `curl` via `direnv exec .`

> Source of truth for `zentaoAPI/workflow_group.go`. The data source
> `st-zentao_workflow_group` resolves a workflow group to the numeric
> `workflow_group` id consumed by `st-zentao_project`. Lookup is dispatched
> by a required `type` input (`product` | `project`); within the project
> catalog it is further narrowed by `(project_model, project_type)`.

## 1. Two catalogs, selected by `type`

ZenTao exposes **two** workflow-group catalogs, distinguished by the
`zt_workflowgroup.type` column — NOT the per-row `projectType` field (see §4):

| Catalog | Endpoint | `type` column | Factory rows |
|---|---|---|---|
| 项目流程 (project flow) | `GET workflowgroup-project.json` | `project` | 10 (scrum/waterfall/… × product/project) |
| 产品流程 (product flow) | `GET workflowgroup-product.json` | `product` | 1 (默认流程) |

`type` (product | project) is the **catalog/endpoint selector**, surfaced as
the required `type` input on the data source. `project_model` / `project_type`
only further-filter within the **project** catalog (Optional: forbidden when
`type=product`, required when `type=project`).

Both endpoints are Controller routes, auth via `?zentaosid=`, and return the
full catalog keyed by id in **one** request (`browseType:"all"`).

## 2. `workflowgroup-project.json` (project catalog)

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

## 3. `workflowgroup-product.json` (product catalog)

Same stringified-`data` envelope and `groups`/`pager` shape as the project
catalog, so `CtrlResp.DecodeData` and the `WorkflowGroup` struct parse it
unchanged:

```json
{"status":"success","data":"{\"title\":\"产品流程列表\",\"groups\":{\"1\":{\"id\":1,\"objectID\":0,\"type\":\"product\",\"projectModel\":\"\",\"projectType\":\"project\",\"name\":\"默认流程\",\"code\":\"productproject\",\"status\":\"normal\",\"main\":1,\"vision\":\"rnd\",\"deleted\":0}},\"pager\":{\"recTotal\":1,\"pageTotal\":1,\"methodName\":\"product\"},\"browseType\":\"all\"}","md5":"..."}
```

The single factory product row:

| id | code | type | projectModel | projectType | name |
|---|---|---|---|---|---|
| 1 | productproject | product | `""` (empty) | project | 默认流程 |

Key consequences for the client:

- **`projectModel` is empty for product-flow rows**, so the data source's
  `project_model` / `project_type` are Optional and not applied to this
  catalog.
- The product catalog ships exactly one row. `ListProductWorkflowGroups`
  returns it; if an admin adds more, the data source reports an ambiguity error
  (no narrowing knob exists for the product catalog).

## 4. Field semantics

- `groups`: `map[idString]record`. Per-row fields used by the client:
  `id`, `code`, `name`, `projectModel`, `projectType`, `vision`, `deleted`.
- **`type` is NOT `projectType`.** `type` is the `zt_workflowgroup.type`
  column (the catalog selector, set by the listing method), while
  `projectType` is the product/project discriminator we filter on within the
  project catalog.
- `deleted`: soft-delete flag (`0`/`1`). The client drops `deleted == 1`.
- `status`: observed `"normal"` for all factory rows. The disabled-row value
  was **not probed**, so the client does **not** filter on `status`.

## 5. Pagination decision (deferred)

`pager.recPerPage` defaults to 20; factory catalogs are 10 and 1, so realistic
catalogs fit one page. The PATH_INFO paging-parameter format was **not
probed**.

Decision: the list calls issue a single default-page request and read back
`pager.pageTotal`. If `pageTotal > 1` they **return an error** rather than
silently truncating (which would make `FindWorkflowGroup` falsely report
`ErrNotFound` for groups on later pages). Implementing real pagination is
deferred until someone actually grows a catalog past one page — at which
point the paging URL form must be probed first.
