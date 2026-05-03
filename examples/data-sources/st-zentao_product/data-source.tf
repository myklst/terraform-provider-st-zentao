data "st-zentao_product" "by_id" {
  id = "1"
}

output "product_name" {
  value = data.st-zentao_product.by_id.name
}
