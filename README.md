# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/) — self-hosted open source plus Pro / Biz / Max editions. Authenticates via `POST /api.php/v1/tokens` (the documented [V1 token endpoint](https://www.zentao.net/book/api/664.html)); the issued sessionID drives all three of ZenTao's API surfaces, so resources can target whichever surface exposes a given entity:

- **API V1** (`/api.php/v1/...`) — RESTful, JSON, `Token:` header (open-source 16.5+)
- **API V2** (`/api.php/v2/...`) — RESTful, JSON, `Token:` header
- **Controller / PATH_INFO** (`/<module>-<method>-...json`) — legacy, JSON or form-urlencoded, `?zentaosid=` query parameter

## Status

Initial release: ships the `st-zentao_product`, `st-zentao_program`, and `st-zentao_project` resources (V2-backed), plus matching data sources, plus typed wrappers for User CRUD (Controller-backed, since V2 doesn't expose users on Max 8.x). More resources (execution, group) planned — choice of transport per entity follows what the target ZenTao version actually accepts.

## Architecture

The HTTP client (`zentaoAPI/`) is split into three transport files, each owning the full request lifecycle (URL composition, body encoding, expiry detection, refresh & replay) for one ZenTao surface:

- `apiv1_transport.go` → `doV1Request` (RESTful, `Token:` header, expiry = 401/403)
- `apiv2_transport.go` → `doV2Request` (RESTful, `Token:` header, expiry = 401)
- `controller_transport.go` → `doController` + `doControllerForm` (PATH_INFO, `?zentaosid=` query, expiry = 302→user-login or 200+please-login envelope)

A single `Login()` (in `client.go`) calls `POST /api.php/v1/tokens` and stores the resulting sessionID in `*Client`; per probe, the same sessionID authenticates every transport, so refresh remains a single round-trip even when several transports are in concurrent use. Common HTTP plumbing (URL composition, optional zentaosid injection, 5xx backoff) is concentrated in `client.go`'s `sendHTTP` helper, and the send→detect-expiry→refresh→replay loop is shared via `doWithRefresh`, parameterised by each transport's expiry detector.

See `docs/superpowers/specs/2026-05-06-controller-extension-stage1.md` for the original design contract and `docs/superpowers/specs/probe-controller-auth.md` for the auth probes (Controller-flavoured 2026-05-06; V1 token cross-transport compatibility 2026-05-08).

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

### `st-zentao_project`

```hcl
resource "st-zentao_project" "demo" {
  name  = "Smart Home Sprint"
  model = "scrum"     # scrum | waterfall | kanban | agileplus | waterfallplus | cmmi
  begin = "2026-01-01"
  end   = "2026-12-31"

  products       = [1] # >= 1 product id (server-required)
  workflow_group = 1   # workflow scheme id (server-required; 1 typically = default)

  program     = 1            # parent program id; 0 / unset = top-level
  description = "Managed by Terraform"
  pm          = "alice"
  acl         = "private"    # open | private | custom
}
```

The v2 `POST /api.php/v2/projects` body requires `name`, `model`, `begin`, `end`,
plus two **fields that are not in the public V2 docs but are server-enforced on
ZenTao Max 8.x**: `products` (≥ 1 product id) and `workflow_group` (any int).
`begin` and `end` must be `YYYY-MM-DD`. Changing `model` triggers a
destroy-then-create because ZenTao's per-model state machine cannot be migrated
in place. The TF attribute `program` maps to the wire field `parent` (parent
program id).

This resource manages only `type=project` rows in the shared `zt_project`
table — sprints (`type=sprint`) and programs (`type=program`) are out of
scope and remain managed via their own resources / future additions. The
read path defensively returns `ErrNotFound` if a row's `type` drifts away
from `project`, so an out-of-band edit removes the resource from state
rather than silently corrupting it.

Server-managed read-only outputs: `id`, `code`, `status`, `lifetime`,
`opened_by`, `opened_date`, `last_edited_by`, `real_began`, `real_end`,
`progress`, `team_count`, `budget`, `budget_unit`.

See `docs/superpowers/specs/probe-project-v2.md` for the full V2 surface
contract — including the `productsBox`/`products` validator-name mismatch
and the `result:fail` envelope variant on validation errors.

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

### `st-zentao_project`

Look up an existing project by its numeric id:

```hcl
data "st-zentao_project" "existing" {
  id = "28"
}

output "project_name" {
  value = data.st-zentao_project.existing.name
}
```

Note: the V2 GET endpoint does not echo the project's product associations
back, so the `products` attribute is exposed as `Computed` but always reads
as an empty list — the live association set has to be queried via the
Controller surface (not yet wrapped).

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
