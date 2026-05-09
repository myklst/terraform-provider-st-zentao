resource "st-zentao_program" "example" {
  name  = "Smart Home"
  begin = "2026-01-01"
  end   = "2026-12-31"
  desc  = "Created by Terraform"

  pm          = "testPM"
  acl         = "private" # open | private | custom
  budget      = "100000"
  budget_unit = "CNY"
  # whitelist = "alice,bob" # only when acl = "custom"
}
