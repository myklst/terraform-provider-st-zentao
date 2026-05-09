data "st-zentao_group" "by_id" {
  # Numeric group id (stringified). On ZenTao Max 8.x the id space is
  # shared across system groups (project=0) and project-scoped groups
  # (project>0); this data source surfaces both flavours.
  id = "10000002"
}

output "group" {
  value = {
    project = data.st-zentao_group.by_id.project
    name    = data.st-zentao_group.by_id.name
    role    = data.st-zentao_group.by_id.role
    desc    = data.st-zentao_group.by_id.desc
  }
}
