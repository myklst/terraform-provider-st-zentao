package zentaoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// loginAPIV1Wire is the response shape ZenTao's `POST /api.php/v1/tokens`
// returns. On success the body is `{"token":"<sid>"}`; on failure
// (observed: HTTP 400 + Chinese-localised "登录失败，请检查您的用户名
// 或密码是否填写正确。") the body collapses to `{"error":"<reason>"}`.
//
// Per probe 2026-05-08 (admin/123456 against lek-ws.sige.la:8080/zentao),
// the same `token` value also functions as the `zentaosid` for Controller
// routes and as the `Token:` header value for V1 and V2 REST endpoints —
// i.e. a single login produces a credential that drives all three
// transports. This replaces the older Controller-flavored two-step
// flow that the previous Login() implementation used.
type loginAPIV1Wire struct {
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
}

// Login authenticates via the ZenTao API V1 token endpoint:
//
//	POST <baseURL>/api.php/v1/tokens
//	Content-Type: application/json
//	{"account":"…","password":"…"}
//
// On success, the response carries a `token` that the client stores
// in c.token (under tokenMu) and uses as the credential for all three
// transports:
//   - Controller: passed via `?zentaosid=<token>` query parameter
//   - V1 / V2 REST: passed via `Token: <token>` request header
//
// The cookiejar attached to c.http also captures a `zentaosid` cookie
// from the response; the cookie is harmless but not relied upon —
// authentication on subsequent requests goes through the explicit
// query / header above, so the client behaves consistently whether
// or not the server happens to set the cookie.
//
// Failure classification:
//   - HTTP 401 → ErrUnauthorized
//   - any status with `{"error":"<reason>"}` body where reason looks
//     like a credential failure (per isUnauthorizedReason) → ErrUnauthorized
//   - any status with `{"error":"<reason>"}` body otherwise → *APIError
//   - empty token in 2xx response → "empty token" error (defensive,
//     not observed in the wild)
func (c *Client) Login(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api.php/v1/tokens"

	payload, err := json.Marshal(map[string]string{
		"account":  c.account,
		"password": c.password,
	})
	if err != nil {
		return fmt.Errorf("login: marshal credentials: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("login: read body: %w (http=%d)", err, resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("login: %w (http=401 body=%s)", ErrUnauthorized, string(body))
	}

	var wire loginAPIV1Wire
	parseErr := json.Unmarshal(body, &wire)

	// V1 returns 4xx + {"error":"..."} for credential failure and
	// generic validation errors. Classify against isUnauthorizedReason
	// so a "wrong password" / "登录失败" / "认证" reason becomes
	// ErrUnauthorized while other 4xx (server-side validation,
	// malformed body) surfaces as a structured *APIError.
	if parseErr == nil && wire.Error != "" {
		if isUnauthorizedReason(wire.Error) {
			return fmt.Errorf("login: %w (reason=%s)", ErrUnauthorized, wire.Error)
		}
		return &APIError{HTTPStatus: resp.StatusCode, Reason: wire.Error, RawBody: body}
	}

	if resp.StatusCode >= 400 {
		return &APIError{HTTPStatus: resp.StatusCode, RawBody: body}
	}

	if parseErr != nil {
		return fmt.Errorf("login: parse response: %w (body=%s)", parseErr, string(body))
	}

	if wire.Token == "" {
		return fmt.Errorf("login: empty token in response (body=%s)", string(body))
	}

	c.tokenMu.Lock()
	c.token = wire.Token
	c.tokenMu.Unlock()
	return nil
}

// isSessionExpired detects "session is gone" across transport flavours:
// V2 endpoints answer with HTTP 401, while Controller routes either
// redirect (302 → /user-login*) or carry a 200-OK envelope with reason
// "please login" / "请重新登录".
//
// (Commit #3 will split this into per-transport detectors as part of
// the apiv1/apiv2/controller transport file split. Until then it
// continues to serve doRequest and doControllerForm uniformly.)
func isSessionExpired(httpStatus int, body []byte, location string) bool {
	if httpStatus == http.StatusUnauthorized {
		return true
	}
	if httpStatus == http.StatusFound || httpStatus == http.StatusMovedPermanently || httpStatus == http.StatusSeeOther {
		if location != "" && strings.Contains(location, "user-login") {
			return true
		}
		return false
	}
	if httpStatus == http.StatusOK && len(body) > 0 {
		var env CtrlEnvelope
		if err := json.Unmarshal(body, &env); err == nil {
			if env.Status != "" && env.Status != "success" && isLoginRedirectReason(env.ZentaoFailReason()) {
				return true
			}
		}
	}
	return false
}

// refreshSession re-runs Login if the caller's observed token is still
// current. Concurrent callers serialize on refreshMu so only one Login
// is in flight at a time; later callers re-check the token under
// refreshMu and no-op if a peer already rotated it.
func (c *Client) refreshSession(ctx context.Context, observedToken string) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.tokenMu.Lock()
	current := c.token
	c.tokenMu.Unlock()
	if current != observedToken {
		return nil
	}
	return c.Login(ctx)
}
