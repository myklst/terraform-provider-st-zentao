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

// auth.go owns the credential lifecycle for *Client: the V1 login
// round-trip and the lock-protected refresh path. *Client setup,
// transport plumbing (sendHTTP, doWithRefresh), and per-transport
// helpers live in their own files; this file is intentionally
// transport-agnostic — the sessionID it produces drives V1, V2 and
// Controller routes alike (see probe-controller-auth.md addendum).

// loginAPIV1Wire is the response shape ZenTao's `POST /api.php/v1/tokens`
// returns. On success the body is `{"token":"<sid>"}`; on failure
// (observed: HTTP 400 + "登录失败，请检查您的用户名或密码是否填写正确。")
// the body collapses to `{"error":"<reason>"}`.
//
// Per probe 2026-05-08, the same `token` value drives all three transports —
// Controller via `?zentaosid=<token>`; V1 and V2 REST via `Token: <token>`
// header — so this single login replaces the older Controller-flavored
// two-step flow that earlier code used.
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
// On success, the returned token is stored under tokenMu and used as the
// credential for every transport. Login bypasses doWithRefresh (it must
// not recurse into refreshSession) and reuses c.http only for cookie jar
// continuity with the rest of the client.
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
	// generic validation errors. Classify via isUnauthorizedReason so
	// "wrong password" / "登录失败" / "认证" become ErrUnauthorized
	// while other 4xx surfaces as a structured *APIError.
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
