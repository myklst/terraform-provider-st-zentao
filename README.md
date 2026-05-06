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

## API client only — `User`

The `zentaoAPI.Client` exposes typed methods for ZenTao users via the Controller transport (no Terraform resource wraps this yet — coming after the wrapper stabilises across more entities):

```go
// Read — works on any instance:
u, err := client.GetUser(ctx, 1)            // by numeric id

// Write — instance-dependent:
_, err = client.CreateUser(ctx, &zentaoapi.User{
    Account: "alice", Password: "P@ssw0rd",
    Realname: "Alice", Email: "alice@example.test",
    Dept: 500, Gender: "f",
})
_, err = client.UpdateUser(ctx, &zentaoapi.User{
    ID: 7, Account: "alice", Realname: "Alice Renamed",
    VerifyPassword: "<admin-password>", // see caveat below
})
err = client.DeleteUser(ctx, 7)
```

**Read primitive uses `user-edit-<id>` GET, not `user-view`.** ZenTao Max 8.x always 302s `user-view-<x>.json` to that user's todo calendar, so the read implementation pulls the user record out of the edit-form context envelope. This is invisible to callers but worth knowing if you compare with the reference API docs.

**Two version-specific caveats:**

- **License cap on create.** Editions enforcing a licensed user count (Pro / Biz / Max) reject `CreateUser` with the verbatim license message once the cap is reached. The wrapper surfaces it as `*APIError` with the original Chinese / English text intact so you can detect and bump licensing rather than retry.
- **VerifyPassword sudo gate on update / delete.** Some editions (observed: ZenTao Max 8.1) require the calling admin to re-confirm their password as `verifyPassword` for every mutating user-controller operation. Set `User.VerifyPassword` accordingly. Editions without the gate ignore the field. The exact hashing scheme (plain / md5 / salted) varies and is not documented; the wrapper passes the field verbatim.

`Password` and `VerifyPassword` are write-only on the `User` struct: read-side methods leave them empty, and `CreateUser` zeros them on the returned `*User` so the round-trip can never accidentally surface them in error formatting or logs.

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
