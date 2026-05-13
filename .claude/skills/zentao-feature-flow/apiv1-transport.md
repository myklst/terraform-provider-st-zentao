# API V1 transport

Currently used only for `POST /api.php/v1/tokens` (the login round-trip in [zentaoAPI/auth.go](../../../zentaoAPI/auth.go) — `Login` + `refreshSession`). No entity wrappers are built on V1.

## When to add a V1 wrapper

Only when an endpoint genuinely lives at `/api.php/v1/<route>` AND has no V2 or controller equivalent. Rare on Max 8.x — most endpoints upstream docs label "V1" are also reachable via V2 or controller, where they're better-behaved.

If you do need one:
- Path: compose from `apiV1PathPrefix` (defined in [apiv1_transport.go](../../../zentaoAPI/apiv1_transport.go)) — never hard-code `"/api.php/v1/..."`.
- Auth carrier: `Token: <sid>` header.
- Body: JSON.
- Expiry signal: HTTP 401 or 403.
- Dispatch: `doV1Request` → `doWithRefresh` with `isV1SessionExpired`.

Probe-verify the endpoint is actually V1-only before adding the wrapper.
