# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/) — self-hosted open source plus Pro / Biz / Max editions. Authenticates via the v1 two-step apilogin flow; the resulting session drives both the **ZenTao RESTful API v2** (`/api.php/v2/...`) and the legacy **PATH_INFO Controller** routes (`/<module>-<method>-...json`), so resources can target whichever entity surface ZenTao exposes.

## Status

Initial release: ships the `st-zentao_product` and `st-zentao_program` resources, plus matching data sources. More resources (project, execution, user, group) planned — these will use the Controller transport since V2 doesn't expose them.

## Architecture

The HTTP client (`zentaoAPI/`) carries a single auth pipeline that serves two transport flavours. V2 wrappers (`api.php/v2/...`) and Controller wrappers (`<module>-<method>-...json`) both flow through the same `doRequest` → `send` → cookiejar+`Token` header chain, and share the same session refresh logic. See `docs/superpowers/specs/2026-05-06-controller-extension-stage1.md` for the design contract and `docs/superpowers/specs/probe-controller-auth.md` for the auth probe that informs it.

For Controller endpoints not yet covered by a typed wrapper, the client exposes `CallController(ctx, module, method, pathArgs, query, body)` — marked **EXPERIMENTAL**; prefer typed methods when they exist.

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

### `st-zentao_program`

```hcl
resource "st-zentao_program" "demo" {
  name        = "Smart Home"
  begin       = "2026-01-01"
  end         = "2026-12-31"
  description = "Managed by Terraform"

  pm = "alice" # optional; ZenTao auto-assigns the calling account when unset
}
```

Writeable fields per the v2 `POST /api.php/v2/programs` body: `name`, `begin`,
`end`, `pm`, `desc` (mapped as `description`). `begin` and `end` must be
`YYYY-MM-DD`. Server-managed read-only outputs include `id`, `code`, `status`,
`parent`, `type`, `category`, `acl`, `po`, `qd`, `rd`, `budget`, `budget_unit`,
`opened_by`, `opened_date`, `real_began`, `real_end`, `progress`, `team_count`.

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

### `st-zentao_program`

Look up an existing program by its numeric id:

```hcl
data "st-zentao_program" "existing" {
  id = "1"
}

output "program_name" {
  value = data.st-zentao_program.existing.name
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

## References

- https://www.zentao.net/book/api/2309.html
