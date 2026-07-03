data "st-zentao_project" "by_id" {
  id = "28"
}

output "project" {
  value = {
    name            = data.st-zentao_project.by_id.name
    model           = data.st-zentao_project.by_id.model
    begin           = data.st-zentao_project.by_id.begin
    end             = data.st-zentao_project.by_id.end
    program         = data.st-zentao_project.by_id.program
    workflow_group  = data.st-zentao_project.by_id.workflow_group
    acl             = data.st-zentao_project.by_id.acl
    auth            = data.st-zentao_project.by_id.auth
    task_date_limit = data.st-zentao_project.by_id.task_date_limit
    story_types     = data.st-zentao_project.by_id.story_types
    status          = data.st-zentao_project.by_id.status
  }
}
