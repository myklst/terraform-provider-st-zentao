# Manages the complete privilege set of a permission group. Applying
# replaces the group's privileges; destroying clears them. Each priv is
# "module-method"; an empty set asserts the group has no privileges.

resource "st-zentao_group" "developers" {
  project = 28
  name    = "Developers"
}

resource "st-zentao_group_privs" "developers" {
  group = tonumber(st-zentao_group.developers.id)
  privs = [
    "story-view",
    "story-create",
    "task-view",
    "task-finish",
  ]
}
