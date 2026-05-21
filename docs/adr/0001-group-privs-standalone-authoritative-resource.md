# Group privileges are a standalone, full-set-authoritative resource

A permission group's privilege set (`zt_grouppriv`, a collection of `module-method`
grants) is modelled as a standalone `st-zentao_group_privs` resource keyed by group
id, **not** as an inline attribute on `st-zentao_group`. The resource owns the group's
*entire* privilege set: Create/Update submit the complete declared set via ZenTao's
`managePriv` Controller action (which is replace-all, not additive), and Delete clears
the set to empty. `privs = []` is therefore a meaningful authoritative assertion ("this
group has no privileges"), guarding against out-of-band grants made in the ZenTao UI.

## Considered Options

- **Inline `privs` field on `st-zentao_group`** — rejected. `managePriv` is a separate
  Controller step from group CRUD (`group-create`/`group-edit`), with its own ~137KB
  catalog GET and its own write semantics; folding it into the group entity's CRUD would
  couple two unrelated wire surfaces and force every group apply to round-trip the priv
  catalog.
- **Additive priv management** (resource adds the listed privs, leaves others untouched)
  — rejected as un-Terraform-like and impossible to reconcile against ZenTao's replace-all
  `managePriv` semantics; drift detection would be unreliable.

## Consequences

- Two Terraform configs pointing `st-zentao_group_privs` at the same group will fight,
  each replacing the other's set. This is inherent to full-set ownership; one resource
  per group is the contract.
- One Controller path serves both group scopes:
  `project-managePriv-{projectID}-{groupID}`, where `projectID` is derived internally
  via `GetGroup().project` (`0` for system groups). The `group-managePriv` JSON surface
  is dead on Max 8.x and is not used.
