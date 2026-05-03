# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/) — self-hosted open source plus Pro / Biz / Max editions, all of which share the same PHP REST API surface.

## Status

Initial release: ships the `st-zentao_product` resource. More resources (project, execution, user, group) planned.

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
  code        = "demo"
  description = "Managed by Terraform"
}
```

`code` requires replacement on change (ZenTao does not allow editing in place).

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
