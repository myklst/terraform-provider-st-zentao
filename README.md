# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/) — self-hosted open source plus Pro / Biz / Max editions.

## Architecture

The HTTP client (`zentaoAPI/`) is split into three transport files, each owning the full request lifecycle (URL composition, body encoding, expiry detection, refresh & replay) for one ZenTao surface:

- `apiv1_transport.go` → `doV1Request` (RESTful, `Token:` header, expiry = 401/403)
- `apiv2_transport.go` → `doV2Request` (RESTful, `Token:` header, expiry = 401)
- `controller_transport.go` → `doController` + `doControllerForm` (PATH_INFO, `?zentaosid=` query, expiry = 302→user-login or 200+please-login envelope)

A single `Login()` (in `client.go`) calls `POST /api.php/v1/tokens` and stores the resulting sessionID in `*Client`; per probe, the same sessionID authenticates every transport, so refresh remains a single round-trip even when several transports are in concurrent use. Common HTTP plumbing (URL composition, optional zentaosid injection, 5xx backoff) is concentrated in `client.go`'s `sendHTTP` helper, and the send→detect-expiry→refresh→replay loop is shared via `doWithRefresh`, parameterised by each transport's expiry detector.

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

### `st-zentao_group`

```hcl
# Project-scoped (recommended): grants the role inside one project only.
resource "st-zentao_group" "developers" {
  project = 28          # Optional + RequiresReplace; default 0 (system flavour)
  name    = "Developers"
  role    = "dev"       # Optional, free-text (bound to ZenTao role registry)
  desc    = "Developers working on this project (Terraform-managed)."
}

# System-scoped: org-wide RBAC. Omit `project` (or set it to 0) to opt in.
resource "st-zentao_group" "org_finance" {
  name = "Finance Reviewers"
  role = "fin"
}
```

Manages a row in `zt_group`. The same row shape covers two flavours,
distinguished by the `project` attribute:

- `project = 0` (the default) → **system group** (org-wide RBAC; e.g.
  the built-in admin group)
- `project > 0` → **project-scoped permission group** (the in-project
  RBAC bucket users typically think of as "project group")

Backed by the **Controller transport**, not V2 — the V1/V2 RESTful APIs
do not expose group CRUD on ZenTao Max 8.x. URL plumbing routes through
`module/group/control.php` for both flavours; the `module/project/control.php`
action of the same name is just a per-project listing view, not its own
CRUD module.

Two non-obvious server behaviours encoded in the wrapper (see the spec
for the full probe transcript):

- **Update silently no-ops on missing rows.** `POST /group-edit-<id>.json`
  on a non-existent id returns the same `{result:success, message:"保存成功"}`
  envelope as a real update. The wrapper re-reads after every POST and
  surfaces `ErrNotFound` when the row is gone.
- **Delete is a destructive `GET` with no `confirm=yes` gate** (unlike
  `user-delete`). The endpoint executes immediately. Idempotent on
  missing rows.

> ⚠️ Managing system groups (`project = 0`) via Terraform affects
> org-wide RBAC. Most installs reserve system groups for manual admin
> work; prefer `project > 0` unless you specifically intend to put
> org-level RBAC under IaC.

In-scope for this resource: `project`, `name`, `role`, `desc`. Out of
scope (planned as separate resources): per-group permission lists
(`groupManagePriv`) and group membership (`zt_usergroup` joins).

See `docs/superpowers/specs/probe-group-controller.md` for the full
probe transcript including the §0 safety notes that drove these
decisions.

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

### `st-zentao_group`

Look up an existing permission group by its numeric id:

```hcl
data "st-zentao_group" "existing" {
  id = "10000002"
}

output "group_name" {
  value = data.st-zentao_group.existing.name
}
```

The data source surfaces both system groups (`project=0`) and
project-scoped groups (`project>0`); the matching `st-zentao_group`
resource manages either flavour.

## API client only — `User`

The `zentaoAPI.Client` exposes typed methods for ZenTao users via the Controller transport (no Terraform resource wraps this yet — coming after the wrapper stabilises across more entities):

```go
// Read — works on any instance:
u, err := client.GetUser(ctx, 1)            // by numeric id

// Write — instance-dependent:
_, err = client.CreateUser(ctx, &zentaoapi.User{
    Account: "alice", Password: "P@ssw0rd", Realname: "Alice",
    Email: "alice@example.test", Dept: 500, Gender: "f",
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
export TF_ACC=1
export ZENTAO_URL=http://localhost:8080
export ZENTAO_ACCOUNT=admin
export ZENTAO_PASSWORD=...
make go-test-acc
```

## References

- https://www.zentao.net/book/api/2309.html
