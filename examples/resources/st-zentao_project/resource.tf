resource "st-zentao_project" "example" {
  name  = "Smart Home Sprint"
  model = "scrum"
  begin = "2026-01-01"
  end   = "2026-12-31"

  workflow_group = 1

  products = [1]
  multiple = true

  program = 1
  pm      = "testPM"
  desc    = "Created by Terraform"
}
