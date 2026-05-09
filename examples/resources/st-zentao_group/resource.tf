# A project-scoped permission group: lives under project=28 and grants
# the bound role within that project only.
resource "st-zentao_group" "developers" {
  project = 28
  name    = "Developers"
  role    = "dev"
  desc    = "Developers working on this project (Terraform-managed)."
}

# A system group: org-wide RBAC. Omitting `project` (or setting it to 0)
# selects the system flavour. Most installs reserve system groups for
# manual administration — only manage them via Terraform when you
# specifically need org-level RBAC under IaC.
resource "st-zentao_group" "org_finance" {
  name = "Finance Reviewers"
  role = "fin"
  desc = "Org-wide finance reviewers (Terraform-managed)."
}

# NOTE: permission groups are managed via ZenTao's Controller transport
# (PATH_INFO routing) because the public V1/V2 RESTful API does NOT
# expose them on Max 8.x. The provider hides this — usage is the same
# as any other Plugin Framework resource. Member assignment and
# per-group permission lists are NOT in scope for this resource; they
# will land as separate resources (`st-zentao_group_members` /
# `st-zentao_group_privs`) in a follow-up.
