# Controller transport (PATH_INFO routes)

Reference: [zentaoAPI/program.go](../../../zentaoAPI/program.go) (canonical), [zentaoAPI/user.go](../../../zentaoAPI/user.go), [zentaoAPI/group.go](../../../zentaoAPI/group.go).

## When to use

Reach for controller when **any** of these is true:

- The entity isn't exposed on V2 (Max 8.x lacks V2 for `user`, `group`, and several `zt_project`-row sub-types).
- V2 GET echoes only a subset of the columns and you need the full row — controller's `<module>-edit-{id}` GET returns the complete `zt_*` row.
- Write semantics require an endpoint V2 doesn't surface (e.g. role binding routes).

If V2 covers the entity cleanly, prefer [apiv2-transport.md](apiv2-transport.md).

## Hard rule

**Read `module/<entity>/config/form.php` upstream before writing any controller wrapper.** Authoritative list of accepted form keys + `required` flags for PATH_INFO routes — much more reliable than guessing from controller PHP body or V2 docs. Reference: `https://github.com/easysoft/zentaopms/blob/main/module/<entity>/config/form.php`. Cross-check `module/<entity>/model.php` for extra required validators that contradict the form (model layer occasionally adds or drops requireds — e.g. `products` on project create is optional in form.php but enforced by model). Probe still wins; form.php is the cheapest first-pass.

## Surface conventions

- **Paths**: `<module>-<method>-<args>.json`. Dispatch via `c.doController(ctx, module, method, args, query, body)` for GET / JSON-body POST, or `c.doControllerForm(ctx, module, method, args, query, form)` for form-urlencoded POST. **No `<entity>sPath` constant** — controller paths are entirely module/method-driven; do not invent the V2-style path-constant pattern.
- **Auth carrier**: `?zentaosid=<sid>` query string. Token header alone yields 302 → `user-login`. The HTTP client's `CheckRedirect = http.ErrUseLastResponse` keeps the 302 visible to `isControllerSessionExpired`.
- **Envelopes**: two shapes coexist.
  - **`CtrlEnvelope`** (outer `status` / `data` wrapper, `data` is a JSON-string-encoded payload) — used by `<module>-edit-{id}` GET. Unwrap with `DecodeData(env, &inner)` into your `<entity>EditInner` struct.
  - **`CtrlSimpleResponse`** (`result` / `message`) — used by edit POST / create / delete. Classify success with `IsSuccess()`, errors with `classifyCtrlSimple(status, resp, body)`.
- **Error classification helpers** in [errors.go](../../../zentaoAPI/errors.go): `classifyCtrlError` (envelope-fail), `classifyCtrlSimple` (simple-fail), `isNotFoundReason`, `isLoginRedirectReason`. Reuse — don't open-code reason matching.
- **`CallController` is the escape hatch.** One-off endpoint with no need for a typed wrapper: call it directly. The moment you call the same endpoint twice, build a typed wrapper.

## Edit POST is not PATCH

`<module>-edit-{id}` POST resets any **omitted** `form.php` column to its form-default. Two patterns flow from this, both baked into the default struct shape below:

1. **M-Z merge** in `Update<Entity>` — fetch baseline, override only the fields the caller explicitly set (non-nil pointers), emit the full form. See [zentaoAPI/program.go](../../../zentaoAPI/program.go) `mergeProgramBaseline` and `programToForm`.
2. **Pointer fields on the public struct** so `nil` ("preserve baseline") and `""` / `0` ("set this column empty") are distinct wire intents.

## Struct shape: slim + pointer + UnmarshalJSON

This is the default for every controller wrapper. `program.go` is the canonical reference (since 2026-05-13).

### Slim struct — strict three classes

A controller wrapper's `<Entity>` struct carries **only** columns in one of these three classes:

1. Columns the Terraform resource / data source actually exposes.
2. Columns the wrapper's own behaviour needs (e.g. `Path` for a sibling mutator's client-side cycle check).
3. Columns `form.php` marks `required` but the wrapper doesn't expose — they must still ride the edit-POST form carrying their baseline value.

`zt_*` columns in none of those three classes **do not go in the struct**. Adding a field later is a deliberate act — the PR says which class it joins. The historical fat struct (every `zt_project` column with `json:"-"`) is an anti-pattern: it buries intent, bloats `UnmarshalJSON`, and makes "expose or internal?" a coin-flip for the next author.

### Pointer fields + UnmarshalJSON

- **Pointer fields** (`*string` / `*int64` / `*bool`) are the default. Controller's edit-POST always emits every writeable field (see "Edit POST is not PATCH"), so `nil` = "preserve baseline" and non-nil = "override, even with empty string". Value types collapse those two states and the user can never clear a column.
- **Decode via `UnmarshalJSON` on `*<Entity>`**, not a separate `<entity>CtrlWire` + `to<Entity>()`. `json.Number`-typed locals tolerate ZenTao's mixed number / quoted-number shapes, then take address into the pointer fields. Keep a wire struct only when it earns its place — field renames or transforms beyond `json.Number → int64 / bool`. Plain controller wrappers don't need one.

### Every layer of the pointer model

1. `<Entity>` struct fields → `*T`; `UnmarshalJSON` on `*<Entity>` decodes them.
2. `<entity>ToForm` derefs `nil` → `""` / `"0"` / `"false"`. File-local helpers:
   ```go
   func derefString(p *string) string { if p == nil { return "" }; return *p }
   func derefInt64(p *int64) int64    { if p == nil { return 0 };  return *p }
   ```
3. `merge<Entity>Baseline` predicates are `!= nil`. Test name: `Preserves...WhenInputNil`.
4. CRUD wrappers' id checks: `p.ID == nil || *p.ID == 0` for "missing"; `*p.ID` to use. Returned-id assignment: `out.ID = &id`.
5. **Resource layer** (`zentao/resource_<entity>.go`) — `toAPI()` maps `types.String.IsNull() || IsUnknown()` → `nil` via a helper. Required schema fields always produce non-nil (the framework rejects null/unknown for Required); Optional+Computed fields the user didn't set produce nil → the M-Z merge reads "preserve baseline":
   ```go
   func optString(v types.String) *string {
       if v.IsNull() || v.IsUnknown() {
           return nil
       }
       s := v.ValueString()
       return &s
   }
   ```
   For pointer-typed FK fields an attachment resource owns (e.g. `program.Parent`), omit them from `toAPI` entirely so the M-Z merge always preserves the column.
6. **Data source layer** — `<entity>FromAPI` and the data-source `Read` deref every field through `derefString` / `derefInt64`. Nil-safe: missing wire fields surface as `""` / `0` to Terraform state.
7. **Tests** — add `func sp(s string) *string`, `func ip(i int64) *int64`, `func bp(b bool) *bool` at the top of `<entity>_test.go`. Every `&<Entity>{Name: "x"}` literal becomes `&<Entity>{Name: sp("x")}`. `reflect.DeepEqual` `want` literals must include **every** field `UnmarshalJSON` populates from the fixture — else nil-vs-non-nil-pointer mismatches fail the comparison.
8. **JSON marshaling note** — `<Entity>` is never `json.Marshal`ed to the wire (writes go through `<entity>ToForm`), so omitting `MarshalJSON` is safe. If a future caller needs to marshal one, add an explicit `MarshalJSON` that strips server-managed fields rather than relying on `omitempty`.

## Sibling-relation mutators

A **sibling-relation mutator** points one entity's FK at another instance of the same (or a related) entity — `SetProgramParent` is the canonical case. ZenTao Max 8.x does **no** server-side validation here (probe finding F3, generalised): it silently accepts self-attach and multi-level ancestry cycles. The wrapper owns all guarding.

### Signature + delegation

- **Return `(*<Entity>, error)`.** The form-edit POST returns no row; the wrapper must refetch so the caller gets server-derived fields (`Path`, `Grade`, `LastEditedDate`). Matches `Create<Entity>` / `Update<Entity>`'s shape.
- **Delegate the write to `Update<Entity>`.** A sibling mutator is `[client validation] + Update<Entity>(&<Entity>{ID: &id, <FK>: &target})` — nothing more. It must NOT open-code its own edit-POST / decode / refetch: that path lives once, in `Update<Entity>`, and the M-Z merge preserves every other column. `SetProgramParent` is the reference.

### Validation order (cost-ordered)

1. **Zero-cost checks first** — positive child id, non-negative target id, self-attach (`child == target`). No network round-trip.
2. **Baseline-dependent checks second** — and only when reachable: skip the cycle check entirely when detaching (`target == 0`).

### Path-based ancestor-membership check (reusable primitive)

`zt_project.path` is a comma-bracketed ancestry list, e.g. `,1,5,20,`. To reject a cycle without recursive queries: fetch the prospective parent, and if `",<childID>,"` is a substring of `parent.Path`, the parent is already a descendant of the child — attaching forms a cycle. Reject with `ErrCycleDetected` (the shared, entity-agnostic sentinel in [errors.go](../../../zentaoAPI/errors.go)). Self-attach uses the same sentinel.

### Test grid (mandatory)

Because the write is delegated, **test only the increment the sibling mutator adds** — full-form preservation, POST encoding and refetch are `Update<Entity>`'s test surface.

| Scenario | Server | Assertion |
|---|---|---|
| cheap preflight (`child ≤ 0` / `target < 0` / self-attach) | none | table-driven; self-attach must `errors.Is(ErrCycleDetected)` |
| happy attach | mock | delegation reaches the child's edit route; returned `*<Entity>` carries the new `Path` |
| detach (`target == 0`) | mock | **zero** parent GET (baseline-dependent check short-circuited); POST carries `<FK>=0` |
| cycle via `Path` | mock | `errors.Is(ErrCycleDetected)` and **zero POST** (rejected before any mutation) |
| child / parent not found | mock | `ErrNotFound` propagates |

The negative assertions — zero POST, zero parent GET — are the easiest to omit and the most important to keep.

## Probe execution

```bash
direnv exec . bash -c '
SID=$(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" \
  -H "Content-Type: application/json" \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)

# GET (zentaosid in QUERY — Token header alone 302s)
curl -sS "$ZENTAO_URL/program-edit-7.json?zentaosid=$SID" | jq .

# POST form-urlencoded — JSON body to controller silently re-renders the form
curl -sS -X POST "$ZENTAO_URL/program-edit-7.json?zentaosid=$SID" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "name=test&begin=2026-01-01&end=2026-12-31"
'
```

Controller-specific probe checklist (additions to V2's):

9. `<module>-edit-{id}` GET — full row in `data` JSON-string? Does `data` need a second-pass JSON-unwrap?
10. POST with JSON body — actually mutates, or silently re-renders the form? (User-edit POST silently re-renders — use form-urlencoded.)
11. DELETE — 302 redirect, 200 + simple envelope, or HTTP 404? `CheckRedirect = http.ErrUseLastResponse` keeps the redirect visible to `isControllerSessionExpired`.
12. Wire-only fields that V2 doesn't surface — `path`, `grade`, `parent` ancestry list on `zt_project`, etc. Document them as Computed read-only on the TF side.
