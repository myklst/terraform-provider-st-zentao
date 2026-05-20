# terraform-provider-st-zentao

A Terraform provider wrapping ZenTao's HTTP APIs. This glossary captures the
ZenTao domain terms that are easy to confuse when modelling resources and data
sources — especially where ZenTao's wire vocabulary overloads a single word.

## Language

**Workflow Group**:
A preset row in ZenTao's `zt_workflowgroup` table that the project resource's
`workflow_group` form input references by numeric id.
_Avoid_: workflow scheme, flow.

**Workflow Group Type** (`type`):
Which *catalog* of workflow groups a row belongs to — `product` (产品流程,
served by `workflowgroup-product`) or `project` (项目流程, served by
`workflowgroup-project`). It is the `zt_workflowgroup.type` column and selects
the controller endpoint.
_Avoid_: conflating with **Project Type**.

**Project Type** (`projectType`):
A per-row classifier *within* the project-flow catalog — `product` or
`project`. It is NOT the same axis as **Workflow Group Type**; a row can have
`type=product` while `projectType=project` (the factory 默认流程 does exactly
this).

**Project Model** (`projectModel`):
The methodology of a project-flow workflow group — `scrum` / `waterfall` /
`kanban` / `agileplus` / `waterfallplus` / `cmmi`. Empty for product-flow rows.

## Relationships

- A **Workflow Group** has exactly one **Workflow Group Type** (`product` | `project`).
- The `product` catalog ships a single default row whose **Project Model** is empty.
- The `project` catalog subdivides rows by (**Project Model** × **Project Type**).
- **Workflow Group Type** selects the endpoint; **Project Model** / **Project Type**
  only further-filter within the `project` catalog.

## Example dialogue

> **Dev:** "If I ask for a `product` workflow group, do I still pass a project_model?"
> **Domain expert:** "No — product-flow rows have an empty projectModel. The
> product catalog is just the single 默认流程 row. project_model and
> project_type only make sense when type=project."

## Flagged ambiguities

- `type` was overloaded: the request first used it as the product/project
  discriminator already covered by `projectType`. Resolved: `type` is the
  **Workflow Group Type** (catalog/endpoint selector), a distinct axis from
  **Project Type**.
