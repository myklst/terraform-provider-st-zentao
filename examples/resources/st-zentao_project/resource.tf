resource "st-zentao_project" "example" {
  name  = "Smart Home Sprint"
  model = "scrum"
  begin = "2026-01-01"
  end   = "2026-12-31"

  # Required by ZenTao Max 8.x: at least one product id and a workflow
  # group id. The validator name "productsBox" surfaces in error
  # messages but the wire field is "products". The workflow group id
  # is install-specific (commonly 1 = default).
  products       = [1]
  workflow_group = 1

  # Optional: parent program (mapped to ZenTao "parent"). 0 / unset
  # leaves the project at top level — the server does NOT auto-assign
  # a default program.
  program = 1

  description = "Created by Terraform"

  # Optional: assign a PM. ZenTao auto-assigns the calling account
  # when unset.
  pm = "testPM"
}
