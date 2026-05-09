# Probe: Does ZenTao API V2 Cover All Controller Methods?

**Date:** 2026-05-09
**Probe target:** `http://lek-ws.sige.la:8080/zentao/` (Max 8.1)
**Probe operator:** Claude (curl/python automated sample probe)
**Verdict:** **No — but the answer is more nuanced than "no":** V2 has only **94 explicit routes** in `config/apiv2.php`, yet the V2 router silently **falls back to dispatching almost any controller method** when the path doesn't match. The fallback "works" in the wire-routing sense for ~73% of read-leaning controller methods, but the response is only a usable JSON envelope for a fraction of those, and the fallback also dispatches **write methods on GET** with side effects (see ⚠️ below).

> Source-of-truth refs: `config/apiv2.php` (the explicit table), `framework/api/router.class.php#routeV2()` (the fallback dispatch logic), `module/<x>/control.php` (the methods being reached).

---

## TL;DR

| Question | Answer |
|---|---|
| Does V2 expose every controller method as a documented REST route? | **No.** Only 94 routes across 25 resources are listed in `config/apiv2.php`. |
| Does the V2 router *physically* dispatch arbitrary `/api.php/v2/<module>/<method>` paths? | **Yes.** Any non-numeric second path segment is taken as a method name and dispatched via the same MVC code path the Controller transport uses. |
| Does the dispatched method return a JSON envelope a client can parse? | **Sometimes.** Of 158 read-safe (module, method) probes, 27% returned no JSON at all (HTML pages, fatal errors, login redirects, empty bodies). |
| Is V2 fallback safe to use as a generic transport? | **No.** It bypasses the explicit route allowlist, sends `Content-Type` headers that don't match the actual body, and dispatches write methods (e.g. `release/changeStatus`) on a bare `GET`. |

**Recommendation:** Continue the existing pattern — typed wrappers per entity, V2 only when the entity is in `config/apiv2.php`'s explicit table, Controller transport (with cookie sessionID) for everything else. **Do not** add a generic V2 fallback wrapper.

---

## How V2 actually routes (read the source, not the URL shape)

`framework/api/router.class.php:300` — the `routeV2()` body, paraphrased:

```php
$this->action = strtolower($_SERVER['REQUEST_METHOD']);

if ($this->action == 'get') $methodName = $this->parseRouteV2($routes);   // try apiv2.php first

$pathItems  = explode('/', trim($this->path, '/'));
$moduleName = $this->singular($pathItems[0]);   // products → product

$actionToMethod = ['get'=>'browse', 'post'=>'create', 'put'=>'edit', 'delete'=>'delete'];

if (isset($pathItems[1])) {
    if (is_numeric($pathItems[1])) {
        if ($this->action == 'get') $methodName = 'view';        // /v2/products/123 → product->view
        else $_GET[$moduleName.'ID'] = $pathItems[1];           // PUT/DELETE /v2/products/123 → product->edit/delete
    } else {
        $methodName = $pathItems[1];                              // /v2/user/suspend → user->suspend !
    }
}
if (isset($pathItems[2])) $methodName = $pathItems[2];
if (!$methodName) $methodName = $actionToMethod[$this->action];

$this->setModuleName($moduleName);
$this->setMethodName($methodName);
$this->setControlFile();    // loads module/<x>/control.php and invokes <method>()
```

Key consequences:

1. **`apiv2.php` is the allowlist for `GET`-only documented routes** — and only 94 of them exist. They are mostly parameterised paths (`/programs/:programID`, `/projects/:projectID/builds`), with only 28 fully static.
2. **Method-name in the URL bypasses the allowlist.** `/api.php/v2/<module>/<methodName>` reaches `<module>` controller's `<methodName>()` directly, with no allowlist check. The HTTP verb is recorded as `$action` but the method is invoked unconditionally — so a `GET` can trigger a write method.
3. **Numeric path segment short-circuits to `view`/`edit`/`delete`** based on verb — that's the documented RESTful behavior.
4. **Permission/visibility is delegated to the controller method** (via `$this->checkObjectPriv()` etc.), not to the router. If the method itself doesn't gate output, V2 inherits whatever the controller does.

There is no `api/v2/entries/<x>.php` directory analogous to `api/v1/entries/`. V2 is not a curated entry-class API like V1 — it is a thin adapter on top of the same MVC controllers used by the web UI.

---

## Live-server probe: 158 (module, method) pairs

**Methodology:**

- Source: `easysoft/zentaopms/main` tarball, parsed `module/*/control.php` for `public function` methods → 1625 distinct methods across 98 modules.
- Sample: 1–2 read-leaning methods per module, filtered by safe prefix (`browse`, `view`, `get*`, `list*`, `index`, `show`, `print`, `export`, `count`, `search`, `ajaxGet*`, `fetch`, `load`, `preview`, `detail`, `tree`) **and** explicit denylist of write substrings (`delete`, `close`, `start`, `cancel`, `finish`, `activate`, `confirm`, `suspend`, `edit`, `create`, `batch`, `link`, `save`, `change`, `set`, `update`, `remove`, `assign`, `submit`, `reject`, `approve`, …).
- Excluded 11 modules with no read-safe method: `ai, block, cache, ci, datatable, dataview, dev, misc, personnel, score, setting`.
- Each probe: `GET /api.php/v2/<module>/<method>` with `Token: <v1-token>` header, no body.

**Result distribution (n=158):**

| Outcome | Count | % | Meaning |
|---|---:|---:|---|
| `JSON_RAW` (typed envelope, no `status` field) | 59 | 37.3% | V2 dispatched, controller returned a JSON object/array (e.g. `{"groups":[...]}`) |
| `JSON_BUSINESS_FAIL` (`{"status":"fail"}`) | 43 | 27.2% | V2 dispatched, controller ran, business validation rejected (typically "X does not exist" because no ID given) |
| `HTML_PAGE` | 19 | 12.0% | Controller rendered the full HTML page (login redirect or web view) — auth gate failed or the method only does `display()` |
| `JSON_SUCCESS` (`{"status":"success"}`) | 13 | 8.2% | V2 dispatched and returned the standard success envelope |
| `HTML_FATAL_ERROR` (xdebug stack) | 9 | 5.7% | Controller fatally errored (header sent, missing dependency, `setControlFile` failure, etc.) |
| `EMPTY_RESPONSE` | 8 | 5.1% | Controller returned 200 with empty body |
| `HTML_PARTIAL_FRAGMENT` | 2 | 1.3% | Controller returned a UI partial (e.g. `<select>` for an AJAX dropdown) |
| Misc (XML, plain text, `null`) | 5 | 3.2% | One-offs |

**Reaches-controller rate = JSON_RAW + JSON_BUSINESS_FAIL + JSON_SUCCESS = 115/158 = 72.8%.**

**Source breakdown of the JSON-reaching probes:**

- 64/72 successful JSON responses come from **pure fallback dispatch** (path NOT in `apiv2.php`).
- 8/72 hit a module that *has* an apiv2 entry but for a method not listed there.
- 0/72 matched a fully static apiv2 path — because the apiv2 table is mostly parameterised (`/projects/:projectID`).

So in this sample, **none** of the JSON wins came from the documented route table. The fallback is doing all the work.

---

## Module-level coverage buckets

### A. In `apiv2.php` AND fallback returns parseable JSON for our sample (8)
`bug`, `dept`, `product`, `program`, `project`, `task`, `testcase`, `testtask`

### B. NOT in `apiv2.php` but fallback returns parseable JSON (48)
`action`, `admin`, `backup`, `bi`, `company`, `compile`, `convert`, `cron`, `custom`, `design`, `dimension`, `doc`, `extension`, `gitea`, `gitlab`, `group`, `holiday`, `host`, `index`, `instance`, `job`, `kanban`, `mail`, `message`, `metric`, `my`, `pivot`, `programplan`, `projectbuild`, `projectrelease`, `qa`, `screen`, `search`, `serverroom`, `sonarqube`, `space`, `stage`, `stakeholder`, `store`, `testsuite`, `transfer`, `tree`, `tutorial`, `upgrade`, `webhook`, `weekly`, `zahost`, `zanode`

### C. Fallback reaches the controller but returns only `JSON_BUSINESS_FAIL` (17)
`aiapp`, `branch`, `build`, `chart`, `epic`, `execution`, `productplan`, `projectplan`, `projectstory`, `release`, `requirement`, `story`, `system`, `testreport`, `todo`, `user`, `zai`

These methods need real IDs / params before they'll succeed — but the dispatch itself worked.

### D. Fallback fails outright (HTML page / fatal / login redirect / empty / xml) (14)
`api`, `caselib`, `editor`, `entry`, `file`, `git`, `gogs`, `install`, `jenkins`, `mr`, `repo`, `report`, `sso`, `svn`

For these modules V2 fallback either renders a full HTML page (auth-gate failure), throws a PHP fatal (missing dependency / can't `setControlFile`), or returns empty.

> Module `projectGroup` was not in this sample (no `control.php` matches the snake-case filename — the actual file is `module/group/control.php` with project-scoped methods on the `group` module). The earlier `probe-group-controller.md` already demonstrated that V2 fallback for project-group-style methods responds `the control file ... not found` because of case-sensitive `setControlFile` resolution on Linux.

The full per-(module, method) table is in `/tmp/zt-probe/per_module_table.md` from the probe run.

---

## ⚠️ Safety: V2 fallback is dangerous as a generic surface

A pilot probe of `GET /api.php/v2/release/changeStatus` (no body, no params) returned:

```json
{"load":true,"status":"success","message":"保存成功"}
```

`changeStatus` is a write method on the `release` module. The V2 router invoked it via the GET fallback. Verification afterwards (`GET /api.php/v2/products/1/releases`) showed the single existing release was unchanged — but the controller did execute its full code path including the `Saved` branch, which means:

- V2 fallback dispatch **does not enforce HTTP-verb safety**: a GET can run code intended only for POST/PUT.
- Controller methods that don't do strict `if ($_POST)` gating will run on bare GETs.
- The "no data changed" outcome here is a happy coincidence of `changeStatus` requiring a `$_POST['status']` value that wasn't there — it ran the validation branch and returned the success envelope, but the actual UPDATE was skipped.

**You cannot assume `GET /api.php/v2/<x>/<y>` is read-only.** If `<y>` is a state-mutating method, V2 will dispatch it.

This is a known-weak property of MVC frameworks reused as APIs. ZenTao's V2 inherits the entire web controller surface, so any web-endpoint quirk (CSRF-via-GET, idempotency assumptions, partial form processing) becomes an API quirk.

---

## Other notable behaviours observed

- **Wrong `Content-Type`:** Many `ajaxGet*` methods echo JSON via `echo json_encode(...)` but the framework defaults the response header to `text/html; charset=UTF-8`. Examples: `backup/ajaxGetDiskSpace`, `bi/ajaxGetTableFieldsMenu`, `task/ajaxGetUserTasks`, `space/getStoreAppInfo`. A V2 client cannot trust `Content-Type` alone — it must sniff the body.
- **`HTML_LOGIN_REDIRECT` is silent:** When a controller's auth gate redirects to `/user-login`, the V2 router still returns 200 and `text/html`, with the full login HTML as the body. There is no `WWW-Authenticate`, no 401, no `Location` header. This is the same problem the **Controller transport** has (see `probe-controller-auth.md` "session-expired detection") — the only signal is "body looks like a login page".
- **Case-sensitive control file resolution:** `module/projectgroup/control.php` doesn't exist — the actual file is `module/group/control.php`, which means V2 paths like `/api.php/v2/projectgroup/...` always 200 with an HTML stack trace. Any V2 wrapper for project-group-scoped operations must use `/api.php/v2/group/...`. Confirmed against `gogs`, `jenkins` (different cause: control file isn't auto-loaded for these integrations).
- **`api/index` and `api/view` xdebug-fatal:** dispatching the `api` module reflectively triggers a recursive `setControlFile`/`include` failure. The `api` module exists but is the framework's own bootstrap, not a domain controller — V2 cannot reach it.

---

## Implications for `terraform-provider-st-zentao`

1. **Keep the per-transport split** described in `CLAUDE.md`. A "generic V2" wrapper is a footgun: 27% of methods produce non-JSON, response-shape varies by method, and the safety property (no GET-side-effects) does not hold.
2. **Continue using V2 only for entities in `config/apiv2.php`** (the bucket A modules, plus param-route resources `/programs/:id`, `/products/:id`, `/projects/:id`, `/executions/:id`, `/builds/:id`, `/users/login`, etc.). For these the response shape is documented in the route's `'response' => ...` field and the body is real JSON.
3. **For everything else, prefer the Controller transport** with cookie + `?zentaosid=` query (current pattern in `controller_transport.go`). Even though the V2 fallback would *also* reach those methods, Controller has:
   - explicit module/method/args path style
   - well-known envelope shape (`CtrlEnvelope`, `CtrlSimpleResponse`)
   - a single, documented expiry signal (302 → `user-login*` OR `please login` reason)
   - no risk of accidentally invoking via the wrong HTTP verb
4. **Project-group resource (in flight): stay on Controller.** The fallback would return `the control file not found` HTML for `/api.php/v2/projectgroup/...`. This matches the rationale already captured in `2026-05-09-zentao-project-group-resource.md`.
5. **If a future entity *only* exists in V2 fallback (bucket B):** wrap it as a typed V2 call but **explicitly** treat the response as `JSON_RAW` (no `status` field), and *do not* assume `Content-Type: application/json`. Sniff for a leading `{` or `[` before `json.Decode`.

---

## Reproducing this probe

```bash
# Source the live-server credentials
set -a && source .envrc && set +a

# 1. Get the controller methods table
curl -sL https://github.com/easysoft/zentaopms/archive/refs/heads/main.tar.gz \
  | tar -xz --wildcards 'zentaopms-main/module/*/control.php' 'zentaopms-main/config/apiv2.php' \
            'zentaopms-main/framework/api/router.class.php' -C /tmp/

# 2. Login once
TOKEN=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d "{\"account\":\"$ZENTAO_ACCOUNT\",\"password\":\"$ZENTAO_PASSWORD\"}" \
  "$ZENTAO_URL/api.php/v1/tokens" | jq -r .token)

# 3. Probe a single (module, method) pair
curl -s -H "Token: $TOKEN" "$ZENTAO_URL/api.php/v2/group/" | head -c 200

# 4. Full read-safe sweep — see python script in /tmp/zt-probe/ from the 2026-05-09 session
```

The intermediate artefacts from this probe (the 1625-method table, the V2 route table, the 158-row results JSON, the per-module markdown table) live in `/tmp/zt-probe/` and are not committed.
