package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

//nolint:all
// apiV2PathPrefix is the URL prefix for ZenTao's V2 REST API surface.
// Used only for documentation / sanity assertions; doV2Request does
// not validate the caller's path against this prefix because some
// callers pass paths starting with `/` and others without.
const apiV2PathPrefix = "api.php/v2/"

// doV2Request invokes a ZenTao API V2 REST endpoint. V2 authenticates
// via the `Token: <sessionID>` header; the `zentaosid` query parameter
// MUST NOT be set on V2 writes (Max 8.x mis-parses it as a record id
// on PUT, yielding "Unknown column" SQL errors on unique-check).
//
// Auth, session refresh, retry on 5xx, and cookie persistence are
// inherited from the shared sendHTTP + doWithRefresh helpers in
// client.go. Per-transport expiry detection lives in
// isV2SessionExpired below: V2 signals an expired session strictly
// via HTTP 401 (no body-level "please login" envelope on V2 routes).
func (c *Client) doV2Request(
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
	return c.doWithRefresh(ctx, isV2SessionExpired, func(token string) ([]byte, int, string, error) {
		return c.sendHTTP(ctx, method, path, query, bodyBytes, "application/json", token, false)
	})
}

// isV2SessionExpired returns true iff the response indicates a V2
// session has gone away. V2's only documented expiry signal is HTTP
// 401; the body and Location header are unused on this transport.
func isV2SessionExpired(status int, _ []byte, _ string) bool {
	return status == http.StatusUnauthorized
}
