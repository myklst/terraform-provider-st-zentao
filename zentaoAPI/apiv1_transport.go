package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// apiV1PathPrefix is the URL prefix for ZenTao's V1 REST API surface.
// Used only for documentation / sanity assertions.
const apiV1PathPrefix = "api.php/v1/"

// doV1Request invokes a ZenTao API V1 REST endpoint. V1 uses the same
// Token-header convention as V2 (the underlying sessionID drives both),
// but its expiry signal differs: per probe 2026-05-08, an unauthenticated
// V1 request returns HTTP 403 — not 401 — so isV1SessionExpired
// recognises both. The `zentaosid` query parameter is NOT injected on
// V1 routes (same as V2 — Token-header is the documented carrier).
//
// V1 has no production callers in the typed wrapper layer yet; this
// transport is plumbed for future wrappers (and as a documented escape
// hatch matching CallController on the Controller side).
func (c *Client) doV1Request(
	ctx context.Context,
	method, path string,
	query map[string]string,
	body any,
) ([]byte, int, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
	}
	return c.doWithRefresh(ctx, isV1SessionExpired, func(token string) ([]byte, int, string, error) {
		return c.sendHTTP(ctx, method, path, query, bodyBytes, "application/json", token, false)
	})
}

// isV1SessionExpired returns true iff the response indicates a V1
// session has gone away. V1 returns HTTP 403 on missing/invalid auth
// (probe 2026-05-08). 401 is also accepted defensively — REST
// convention prefers 401 for this state, and a future ZenTao version
// might align with the convention.
func isV1SessionExpired(status int, _ []byte, _ string) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}
