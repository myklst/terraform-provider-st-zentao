# Controller transport (PATH_INFO routes)

Reference: [zentaoAPI/program.go](../../../zentaoAPI/program.go) (canonical), [zentaoAPI/user.go](../../../zentaoAPI/user.go), [zentaoAPI/group.go](../../../zentaoAPI/group.go).

## When to use

Reach for controller when **any** of these is true:

- The entity isn't exposed on V2 (Max 8.x lacks V2 for `user`, `group`, and several `zt_project`-row sub-types).
- V2 GET echoes only a subset of the columns and you need the full row — controller's `<module>-edit-{id}` GET returns the complete `zt_*` row.
- Write semantics require an endpoint V2 doesn't surface (e.g. `<module>-undelete-{id}`, role binding routes).

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

`<module>-edit-{id}` POST resets any **omitted** `form.php` column to its form-default. Two patterns flow from this:

1. **M-Z merge** in `Update<Entity>` — fetch baseline, override only the fields the caller explicitly set, emit the full form. See [zentaoAPI/program.go](../../../zentaoAPI/program.go) `mergeProgramBaseline` and `programToForm`.
2. **Pointer fields on the public struct** when the user has a legitimate intent to set a writeable field back to empty. See "Pointer fields" below.

## Soft-delete baseline lookup

The Create-restore SOP (SKILL.md §6a-bis) requires a `<entity>` lookup that does NOT collapse `deleted=1` into `ErrNotFound`. Convention for controller wrappers:

- **`get<Entity>Row(ctx, id) (*<Entity>, error)`** — the shared decode pipeline for `<module>-edit-{id}` GET. Returns the row as-is, including the `Deleted` field on the struct (or `(prog, deleted, error)` if the struct can't carry it). Used by both `Get<Entity>` and the Create restore baseline path.
- **`Get<Entity>(ctx, id) (*<Entity>, error)`** — thin wrapper around `get<Entity>Row` that returns `ErrNotFound` when the row is soft-deleted. This is the Terraform-Read contract.

Name the private one `get<Entity>Row` (the unfiltered shape IS the row), not `get<Entity>Any` — the latter suggests there's a `getAll` counterpart, which there isn't.

## Pointer fields on the API struct

Default: **value types** (`string` / `int64` / `bool`), as in the V2 reference [zentaoAPI/product.go](../../../zentaoAPI/product.go) — M-Z merge reads `""` / `0` as "preserve baseline". Cheap to write and test; all controller wrappers should start here.

**Switch to pointers** (`*string` / `*int64` / `*bool`) when **both** are true:

1. The form-edit POST forces always-emit-every-writeable-field semantics (typical for controller — see "Edit POST is not PATCH" above), AND
2. At least one writeable field has a legitimate user intent to set back to empty string (or 0) — e.g. `whitelist = ""` to clear the ACL list, which must reach the server instead of being read as "preserve".

When both apply, value-typed `""` collides with "preserve baseline" and the user can't clear the column. Pointers split the two states cleanly: `nil` = preserve, non-nil = override (even with empty string). Pointer-typed reference: [zentaoAPI/program.go](../../../zentaoAPI/program.go) (since 2026-05-13).

### When you pick pointers, every layer changes

1. `<Entity>` struct fields → `*T`. **Two acceptable shapes:**
   - **Wire struct + `to<Entity>()`** — keep a value-typed `<entity>CtrlWire` (`json.Number` for numerics) and convert via `func (w wire) to<Entity>() (*<Entity>, error)`.
   - **Custom `UnmarshalJSON` on `*<Entity>`** — collapse the wire struct away. `json.Number`-typed locals decode then take address into the pointer fields. Program uses this shape since 2026-05-13.

   Pick the second when the wire struct adds no value beyond decoding (no field renames, no transformations beyond `json.Number → int64 / bool`).
2. `<entity>ToForm` derefs `nil` → `""` / `"0"` / `"false"`. Add file-local helpers:
   ```go
   func derefString(p *string) string { if p == nil { return "" }; return *p }
   func derefInt64(p *int64) int64    { if p == nil { return 0 };  return *p }
   ```
3. `merge<Entity>Baseline` switches predicates from `!= ""` / `!= 0` to `!= nil`. Test name: `Preserves...WhenInputNil` (not `...WhenInputZero`).
4. CRUD wrappers' id checks: `p.ID == nil || *p.ID == 0` for "missing"; `*p.ID` to use. Returned-id assignment: `out.ID = &id`.
5. **Resource layer** (`zentao/resource_<entity>.go`) — `toAPI()` returns `&zentaoapi.<Entity>{ Name: optString(m.Name), ... }` via a helper that maps `types.String.IsNull() || IsUnknown()` → `nil`. Required schema fields always produce non-nil (the framework rejects null/unknown for Required); Optional+Computed fields the user didn't set produce nil, which the M-Z merge reads as "preserve baseline":
   ```go
   func optString(v types.String) *string {
       if v.IsNull() || v.IsUnknown() {
           return nil
       }
       s := v.ValueString()
       return &s
   }
   ```
   For pointer-typed FK fields the attachment resource owns (e.g. `program.Parent`), omit them from `toAPI` entirely so the M-Z merge always preserves the column.
6. **Data source layer** — `<entity>FromAPI` and the data-source `Read` deref every field through `derefString` / `derefInt64`. Nil-safe: missing wire fields surface as `""` / `0` to Terraform state.
7. **Tests** — add `func sp(s string) *string { return &s }`, `func ip(i int64) *int64 { return &i }`, `func bp(b bool) *bool { return &b }` at the top of `<entity>_test.go`. Every `&<Entity>{Name: "x"}` literal becomes `&<Entity>{Name: sp("x")}`. Every `out.ID != 42` assertion becomes `out.ID == nil || *out.ID != 42`. `reflect.DeepEqual` `want` literals must include **every** field `UnmarshalJSON` (or `to<Entity>()`) populates from the test fixture — else nil-vs-non-nil-pointer mismatches fail the comparison.
8. **JSON marshaling note** — `<Entity>` is never `json.Marshal`ed to the wire (writes go through `<entity>ToForm`), so omitting `MarshalJSON` is safe. If a future caller needs to marshal one, add an explicit `MarshalJSON` that strips server-managed fields rather than relying on `omitempty` semantics.

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
  -d "name=test&begin=2026-01-01&end=2026-12-31&deleted=0"
'
```

Controller-specific probe checklist (additions to V2's):

9. `<module>-edit-{id}` GET — full row in `data` JSON-string? Does `data` need a second-pass JSON-unwrap?
10. POST with JSON body — actually mutates, or silently re-renders the form? (User-edit POST silently re-renders — use form-urlencoded.)
11. DELETE — 302 redirect, 200 + simple envelope, or HTTP 404? `CheckRedirect = http.ErrUseLastResponse` keeps the redirect visible to `isControllerSessionExpired`.
12. Does `form.php` accept `deleted` as a writeable column? Required for the soft-delete restore SOP — see SKILL.md §6a-bis "Form acceptance assumption".
13. Wire-only fields that V2 doesn't surface — `path`, `grade`, `parent` ancestry list on `zt_project`, etc. Document them as Computed read-only on the TF side.
