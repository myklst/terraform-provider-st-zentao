terraform {
  required_providers {
    st-zentao = {
      source = "registry.terraform.io/myklst/st-zentao"
    }
  }
}

provider "st-zentao" {
  url      = "http://localhost:8080"
  account  = "admin"
  password = "P@ssw0rd"
}
