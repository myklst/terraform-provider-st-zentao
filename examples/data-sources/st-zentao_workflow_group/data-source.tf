# A project flow: `type = "project"` requires project_model and project_type.
data "st-zentao_workflow_group" "scrum_product" {
  type          = "project"
  project_model = "scrum"
  project_type  = "product"
}

# A product flow: `type = "product"` takes no project_model / project_type.
data "st-zentao_workflow_group" "default_product" {
  type = "product"
}

resource "st-zentao_project" "example" {
  name           = "Smart Home Sprint"
  model          = "scrum"
  begin          = "2026-01-01"
  end            = "2026-12-31"
  acl            = "open"
  workflow_group = data.st-zentao_workflow_group.scrum_product.id
  products       = [1]
}
