# Project-scoped group:
resource "st-zentao_group" "developers" {
  project = 28
  name    = "Developers"
  role    = "dev"
  desc    = "Developers working on this project."
}

# System (org-wide) group: omit `project` or set to 0.
resource "st-zentao_group" "org_finance" {
  name = "Finance Reviewers"
  role = "fin"
  desc = "Org-wide finance reviewers."
}
