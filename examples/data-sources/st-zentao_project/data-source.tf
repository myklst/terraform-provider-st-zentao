data "st-zentao_project" "by_id" {
  id = "28"
}

output "project" {
  value = {
    name           = data.st-zentao_project.by_id.name
    model          = data.st-zentao_project.by_id.model
    begin          = data.st-zentao_project.by_id.begin
    end            = data.st-zentao_project.by_id.end
    program        = data.st-zentao_project.by_id.program
    workflow_group = data.st-zentao_project.by_id.workflow_group
    acl            = data.st-zentao_project.by_id.acl
    status         = data.st-zentao_project.by_id.status
    opened_by      = data.st-zentao_project.by_id.opened_by
    progress       = data.st-zentao_project.by_id.progress
    team_count     = data.st-zentao_project.by_id.team_count
  }
}
