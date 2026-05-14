# API V2 transport (RESTful)

Reference: [zentaoAPI/product.go](../../../zentaoAPI/product.go), [zentaoAPI/project.go](../../../zentaoAPI/project.go).

## When to use

The entity has a documented `/api.php/v2/<entity>` resource AND the verbs you need (GET/POST/PUT/DELETE) actually work on the target Max version. Probe-verify first — Max 8.x's V2 is incomplete for many `zt_project`-row sub-types (program, execution, sprint, …) and missing entirely for `user` / `group`. When V2 is missing or echoes a truncated row, fall through to [controller-transport.md](controller-transport.md).

## Surface conventions

- **Public struct** with `json:` tags split writeable (no `-`) vs server-managed (`json:"-"`).
- **Wire struct** `<entity>V2Wire` — `json.Number` for every numeric / FK column (V2 returns mixed string/number shapes; `json.Number` tolerates both). Convert to the public struct via `func (w wire) to<Entity>() (*<Entity>, error)`.
- **Path constants**: `<entity>sPath = apiV2PathPrefix + "<entity>s"`; `<entity>Path(id int)` concatenates `"/<id>"`. **Never hard-code** `"/api.php/v2/..."` literals.
- **CRUD**: `Get/Create/Update/Delete<Entity>` mirroring product. Update is **PUT** at `<entity>Path(id)` followed by a refetch.
- **Value-typed fields** are the default (`string` / `int64` / `bool`). M-Z merge (when needed) reads `""` / `0` as "preserve baseline". For when to switch to pointers, see [controller-transport.md](controller-transport.md) — the pointer model is forced by controller's form-edit semantics, V2 PUT doesn't suffer from it.

## Probe-surfaced deltas

- Force-set `type:"<entity>"` on shared tables (`zt_project`).
- Return `ErrNotFound` on type-mismatch (defensive — V2 won't filter for you).
- Decode `result`-keyed validation envelopes (some V2 routes return `{"result":"fail"}` instead of `{"status":"fail"}`).
- Splice non-echoed fields from caller input (e.g. `products` not in V2 GET → keep the caller's request value on the returned struct).

## Minimum tests

- `Get_FullFieldSet` — every surfaced field decoded from probe-shaped response.
- `Get_NotFound_HTTP404` AND `Get_NotFound_DoesNotExistMessage`.
- `Get_TypeMismatchIsErrNotFound` — when applicable.
- `Create_BodyShape` — required present, server-managed stripped, `type` force-set.
- `Create_FailEnvelope_StatusKey` AND `Create_FailEnvelope_ResultKey`.
- `Update_PutPathAndRefetch`.
- `Update_NotFound_HTTP404` AND `Update_NotFound_FailEnvelope`.
- `Delete_Success`, `_HTTP404IsIdempotent`, `_NotExistMessageIsIdempotent`, `_OtherFailure`.

After each test:
```bash
go test -race ./zentaoAPI/...
golangci-lint run ./zentaoAPI/...
```

## Probe execution

```bash
direnv exec . bash -c '
SID=$(curl -sS -X POST "$ZENTAO_URL/api.php/v1/tokens" \
  -H "Content-Type: application/json" \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" | jq -r .token)

curl -sS "$ZENTAO_URL/api.php/v2/<endpoint>" -H "Token: $SID" | jq .
'
```

Probe checklist (V2-flavoured; controller has its own additions):

1. Single GET exists? (`GET /api.php/v2/<entity>/{id}` — often undocumented but works.)
2. POST without optional fields — server defaults?
3. Each enum value — which pass server validation?
4. PUT mutability of Required fields — actually mutates, or silently ignored?
5. DELETE existing → DELETE again — missing-row response shape?
6. GET on missing/deleted — HTTP 404 or HTTP 200 + envelope-fail?
7. Required-but-undocumented fields — POST a minimum body, capture validation errors.
8. Wire field names — TF `program` may map to wire `parent`. Document mapping.

**V2 must not receive `?zentaosid=` query** — Max 8.x mis-parses it on PUT as a record id and yields `Unknown column` SQL errors. `sendHTTP`'s `injectZentaosid` flag guards this; don't bypass it.
