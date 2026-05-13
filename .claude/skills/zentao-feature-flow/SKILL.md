---
name: zentao-feature-flow
description: End-to-end workflow for adding a new entity (resource + data source) to terraform-provider-st-zentao. Covers grill-driven design, live-server probing, spec-first reconciliation, TDD implementation, parallel agent dispatch, and multi-commit PR shape. Use whenever the user asks to add a new ZenTao resource / data source, port a new V2 endpoint, or extend the API client.
---

# ZenTao Feature Flow

Add a new ZenTao entity (resource + data source) end-to-end. Skipping any phase costs more downstream.

## Hard rules (non-negotiable)

- **Never commit on `main`.** Cut `feat/<topic>` first, before any working-tree change.
- **Probe before assuming.** ZenTao V2 docs routinely omit server-required fields and lie about response shapes.
- **Spec is the source of truth.** Code contradicting the spec is wrong. Update spec **first**, then code.
- **No premature design.** No `force_destroy`, no fallback transports, no abstractions "for later."
- **TF attribute names mirror ZenTao wire names.** `desc` stays `desc` (not `description`); same for `PM`/`PO`/`QD`/`RD`/`acl`/`multiple`. Only acceptable rename: when ZenTao uses two names for the same field (e.g. wire `parent` / docs `program`); pick the user-facing concept and document the wire mapping in the schema description.
- **Transport-specific rules live in companion files.** This SKILL.md covers the phases and the transport-orthogonal patterns. For the API-client template, see one of: [apiv1-transport.md](apiv1-transport.md), [apiv2-transport.md](apiv2-transport.md), [controller-transport.md](controller-transport.md). The `form.php` upstream-read rule, the pointer-field model, and the M-Z merge sit in controller-transport.md because they're forced by controller's form-edit semantics.
- **User-facing text is minimal; technical detail lives only in this SKILL.** Schema `Description`, README, `examples/*.tf` comments, `make generate-docs` output describe **what + how to use** — not implementation. Banned from user-facing surfaces: probe dates/filenames, upstream PHP refs, wire-shape language ("V2 echoes ~24 fields"), transport-name jargon ("Controller transport"), "server-managed" footnotes (just mark Computed), and rationale. All of that lives only here + the transport files + `docs/superpowers/specs/probe-*.md`. Go comments same diet — at most one line of *why-this-is-non-obvious* per declaration; cut multi-paragraph doc-comments. README "Resources" stays "example + brief attribute summary."

## The 7 phases

```
1. Grill           ──  grill-me skill, one question at a time, recommend + ask
2. Plan            ──  freeze decisions to docs/superpowers/plans/YYYY-MM-DD-*.md
3. Branch          ──  git checkout -b feat/<topic>
4. Probe + Spec    ──  curl probes against live ZenTao → docs/superpowers/specs/probe-*.md
5. Reconcile       ──  let the spec overturn earlier decisions; re-grill if needed
6. Implement       ──  zentaoAPI client (TDD) → resource + data source (parallel agents)
                       → provider.go reg → examples → README → make generate-docs
7. Verify + Commit ──  TF_ACC=1 acc tests + lint + multi-commit single PR
```

Phases are sequential. Probe results in phase 4 frequently invalidate phase 1 decisions — by design.

## Phase 1 — Grill

Trigger: user asks to add a resource / data source / API endpoint.

Invoke `grill-me` skill. Walk the decision tree branch by branch:
- **One question at a time.**
- **Always recommend with rationale before asking.**
- **Anchor recommendations** in existing patterns (e.g. "mirror `resource_product.go:93-99`'s Optional+Computed without static default").
- **Probe-deferred questions** are valid — mark "probe-verified later" and move on.

Decision tree typically covers, in order:
1. Read path strategy (single-GET? list-filter? Controller fallback?)
2. Parent / FK fields (Required vs Optional+Computed; TF name vs wire name)
3. Methodology / type / discriminator fields (enum, RequiresReplace?)
4. Required field set
5. ACL / role enums + lifecycle exposure
6. DELETE semantics (idempotent on missing? cascade?)
7. Branch name + probe execution mode (curl vs integration test)
8. PR shape (single vs split) and parallelization boundaries

If user corrects an assumption ("type is not the methodology, model is"), **save as project memory** before continuing.

## Phase 2 — Plan

Output: `docs/superpowers/plans/YYYY-MM-DD-<topic>.md`. Each decision gets: the decision, the rationale, the pattern it mirrors (file + line numbers).

The plan is a snapshot of intent, not a contract. Update in-place when reconciliation forces change.

## Phase 3 — Branch

```bash
git checkout -b feat/<topic>
```

Short resource-oriented topic (e.g. `feat/project-resource`). Repo's historical `zentao/task-NNNN` is overridden by the user's `feat/<topic>` preference. Verify clean working tree first; the plan doc is the only legitimate untracked file.

## Phase 4 — Probe + Spec

Highest-leverage phase. **Skipping ships bugs.**

### Probe execution

Probe URL forms and per-transport checklists live in the transport docs:
- V1 - [apiv1-transport.md](apiv1-transport.md)
- V2 — [apiv2-transport.md § Probe execution](apiv2-transport.md#probe-execution).
- Controller — [controller-transport.md § Probe execution](controller-transport.md#probe-execution); also reads `module/<entity>/config/form.php` upstream **first** (hard rule lives there).

Transport-agnostic probe heuristics:
1. Single GET exists? (Often undocumented but works.)
2. POST without optional fields — server defaults?
3. Each enum value — which pass server validation?
4. Mutation verb (V2 PUT / controller edit-POST) mutability of Required fields.
5. DELETE existing → DELETE again — missing-row response shape?
6. GET on missing/deleted — HTTP 404, HTTP 200 + envelope-fail, or `false` payload?
7. Required-but-undocumented fields — POST a minimum body, capture errors.
8. Wire field names vs TF-facing names. Document mapping.

### Spec format

Output: `docs/superpowers/specs/probe-<entity>-v2.md`. Sections:
- Endpoint summary (which work, which don't, gotchas).
- Required vs Optional fields on POST and PUT separately (PUT often more permissive).
- Response response shapes for success / fail / missing on each verb (verbatim JSON).
- Full GET-shape field set (truncate internals you don't surface; keep every exposed field).
- Reconciliation table mapping plan decisions to probe verdicts.
- Implementation notes for code (decoder peculiarities, defensive checks).

### Cleanup

Every probe-created row gets deleted. Verify:

```bash
curl -sS "$ZENTAO_URL/api.php/v2/<entity>" -H "Token: $SID" | \
  jq '[.[entity_list][] | select(.name | startswith("tfp-") or startswith("tf-probe-"))]'
```

Should return `[]`.

## Phase 5 — Reconcile

For each plan decision, check spec's reconciliation table:
- ✅ Confirmed — proceed.
- 🔄 Modified by probe — update plan doc.
- ➕ Net-new field — re-grill briefly if it affects schema visibility.

Probes commonly surface:
- Server-required fields not in V2 docs (e.g. `products`, `workflowGroup` → Required TF attrs).
- Validation envelope variants (`{"result":"fail"}` vs `{"status":"fail"}` — decoder must accept both).
- Stringified numerics (use `json.Number`).
- No HTTP 404 — DELETE/GET on missing returns `200 + {"status":"fail","message":"<Entity> does not exist."}`. Reuse `isNotFoundReason` from [zentaoAPI/errors.go](../../../zentaoAPI/errors.go).

## Phase 6 — Implement

### 6a. zentaoAPI client (TDD, sequential)

Files: `zentaoAPI/<entity>.go` + `_test.go`. Pick the template by transport — the transport choice falls out of the Phase 4 probe (does V2 cover the entity? does its GET echo every column you need?).

| Transport | When | Template |
|---|---|---|
| **V2 RESTful** | V2 covers GET/POST/PUT/DELETE and the GET echoes the full row | [apiv2-transport.md](apiv2-transport.md) — reference `zentaoAPI/product.go` |
| **Controller (PATH_INFO)** | V2 missing, truncated, or the entity needs PATH_INFO-only verbs (`undelete`, role binding) | [controller-transport.md](controller-transport.md) — reference `zentaoAPI/program.go` |
| **V1** | Endpoint genuinely lives only at `/api.php/v1/...` (rare on Max 8.x) | [apiv1-transport.md](apiv1-transport.md) |

The transport doc covers struct shape, path constants (or lack thereof), envelope decoding, minimum-test grid, and probe checklist. Run `go test -race` + `golangci-lint` after each test, regardless of transport.

### 6a-bis. Create with restore-on-soft-deleted

**Pattern.** ZenTao soft-deletes (sets `zt_project.deleted=1` / `zt_product.deleted=1`) instead of removing rows. When a `terraform destroy` → `terraform apply` cycle re-uses the same name and the user's stale state still carries the original id, a fresh `Create` POST collides on the unique key — or worse, succeeds with a different id while the original row stays as orphan trash.

**Resolution rule (project SOP, simplified form).** Every Create wrapper in `zentaoAPI/` MUST:

1. Accept caller-supplied `p.ID`. Zero means "no prior state, fresh create".
2. When `p.ID != 0`:
   - Fetch baseline via the **private unfiltered-lookup** variant (the one that does NOT collapse `deleted=1` into `ErrNotFound`; public `Get<Entity>` keeps that contract for Terraform-Read consumers). For controller-backed entities the convention is `get<Entity>Row` — see [controller-transport.md § Soft-delete baseline lookup](controller-transport.md#soft-delete-baseline-lookup).
   - On `ErrNotFound` → row is hard-gone; fall through to the fresh-create POST. Caller's id is ignored.
   - Otherwise → run the M-Z merge against baseline and issue **one** edit POST. The form **always carries `deleted=0`** (set unconditionally inside `<entity>ToForm`), so the edit POST is restorative on soft-deleted rows and a no-op on the deleted column for alive rows. Refetch via `Get<Entity>` and return.
3. When `p.ID == 0`, skip the lookup and run the create POST.

**Deliberate non-features.**

- **No name comparison.** The wrapper trusts that an id supplied in stale TF state belongs to the user — Terraform owns the state, not us. Adding a `name` cross-check is a layer-violation fix for a layer-3 (state-corruption) problem.
- **No alive-collision rejection.** A `p.ID != 0` create against an alive row is treated as a replace (M-Z merge applied verbatim). The user's prior message established this contract: "直接基于 Input replace + Set deleted = 0 即可."
- **No separate undelete route.** The restore is folded into the same edit POST via the `deleted=0` form key — one round trip, no `<entity>-undelete-{id}.json` call.

**Why ID-only and not name-based lookup.** Finding by name across both alive + soft-deleted rows needs a list endpoint that doesn't filter on deleted (the standard `<entity>-browse` does). That requires a separate probe + admin permission. The id-based path is sufficient for the common destroy/re-apply cycle and avoids cross-environment data-takeover hazards.

**Form acceptance assumption.** Whether ZenTao's `module/<entity>/config/form.php` actually accepts `deleted` as a writable column is **not yet probe-verified for `program` / `product`**. If the form silently ignores it, soft-deleted rows will go through the merge POST without flipping `deleted=0`, and the post-replace `Get<Entity>` refetch will return `ErrNotFound`. Integration tests (`TF_ACC=1`) are the verification gate; on failure, switch the implementation to the explicit two-step path (`<entity>-undelete-{id}.json` + `<entity>-edit-{id}` POST) and update this SOP.

**Test grid (mandatory).** Each Create wrapper test file MUST cover:

| `p.ID` | Baseline lookup | Expected outcome |
|---|---|---|
| `0` | not issued | normal create POST → returned id from envelope (covered by existing `BodyShape` test) |
| `!=0` | `ErrNotFound` | fresh create POST issued, caller's id ignored |
| `!=0` | row returned (`deleted=1` baseline stub) | one edit POST with merged input + `deleted=0`, then refetch; returned id preserved |
| `!=0` | row returned (`deleted=0` baseline stub) | same path; verifies the alive-row case is treated as replace, not error |

**Signature note.** Internally surfacing the deleted state via the unfiltered-lookup variant does NOT change the public `Get<Entity>` contract — keep it returning `ErrNotFound` on `deleted=1` so Terraform-Read still clears state cleanly. The unfiltered variant is private and used only by the Create restore path's baseline fetch. Concrete naming + signature for controller-backed entities: [controller-transport.md § Soft-delete baseline lookup](controller-transport.md#soft-delete-baseline-lookup).

### 6b. Terraform layer (parallel, post 6a)

Dispatch **two parallel agents**:
- **Agent A**: `zentao/resource_<entity>.go` + `_test.go` + edit Resources slice in `zentao/provider.go`.
- **Agent B**: `zentao/data_source_<entity>.go` + `_test.go` + edit DataSources slice in `zentao/provider.go`.

Both edit `provider.go` in different functions; `Edit` literal-match handles concurrent edits cleanly when each `old_string` is unique.

Each agent prompt MUST include:
- Spec doc path (Phase 4 output).
- Closest existing template (`resource_program.go` for projects; `resource_product.go` for Optional+Computed-without-default).
- Constraints (don't touch other agent's files; don't bump `go.mod`; don't commit).
- Schema table (TF name, type, R/O/C status, wire mapping if differs).
- Required CRUD method names from API client.
- Verification commands: `go build ./...`, `go test -race ./zentao/...`, `golangci-lint run ./zentao/...`.

If sub-agent reports failure, do NOT recover by rewriting — read the file, understand, re-dispatch with sharper guidance.

### 6b-bis. `UseStateForUnknown` is unsafe on server-derived Computed fields

**Hazard.** A Computed (or Optional+Computed) attribute whose value the server **recomputes from another input field on the same resource** must NOT carry `UseStateForUnknown`. The plan modifier pins old state into the planned value; Update returns the recomputed value; Terraform raises `Provider produced inconsistent result after apply: was cty.StringVal("OLD"), but now cty.StringVal("NEW")`.

Concrete case: [zentao/resource_product.go](../../../zentao/resource_product.go) `program_name`. Product's `program` is Optional+Computed FK; ZenTao joins on it and returns `programName`. When user changes `program` A→B: plan pins old `programName`; apply re-GET returns new `programName`; diff fails. Fix: drop the modifier; plan prints `(known after apply)` — correct UX.

**Triage rule.** For every Computed-only / Optional+Computed attribute: *"if the user changes one of THIS resource's other inputs, will Update make the server recompute this attribute?"*

- **Yes** (FK-join field, server-flipped flag tied to another input, derived label) → no `UseStateForUnknown`.
- **No, stable from create-time** (`code`, `created_by`, `created_date`, hard IDs) → keep it.
- **No, drifts from external sources** (workflow `status`, sibling-resource attachments mutating it, server-tick timestamps like `last_edited_date`) → keep it. Update on this resource doesn't recompute it; apply ≡ plan; only refresh shows benign drift.

**Don't conflate "server-derived" with "user-input-derived."** Program's `parent`/`program_path`/`grade` (set via `st-zentao_program_parent_attachment`) are server-derived but not derived from same-resource input; program's own Update never recomputes them, so `UseStateForUnknown` is safe (worst case: one-cycle refresh-drift). Only the same-resource-input → derived-field edge is dangerous.

**Symmetric: user-toggle fields that look derived.** Some Optional+Computed attrs are user-set switches whose default *appears* coupled to other inputs but isn't recomputed by Update once set (e.g. `multiple` on project — "enable iterations" toggle the user owns; server only defaults on create). These are user inputs, **not** derived. `UseStateForUnknown` is correct. Confirm with user/probe before declaring "derived."

**Variant: server-backfills-on-empty.** Some Optional+Computed user-input fields get a server-side default *only when the request body lacks them* (typical for role usernames — ZenTao backfills `po`/`qd`/`rd` with the requesting account; the wire struct's `omitempty` strips empty values from the body). `UseStateForUnknown` is wrong here in two ways: (1) if state somehow holds `""` it pins `""` into plan while Update's refetch returns `"admin"`; (2) when the user *removes* the attribute from a previously-set config, the framework can hand the next plan a non-Unknown PlanValue (`""` or null) — `UseStateForUnknown` only fires on Unknown, so the empty value flows through to apply and the server's "admin" backfill trips inconsistent-after-apply. Use [zentao/use_state_unless_empty.go](../../../zentao/use_state_unless_empty.go) `useStateUnlessEmpty()` — it gates on `ConfigValue` (unambiguous "did the user set this?"), pins state when non-empty, and explicitly sets PlanValue to Unknown otherwise. Concrete case shipped: product.po. Schema test must drive the modifier across the full grid of `(ConfigValue ∈ {null, set}) × (StateValue ∈ {empty, non-empty}) × (PlanValue ∈ {Unknown, null, ""})` to lock both the unknown-state and empty-PlanValue paths.

**Regression test pattern.**

```go
func TestXxxResource_DerivedField_NoUseStateForUnknown(t *testing.T) {
    r := NewXxxResource()
    var resp resource.SchemaResponse
    r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
    a := resp.Schema.Attributes["<derived_field>"].(schema.StringAttribute)
    if len(a.PlanModifiers) != 0 {
        t.Errorf("<derived_field> must have no plan modifiers — UseStateForUnknown causes inconsistent-after-apply when <input_field> changes")
    }
}
```

**Audit checklist.** For each new Computed attribute, write one row:

| Attribute | Type | Source of value | UseStateForUnknown? |
|---|---|---|---|
| `code` | Computed-only | Server-set at create, immutable | yes |
| `program_name` | Computed-only | Joined from `program` (Optional input) | **no** |
| `multiple` | Optional+Computed | User toggle, server default on create only | yes |
| `parent` | Computed-only | Set via sibling attachment resource | yes (drift, not inconsistency) |
| `last_edited_date` | Computed-only | Server timestamp on every edit | yes |
| `po` | Optional+Computed | User input; server backfills current account when omitted | **useStateUnlessEmpty** |

If "Source" references another input attribute on the same resource → flip to **no**.

### 6c. Examples + README + docs

1. Add `examples/resources/st-zentao_<entity>/resource.tf` and `examples/data-sources/st-zentao_<entity>/data-source.tf`. Mirror existing; **call out non-obvious server-required fields** in comments.
2. Update [README.md](../../../README.md): Status note, Resources subsection (schema sketch + "fields not in public docs" callout), Data Sources subsection (read-side caveats).
3. `make generate-docs` to refresh `docs/{resources,data-sources}/<entity>.md`.

## Phase 7 — Verify + Commit

### Verification suite (mandatory, in order)

```bash
go build ./...
go test -race ./...
golangci-lint run ./...
make generate-docs  # idempotent
direnv exec . bash -c 'TF_ACC=1 go test -race -timeout 120s \
  -run "TestAcc<Entity>Resource" ./zentao/...'
```

Acc-test failure means spec lied or resource misapplied it — debug **before** committing.

Verify no leftover rows:

```bash
direnv exec . bash -c 'curl -sS "$ZENTAO_URL/api.php/v2/<entity>" \
  -H "Token: $(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" \
    -H "Content-Type: application/json" \
    -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)" \
  | jq "[.<entity_list>[] | select(.name | startswith(\"acc-\") or startswith(\"imp-\") or startswith(\"dis-\"))]"'
```

Should return `[]`.

### Commit shape

Single PR, **four commits**:

1. `docs: probe-driven design contract for st-zentao_<entity>` — plan + spec.
2. `feat(zentaoAPI): <Entity> type + v2 RESTful CRUD` — API client + tests.
3. `feat(zentao): st-zentao_<entity> resource + data source` — TF layer + tests + provider.go reg.
4. `docs: <entity> examples + README section + regenerated tfplugindocs output`.

Multi-paragraph bodies describing **why** (decisions, probe findings), not just **what**.

### PR

```bash
git push -u origin feat/<topic>
```

Use `gh` if available. PR body must include: 3-bullet summary; spec + plan pointers; schema highlights (R / O+C / C-only); implementation notes from spec; test plan with reviewer-side TODO; out-of-scope list.

## Anti-patterns to refuse

- "Just write the code, figure out schema as we go." → Phase 1 grill **first**.
- "Docs say X, so X." → Probe first.
- "Handle missing-products in resource layer." → Probe surfaces required fields → Required TF attrs. Don't paper over server invariants.
- "Reuse `doRequest` for V1+V2+Controller." → Per CLAUDE.md, transports are split intentionally.
- "Probe controller directly to figure out fields." → Read `form.php` first ([controller-transport.md § Hard rule](controller-transport.md#hard-rule)). Probe second.
- "TF attribute `description` for clarity." → No. Wire `desc` → TF `desc`.
- "Commit while agents finish." → All green first.
- "Skip acc test, units are enough." → Acc proves wire shape matches reality.
- "Use product.go's wire struct shape for program / user / group." → No. Pick the template by transport — see §6a dispatcher.

## Quick reference

| Phase | Output | Tools |
|---|---|---|
| 1. Grill | Decision log | `grill-me` |
| 2. Plan | `docs/superpowers/plans/YYYY-MM-DD-<topic>.md` | `Write` |
| 3. Branch | `feat/<topic>` | `git checkout -b` |
| 4. Probe | curl exchanges + `docs/superpowers/specs/probe-<entity>-v2.md` | `direnv exec .` + `curl` + `jq` + `Write` |
| 5. Reconcile | Updated plan doc | `Edit` |
| 6a. API client | `zentaoAPI/<entity>.{go,_test.go}` | `Write` + `go test` + `golangci-lint` |
| 6b. TF layer | `zentao/{resource,data_source}_<entity>.{go,_test.go}` + `provider.go` | Two parallel `Agent` |
| 6c. Docs | examples + README + `make generate-docs` | `Write` + `Edit` + `Bash` |
| 7. Verify + commit | 4-commit PR on `feat/<topic>` | `go test -race`, `golangci-lint`, `TF_ACC=1`, `git commit`, `git push` |
