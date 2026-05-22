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

**Group Priv** (`zt_grouppriv`):
A single (module, method) privilege grant belonging to a **permission group**.
A group's complete privilege set is the collection of these rows. Managed via
the Controller `project-managePriv-{projectID}-{groupID}` action, where
`projectID` is the group's own `project` column (`0` for system groups, `> 0`
for project-scoped) — one path serves both scopes. Modelled as the standalone
`st-zentao_group_privs` resource, never an inline field on the group entity.
_Avoid_: "permission" (ambiguous between group membership and module/method grant).

**System** (应用):
A ZenTao *application* — a named entity owned by a **Product**, managed through
the `system` controller module's `create` / `edit` / `delete` actions. Carries
`name`, `desc`, an **Integrated** flag, and a list of **Child Systems**.
_Avoid_: conflating with the DevOps system-admin surface (backup / upgrade /
domain / OSS) that shares the `system` module name but is a different concern.

**Integrated**:
A server-set flag on a **System** indicating it aggregates **Child Systems**.
Read-only from the provider's view — ZenTao derives it; the provider never
sets it directly.

**Child System**:
A **System** that belongs to another (integrated) **System**. The parent's
membership list is the `children` array on the parent row; the provider models
each parent→child edge as a standalone attachment, never an inline field.

**Status** (`status`):
A **System**'s enabled state — the `status` enum (`active` / `inactive`),
toggled via the `system-active-{id}` / `system-inactive-{id}` actions rather
than the edit form.
_Avoid_: calling it `active` — the stored column is `status`.

## Relationships

- A **System** belongs to exactly one **Product**.
- An **Integrated System** aggregates zero or more **Child Systems**; each
  parent→child edge is a separate attachment.
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
- `system` is overloaded in ZenTao: the module name covers both the
  DevOps system-admin surface (backup, upgrade, domain, OSS) and the
  **System** application entity (`browse` / `create` / `edit` / `active`).
  Resolved: the provider's `st-zentao_system` resource models only the
  **System** application entity.
