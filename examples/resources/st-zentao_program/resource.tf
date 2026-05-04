resource "st-zentao_program" "example" {
  name        = "Smart Home"
  begin       = "2026-01-01"
  end         = "2026-12-31"
  description = "Created by Terraform"

  # Optional: assign a Program Manager. If unset, ZenTao auto-assigns
  # the calling account.
  pm = "productManager"
}
