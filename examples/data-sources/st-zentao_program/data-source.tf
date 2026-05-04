data "st-zentao_program" "by_id" {
  id = "1"
}

output "program" {
  value = {
    name        = data.st-zentao_program.by_id.name
    begin       = data.st-zentao_program.by_id.begin
    end         = data.st-zentao_program.by_id.end
    pm          = data.st-zentao_program.by_id.pm
    status      = data.st-zentao_program.by_id.status
    opened_by   = data.st-zentao_program.by_id.opened_by
    progress    = data.st-zentao_program.by_id.progress
    budget      = data.st-zentao_program.by_id.budget
    budget_unit = data.st-zentao_program.by_id.budget_unit
  }
}
