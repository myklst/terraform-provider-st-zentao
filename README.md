# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/) — self-hosted open source plus Pro / Biz / Max editions. Talks to the **ZenTao RESTful API v2** (`/api.php/v2/...`) using token-based authentication.

## Status

Initial release: ships the `st-zentao_product` resource and the `st-zentao_product` data source. More resources (project, execution, user, group) planned.

## Local installation

```bash
make install-local-custom-provider
```

Then in your Terraform config:

```hcl
terraform {
  required_providers {
    st-zentao = {
      source = "example.local/myklst/st-zentao"
    }
  }
}

provider "st-zentao" {
  url      = "http://localhost:8080"
  account  = "admin"
  password = "P@ssw0rd"
}
```

`account` and `password` may also be supplied via `ZENTAO_ACCOUNT` / `ZENTAO_PASSWORD`.

## Resources

### `st-zentao_product`

```hcl
resource "st-zentao_product" "demo" {
  name        = "Demo Product"
  description = "Managed by Terraform"

  program  = 0          # 0 = unassigned; required >0 for ZenTao Biz/Max
  type     = "normal"   # normal | branch | platform
  acl      = "open"     # open | private
  po       = "alice"
  reviewer = ["bob"]
}
```

The full writeable field set mirrors the v2 `POST /api.php/v2/products` body:
`name`, `program`, `line`, `type`, `desc` (mapped as `description`), `acl`,
`po`, `qd`, `rd`, `reviewer`.

Server-managed read-only outputs include `id`, `code`, `status`, `created_by`,
`created_date`, and `program_name`. ZenTao v2 does not accept `code` on write
— it is exposed as a Computed attribute only.

## Data Sources

### `st-zentao_product`

Look up an existing product by its numeric id:

```hcl
data "st-zentao_product" "existing" {
  id = "1"
}

output "product_name" {
  value = data.st-zentao_product.existing.name
}
```

## Development

```bash
make go-test-unit          # unit tests, no ZenTao required
make go-test-acc           # acceptance tests, requires a real ZenTao
make generate-docs         # regenerate docs/ via tfplugindocs
make go-lint
```

For acceptance tests:

```bash
docker compose -f examples/docker-compose.yml up -d
export TF_ACC=1
export ZENTAO_URL=http://localhost:8080
export ZENTAO_ACCOUNT=admin
export ZENTAO_PASSWORD=...
make go-test-acc
```

## Why a custom provider

myklst maintains a family of `terraform-provider-st-*` providers for use cases not covered by upstream providers. This one fills the gap for ZenTao project management.

## License

See `LICENSE`.
