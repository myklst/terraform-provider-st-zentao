resource "st-zentao_product" "example" {
  name = "Demo Product"
  desc = "Created by Terraform"

  program = 0
  type    = "normal" # normal | branch | platform
  acl     = "open"   # open | private

  po       = "productManager"
  qd       = "qaLead"
  rd       = "releaseManager"
  reviewer = ["reviewer1", "reviewer2"]
}
