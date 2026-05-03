resource "st-zentao_product" "example" {
  name        = "Demo Product"
  description = "Created by Terraform"

  # Optional v2 fields — set as needed for your ZenTao edition.
  # ZenTao Biz / Max require `program`; open source / Pro can leave it 0.
  program = 0
  type    = "normal" # one of: normal, branch, platform
  acl     = "open"   # one of: open, private

  po       = "productManager"
  qd       = "qaLead"
  rd       = "releaseManager"
  reviewer = ["reviewer1", "reviewer2"]
}
