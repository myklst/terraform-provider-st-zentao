# terraform-provider-st-zentao

Custom Terraform provider for [ZenTao](https://www.zentao.net/).

## Installation

```bash
make install-local-custom-provider
```

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
  name  = "Smart Home"
  begin = "2026-01-01"
  end   = "2026-12-31"
  desc  = "Managed by Terraform"

  pm          = "alice"
  acl         = "private"
  budget      = "100000"
  budget_unit = "CNY"
}
```

Required: `name`, `begin`, `end` (YYYY-MM-DD).
Optional: `pm`, `desc`, `acl` (`open` / `private` / `custom`), `budget`, `budget_unit`, `whitelist`.

Use [`st-zentao_program_parent_attachment`](#st-zentao_program_parent_attachment) to set the parent of a program.

### `st-zentao_program_parent_attachment`

```hcl
resource "st-zentao_program_parent_attachment" "child_under_parent" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.parent.id
}
```

Required: `program` (child program id), `parent` (parent program id).

### `st-zentao_product`

```hcl
resource "st-zentao_product" "demo" {
  name = "Demo Product"
  desc = "Managed by Terraform"

  program  = 0
  type     = "normal"   # normal | branch | platform
  acl      = "open"     # open | private
  po       = "alice"
  reviewer = ["bob"]
}
```

Required: `name`. Optional: `program`, `line`, `type`, `desc`, `acl`, `po`, `qd`, `rd`, `reviewer`.

### `st-zentao_project`

```hcl
resource "st-zentao_project" "demo" {
  name  = "Smart Home Sprint"
  model = "scrum"     # scrum | waterfall | kanban | agileplus | waterfallplus | cmmi
  begin = "2026-01-01"
  end   = "2026-12-31"

  workflow_group = 1

  products = [1]
  multiple = true

  program = 1
  desc    = "Managed by Terraform"
  pm      = "alice"
  acl     = "private"  # open | private | custom
}
```

Required: `name`, `model`, `begin`, `end`, `workflow_group`. Optional: `program`, `products`, `multiple`, `pm`, `desc`, `acl`, `po`, `qd`, `rd`. Changing `model` forces resource replacement.

### `st-zentao_group`

```hcl
# Project-scoped group:
resource "st-zentao_group" "developers" {
  project = 28
  name    = "Developers"
  role    = "dev"
  desc    = "Developers working on this project."
}

# System (org-wide) group:
resource "st-zentao_group" "org_finance" {
  name = "Finance Reviewers"
  role = "fin"
}
```

Required: `name`. Optional: `project` (default `0` = system group), `role`, `desc`.

> Setting `project = 0` manages an org-wide group. Use a positive `project` id for per-project permission groups.

## Data Sources

Each resource has a matching data source that takes `id`:

```hcl
data "st-zentao_product" "p" { id = "1" }
data "st-zentao_program" "p" { id = "1" }
data "st-zentao_project" "p" { id = "28" }
data "st-zentao_group"   "g" { id = "10000002" }
```

### `st-zentao_workflow_group`

Resolves a workflow group's numeric `id`.
`type` selects the catalog: `product` returns the single product flow; `project`
requires `project_model` and `project_type` to pick one project flow.

```hcl
# Project flow — project_model and project_type are required:
data "st-zentao_workflow_group" "scrum_product" {
  type          = "project"
  project_model = "scrum"
  project_type  = "product"
}

# Product flow — project_model / project_type must be omitted:
data "st-zentao_workflow_group" "default_product" {
  type = "product"
}
```

> Breaking change: `type` is now required, and `project_model` / `project_type`
> are optional (required only when `type = "project"`).

## Development

```bash
make go-test-unit          # unit tests
make go-test-acc           # acceptance tests (requires a real ZenTao)
make go-lint
make generate-docs
```

For acceptance tests:

```bash
export TF_ACC=1
export ZENTAO_URL=http://localhost:8080
export ZENTAO_ACCOUNT=admin
export ZENTAO_PASSWORD=...
make go-test-acc
```
