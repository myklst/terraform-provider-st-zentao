data "st-zentao_group" "by_id" {
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
