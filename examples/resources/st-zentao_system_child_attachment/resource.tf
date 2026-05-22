resource "st-zentao_system" "platform" {
  product = 1
  name    = "Platform"
}

resource "st-zentao_system" "billing" {
  product = 1
  name    = "Billing"
}

# Attach "billing" as a child of "platform". A child may belong to several
# parents, so attaching is additive.
resource "st-zentao_system_child_attachment" "platform_billing" {
  parent = st-zentao_system.platform.id
  child  = st-zentao_system.billing.id
}
