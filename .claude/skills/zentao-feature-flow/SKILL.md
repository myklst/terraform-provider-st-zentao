---
name: zentao-feature-flow
description: End-to-end workflow for adding a new entity (resource + data source) to terraform-provider-st-zentao. Covers grill-driven design, live-server probing, spec-first reconciliation, TDD implementation, parallel agent dispatch, and multi-commit PR shape. Use whenever the user asks to add a new ZenTao resource / data source, port a new V2 endpoint, or extend the API client.
---

# ZenTao Feature Flow

Add a new ZenTao entity (resource + data source) to this provider end-to-end. This skill is **the** way to ship a feature here. Skipping any phase costs more downstream.

## Hard rules (non-negotiable)

- **Never commit on `main`.** Cut a `feat/<topic>` branch first, even before any exploration touches the working tree.
- **Probe before assuming.** ZenTao's public V2 docs routinely omit server-required fields and lie about envelope shapes. If you guess instead of probing, you ship a broken resource.
- **Spec is the source of truth.** Code that contradicts the spec is wrong. If reality changes, update the spec **first**, then the code.
- **No premature design.** No `force_destroy` flags, no fallback transports, no abstraction layers "for later." Build what the current task requires.
- **TF attribute names mirror ZenTao wire names.** When a wire field is named `desc`, the Terraform schema attribute is `desc` too — do **not** rename it to `description`. Same goes for any other short ZenTao name (`PM`, `PO`, `QD`, `RD`, `acl`, `multiple`, etc.). Renaming forces the resource layer to translate between two vocabularies and obscures the spec ↔ code mapping. The only acceptable rename is when ZenTao itself uses two names for the same field (e.g. `parent` on the wire / `program` in the docs); in that case prefer the user-facing concept and document the wire mapping in the schema description.
- **Before adding a controller-transport wrapper, read `module/<entity>/config/form.php` upstream.** That file is the authoritative list of fields and their `required` flags for controller (PATH_INFO) routes — much more reliable than the V2 docs or the controller's PHP method body. Reference: `https://github.com/easysoft/zentaopms/blob/main/module/<entity>/config/form.php`. Probe still wins over docs, but `form.php` is the cheapest first-pass to enumerate the field set and required-vs-optional split before you write a single curl probe.
- **User-facing text is minimal; technical detail lives only in this SKILL.** Schema `Description` fields, the README, `examples/*.tf` comments, and `make generate-docs` output all describe **what the attribute is and how to use it** — not how it's implemented. Banned from user-facing surfaces: probe dates, probe filenames, upstream PHP file/line references, wire-shape language ("V2 echoes ~24 fields", "form.php declares X"), transport-name jargon ("Controller transport"), "server-managed" footnotes (just mark Computed), and the "why we chose this" rationale. All of that lives only in this SKILL plus the internal `docs/superpowers/specs/probe-*.md` archives. Go code comments follow the same diet — keep at most one line of *why-this-is-non-obvious* per declaration; cut multi-paragraph doc-comments. The README "Resources" sections stay at "example + brief attribute summary"; deeper context belongs here, not there.

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

Phases are sequential. Probe results in phase 4 frequently invalidate decisions from phase 1; that is by design — the alternative is shipping broken assumptions. Stay open to backtracking.

## Phase 1 — Grill

**Trigger**: user asks to add a resource / data source / API endpoint.

**Action**: invoke the `grill-me` skill. Walk the decision tree branch by branch:

- **One question at a time.** Bundling questions hides dependencies between answers.
- **Always recommend an answer with rationale** before asking — otherwise the user has to reverse-engineer your hesitation.
- **Anchor each recommendation** in an existing pattern (e.g. "mirror `resource_product.go:93-99`'s Optional+Computed without static default").
- **Probe-deferred questions** are valid — when a real answer requires hitting the server, mark the decision "probe-verified later" and move on.

Decision tree typically covers, in order:
1. Read path strategy (single-GET exists? list-filter? Controller fallback?)
2. Parent / FK fields (Required vs Optional+Computed; TF schema name vs wire field name)
3. Methodology / type / discriminator fields (enum, RequiresReplace?)
4. Required field set (`name + ...`)
5. ACL / role enums + lifecycle exposure
6. DELETE semantics (idempotent on missing? cascade?)
7. Branch name + probe execution mode (curl vs integration test)
8. PR shape (single vs split) and parallelization boundaries

If the user corrects an assumption you made (like "type is not the methodology, model is"), **save the correction as a project memory** before continuing. This prevents the same mistake on the next entity.

## Phase 2 — Plan

**Output**: `docs/superpowers/plans/YYYY-MM-DD-<topic>.md` capturing every grilled decision.

Each decision gets:
- The decision itself.
- The rationale (why this option over the alternatives).
- The pattern it mirrors (file + line numbers in the existing codebase).

The plan is a snapshot of intent, not a contract — the spec from Phase 4 may overturn it. Update the plan in-place when reconciliation forces a change.

## Phase 3 — Branch

```bash
git checkout -b feat/<topic>
```

Topic name should be short and resource-oriented (e.g. `feat/project-resource`). The repo's historical convention is `zentao/task-NNNN`, but the user's global feedback overrides that with `feat/<topic>`.

Verify clean working tree first; the plan doc you just wrote is the only legitimate untracked file at this point.

## Phase 4 — Probe + Spec

This is the highest-leverage phase. **Skipping it ships bugs.**

### Probe execution

Probes run via `curl` inside `direnv exec .` so the project's `.envrc` (with `ZENTAO_URL` / `ZENTAO_ACCOUNT` / `ZENTAO_PASSWORD`) is sourced:

```bash
direnv exec . bash -c '
SID=$(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" \
  -H "Content-Type: application/json" \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)

# probe individual endpoints with the sessionID
curl -sS "$ZENTAO_URL/api.php/v2/<endpoint>" -H "Token: $SID" | jq .
'
```

**Before any curl probe targeting a controller (PATH_INFO) route**, fetch `module/<entity>/config/form.php` from the upstream `easysoft/zentaopms` repo and skim the field list — it tells you which keys the controller actually accepts and which ones are flagged required. This catches keys that V2 docs omit (e.g. `multiple`, `workflowGroup`) and avoids wasted probes against fields that don't exist on the controller side. Also cross-check `module/<entity>/model.php` for any required-field validators that contradict the form definition (the model layer sometimes adds extra "required" guards beyond what the form declares — and sometimes drops them, like `products` on project create which `form.php` shows as optional).

The standard probe checklist for a new entity:
1. **Single GET exists?** (`GET /api.php/v2/<entity>/{id}` — often undocumented but works)
2. **POST without optional fields** — what does server default to?
3. **Each enum value** — which actually pass server validation?
4. **PUT mutability of Required fields** — does it actually mutate, or silently ignore?
5. **DELETE on existing → DELETE again** — what's the missing-row response?
6. **GET on missing/deleted** — HTTP 404 or HTTP 200 + envelope-fail?
7. **Required-but-undocumented fields** — try POST with the documented minimum; capture any "field X is required" errors.
8. **Wire field names** — TF schema may use `program`, but the wire might be `parent`. Document the mapping.

### Spec format

Output goes to `docs/superpowers/specs/probe-<entity>-v2.md`. Required sections:
- Endpoint summary (which work, which don't, gotchas).
- Required vs Optional fields on POST and PUT separately (PUT is often more permissive).
- Response envelope shapes for success / fail / missing on each verb (verbatim JSON).
- Full GET-shape field set as a single JSON block (truncate the long tail of internals you don't surface, but keep every field the resource exposes).
- Reconciliation table mapping `2026-MM-DD-<topic>.md §3` decisions to probe verdicts.
- Implementation notes that must flow into code (decoder peculiarities, defensive checks).

### Cleanup

Every probe-created row gets deleted. Verify before moving on:

```bash
curl -sS "$ZENTAO_URL/api.php/v2/<entity>" -H "Token: $SID" | \
  jq '[.[entity_list][] | select(.name | startswith("tfp-") or startswith("tf-probe-"))]'
```

Should return `[]`. Leftover probe rows pollute future tests and confuse the user.

## Phase 5 — Reconcile

For each decision in the plan, check the spec's reconciliation table:
- ✅ Confirmed — proceed as planned.
- 🔄 Modified by probe — update the plan doc with the new shape.
- ➕ Net-new field surfaced by probe — re-grill briefly with the user if the field affects schema visibility (Required / Optional / hidden).

Probes commonly surface:
- **Server-required fields not in V2 docs** (e.g. `products`, `workflowGroup` on project — both became Required TF attributes).
- **Validation envelope variants** (e.g. `{"result":"fail",...}` instead of `{"status":"fail",...}` — decoder must accept both).
- **Stringified numerics** (every numeric/FK column comes back as a JSON string; use `json.Number`).
- **No HTTP 404** — DELETE / GET on missing returns `200 + {"status":"fail","message":"<Entity> does not exist."}`. Reuse `isNotFoundReason` from [zentaoAPI/errors.go](../../../zentaoAPI/errors.go).

## Phase 6 — Implement

### 6a. zentaoAPI client (TDD, sequential)

Files: `zentaoAPI/<entity>.go` + `zentaoAPI/<entity>_test.go`.

Mirror [zentaoAPI/product.go](../../../zentaoAPI/product.go) exactly:
- Public struct with `json:` tags split into writeable (no `-`) vs server-managed (`json:"-"`).
- Wire struct (`<entity>V2Wire`) using `json.Number` for every numeric/FK column.
- `<entity>sPath` const built from `apiV2PathPrefix + "<entity>s"`.
- `<entity>Path(id int)` builder.
- `Get<Entity>` / `Create<Entity>` / `Update<Entity>` / `Delete<Entity>` mirroring product's structure.

Resource-specific deltas surfaced by probe go in this layer:
- Force-set `type:"<entity>"` on the wire if the table is shared (like `zt_project`).
- Return `ErrNotFound` on type-mismatch (defensive).
- Decode validation envelopes that use `result` instead of `status`.
- Splice non-echoed fields back from caller input (e.g. `products` not in V2 GET).

Tests cover at minimum:
- `Get<Entity>_FullFieldSet` — every surfaced field decoded correctly from a probe-shaped response.
- `Get<Entity>_NotFound_HTTP404` AND `Get<Entity>_NotFound_DoesNotExistMessage` — both shapes return `ErrNotFound`.
- `Get<Entity>_TypeMismatchIsErrNotFound` — when applicable.
- `Create<Entity>_BodyShape` — required fields present, server-managed fields stripped, `type` force-set.
- `Create<Entity>_FailEnvelope_StatusKey` AND `Create<Entity>_FailEnvelope_ResultKey` — both validation envelopes.
- `Update<Entity>_PutPathAndRefetch` — verifies the PUT-then-GET sequence.
- `Update<Entity>_NotFound_HTTP404` AND `Update<Entity>_NotFound_FailEnvelope`.
- `Delete<Entity>_Success`, `_HTTP404IsIdempotent`, `_NotExistMessageIsIdempotent`, `_OtherFailure`.

Run after each test addition:

```bash
go test -race ./zentaoAPI/...
golangci-lint run ./zentaoAPI/...
```

### 6b. Terraform layer (parallel, post 6a)

Once the API client is green, dispatch **two parallel agents** for non-overlapping work:

- **Agent A**: `zentao/resource_<entity>.go` + `zentao/resource_<entity>_test.go` + edit Resources slice in `zentao/provider.go`.
- **Agent B**: `zentao/data_source_<entity>.go` + `zentao/data_source_<entity>_test.go` + edit DataSources slice in `zentao/provider.go`.

Both agents edit `provider.go` but in different functions (Resources vs DataSources). The `Edit` tool's literal-match semantics handle the concurrent edits cleanly as long as each agent's `old_string` uniquely identifies its slice.

Each agent's prompt MUST include:
- The path to the spec doc (Phase 4 output).
- A pointer to the closest existing resource/data source as a template (e.g. `resource_program.go` for projects; `resource_product.go` for the Optional+Computed-without-default pattern).
- Explicit constraints (don't touch other agent's files; don't bump go.mod; don't commit).
- A schema table with TF attribute name, type, required/optional/computed status, and the wire-field mapping if it differs.
- The required CRUD method names from the API client (so the agent doesn't redesign them).
- Verification commands the agent must run before reporting success: `go build ./...`, `go test -race ./zentao/...`, `golangci-lint run ./zentao/...`.

If a sub-agent reports failure, do NOT recover by rewriting their output — read the failing file, understand why the spec was misapplied, and re-dispatch with sharper guidance.

### 6b-bis. `UseStateForUnknown` is unsafe on server-derived Computed fields

**The hazard.** A Computed (or Optional+Computed) attribute whose value the server **recomputes from another input field on the same resource** must NOT carry `UseStateForUnknown`. The plan modifier pins the prior state value into the planned value; Update then returns the freshly recomputed value; Terraform raises `Provider produced inconsistent result after apply: was cty.StringVal("OLD"), but now cty.StringVal("NEW")` and the apply hard-fails.

Concrete case shipped in this repo: [zentao/resource_product.go](../../../zentao/resource_product.go) `program_name`. The product's `program` is an Optional+Computed FK; ZenTao joins on it and returns `programName` in the GET response. When a user changes `program` from id A to id B:

1. Plan: `program = B` (user input). `program_name` Computed → `UseStateForUnknown` pins it to the old `programName` (e.g. `"PROGRAM_001"`).
2. Apply: PUT then re-GET returns `programName = "PROGRAM_001_01"`.
3. Terraform diff plan vs. apply → inconsistent-after-apply error.

Fix is a one-line schema edit: drop the plan modifier on that single attribute. Plan will now print `(known after apply)` for it whenever the upstream input is changing — that's the correct UX.

**Triage rule.** For every Computed-only and Optional+Computed attribute, ask: *"if the user changes one of THIS resource's other inputs, will Update make the server recompute this attribute?"*

- **Yes** (FK-join field, server-flipped flag tied to another input, derived label) → no `UseStateForUnknown`.
- **No, value is stable from create-time** (`code`, `created_by`, `created_date`, hard IDs) → keep `UseStateForUnknown`. Plan stability matters.
- **No, value drifts from external sources** (workflow `status`, foreign-resource attachments mutating it via a sibling resource, server-tick timestamps like `last_edited_date`) → keep `UseStateForUnknown`. Update on this resource doesn't recompute it, so apply ≡ plan; only refresh shows the drift, which is benign.

**Don't conflate "server-derived" with "user-input-derived."** A field driven by an external resource (e.g. program's `parent` / `program_path` / `grade`, all set via `st-zentao_program_parent_attachment`) is server-derived but **not** derived from a user input on the same resource. The program resource's own Update never recomputes `parent`/`program_path`/`grade`, so `UseStateForUnknown` on those is safe — the worst case is a one-cycle refresh-detected drift, never an inconsistent-after-apply. Only the same-resource-input → derived-field edge is dangerous.

**Symmetric rule for user-toggle fields that look derived.** Some attributes are user-set switches whose default *appears* coupled to other inputs but isn't actually recomputed by Update once set (e.g. `multiple` on project — a "enable iterations" switch the user owns; the server only defaults it on create). These are Optional+Computed user inputs, **not** derived fields. `UseStateForUnknown` is correct for them. Always confirm the read with the user / probe before declaring something "derived."

**Regression test pattern.** When you remove `UseStateForUnknown` for a derived field, lock the decision in with a schema test:

```go
// In zentao/provider_test.go (or per-resource _test.go).
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

The test is cheap, names the underlying invariant, and stops a future refactor from re-introducing the bug "to keep plan output tidy."

**Audit checklist when reviewing schema additions.** For each new Computed attribute, write one line:

| Attribute | Type | Source of value | UseStateForUnknown? |
|---|---|---|---|
| `code` | Computed-only | Server-set at create, immutable | yes |
| `program_name` | Computed-only | Joined from `program` (Optional input) | **no** |
| `multiple` | Optional+Computed | User toggle, server default on create only | yes |
| `parent` | Computed-only | Set via sibling attachment resource | yes (drift, not inconsistency) |
| `last_edited_date` | Computed-only | Server timestamp on every edit | yes |

If a row's "Source" column references another input attribute on the same resource — flip the modifier to **no**.

### 6c. Examples + README + docs

After both agents complete:

1. Add `examples/resources/st-zentao_<entity>/resource.tf` and `examples/data-sources/st-zentao_<entity>/data-source.tf`. Mirror existing examples, but **call out non-obvious server-required fields** in comments (e.g. probe-surfaced `products`, `workflow_group`).
2. Update [README.md](../../../README.md): Status note, Resources subsection (full schema sketch + the "fields not in public docs" callout), Data Sources subsection (with any read-side caveats like "products not echoed back").
3. Run `make generate-docs` to refresh `docs/{resources,data-sources}/<entity>.md`.

## Phase 7 — Verify + Commit

### Verification suite (mandatory, in this order)

```bash
go build ./...                             # compile every package
go test -race ./...                        # all unit tests, race-checked
golangci-lint run ./...                    # 0 issues required
make generate-docs                         # idempotent — should produce no diff if already run
direnv exec . bash -c 'TF_ACC=1 go test -race -timeout 120s \
  -run "TestAcc<Entity>Resource" ./zentao/...'  # live-server acc tests
```

If any acc test fails, debug **before** committing. Acc tests prove the wire shape matches reality; failure here means the spec lied or the resource misapplied it.

After acc tests, verify no leftover rows:

```bash
direnv exec . bash -c 'curl -sS "$ZENTAO_URL/api.php/v2/<entity>" \
  -H "Token: $(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" \
    -H "Content-Type: application/json" \
    -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)" \
  | jq "[.<entity_list>[] | select(.name | startswith(\"acc-\") or startswith(\"imp-\") or startswith(\"dis-\"))]"'
```

Should return `[]`.

### Commit shape

Single PR, **four commits** following the historical pattern:

1. `docs: probe-driven design contract for st-zentao_<entity>` — plan + spec.
2. `feat(zentaoAPI): <Entity> type + v2 RESTful CRUD` — API client + tests.
3. `feat(zentao): st-zentao_<entity> resource + data source` — TF layer + tests + provider.go reg.
4. `docs: <entity> examples + README section + regenerated tfplugindocs output` — examples + README + generated docs.

Each commit message has a multi-paragraph body describing **why** (decisions, probe findings) not just **what** (file list). Keep the historical commit style — it's verbose by design and reviewers rely on it.

### PR

```bash
git push -u origin feat/<topic>
```

If `gh` is available, open the PR via CLI; otherwise hand the GitHub URL to the user with a ready-to-paste title + body.

PR body must include:
- Summary (3 bullets max).
- Pointer to the spec + plan docs as the source of truth for design decisions.
- Schema highlights (Required / Optional+Computed / Computed-only buckets).
- Implementation notes flagged in the spec (e.g. "force-sets `type:` on wire").
- Test plan checklist with one item left for the reviewer ("Reviewer to verify against their own ZenTao instance with `ZENTAO_TEST_*` overrides").
- Out-of-scope list — explicit so a reviewer doesn't ask "why no `force_destroy`?"

## Anti-patterns to refuse

- "Let me just write the code, we'll figure out the schema as we go." → Phase 1 grill **first**.
- "The docs say X, so X." → Probe before trusting docs. The probe-project-v2.md spec exists because docs lied.
- "I'll handle the missing-products error in the resource layer." → No. Probe surfaces required fields; they become Required TF attributes. Don't paper over server invariants in the resource layer.
- "Let me reuse `doRequest` for V1+V2+Controller." → Per CLAUDE.md, transports are split intentionally. Adding a generic helper breaks that contract.
- "I'll just probe the controller directly to figure out the fields." → Read upstream `module/<entity>/config/form.php` first. Probe second. Skipping the form definition turns a 30-second lookup into an hour of curl bisection.
- "I'll call the TF schema attribute `description` for clarity." → No. The wire field is `desc`; the TF attribute is `desc`. Same vocabulary on both sides keeps the spec ↔ code mapping one-to-one.
- "I'll commit while the agents finish." → Verify all green, then commit. A failing test in a sub-agent's output is your problem to fix, not "their" problem.
- "I'll skip the acc test, the unit tests are enough." → Acc tests are the only thing that proves the wire shape matches reality. Skipping them is how schemas drift.

## Quick reference table

| Phase | Output | Tools |
|---|---|---|
| 1. Grill | Decision log in conversation | `grill-me` skill |
| 2. Plan | `docs/superpowers/plans/YYYY-MM-DD-<topic>.md` | `Write` |
| 3. Branch | `feat/<topic>` | `git checkout -b` |
| 4. Probe | curl exchanges + `docs/superpowers/specs/probe-<entity>-v2.md` | `direnv exec .` + `curl` + `jq` + `Write` |
| 5. Reconcile | Updated plan doc | `Edit` |
| 6a. API client | `zentaoAPI/<entity>.go` + `_test.go` | `Write` + `go test` + `golangci-lint` |
| 6b. TF layer | `zentao/{resource,data_source}_<entity>.{go,_test.go}` + `provider.go` | Two parallel `Agent` calls |
| 6c. Docs | examples + README + `make generate-docs` | `Write` + `Edit` + `Bash` |
| 7. Verify + commit | 4-commit PR on `feat/<topic>` | `go test -race`, `golangci-lint`, `TF_ACC=1`, `git commit`, `git push` |
