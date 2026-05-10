data "st-zentao_product" "by_id" {
  id = "1"
}

output "product" {
  value = {
    name       = data.st-zentao_product.by_id.name
    code       = data.st-zentao_product.by_id.code
    po         = data.st-zentao_product.by_id.po
    status     = data.st-zentao_product.by_id.status
    created_by = data.st-zentao_product.by_id.created_by
  }
}
