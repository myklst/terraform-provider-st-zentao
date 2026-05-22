resource "st-zentao_system" "example" {
  product = 1 # owning product id; changing it replaces the application
  name    = "Back Office"
  desc    = "Created by Terraform"
  status  = "active" # active | inactive
}
