package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// loginV1Wire is the response shape ZenTao's `/user-login.json` returns
// (the v1 apilogin route). On success it carries a `token` value that
// also functions as the session ID; on failure the same envelope-level
// fields used elsewhere (status / error / message / reason) describe
// the cause. The `user` field is intentionally captured as raw bytes —
// we don't need its contents for auth and parsing it would couple us
// to ZenTao's user schema.
type loginV1Wire struct {
	Status  string          `json:"status"`
	Token   string          `json:"token"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	User    json.RawMessage `json:"user,omitempty"`
}

func (w loginV1Wire) failReason() string {
	return zentaoFailReason(w.Error, w.Message, w.Reason)
}

// getsessionidWire is the outer envelope of `/api-getsessionid.json`.
// The inner `data` is itself a JSON-encoded string carrying the
// `sessionID` we need for the apilogin step — see DecodeData semantics
// in types.go.
type getsessionidWire struct {
	Status  string          `json:"status"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type getsessionidInner struct {
	SessionName string `json:"sessionName"`
	SessionID   string `json:"sessionID"`
}

// Login performs the v1 two-step apilogin flow:
//  1. GET /api-getsessionid.json — bootstraps a PHP session, returns its
//     sessionID inside a string-encoded `data`. The cookiejar attached
//     to c.http captures the matching `zentaosid` cookie automatically.
//  2. GET /user-login.json?account=&password=&zentaosid=<sid> — verifies
//     credentials. On success the response repeats the (possibly
//     rotated) sessionID under `token`, and the cookiejar updates.
//
// The probe (docs/superpowers/specs/probe-controller-auth.md) showed
// the v1 sessionID drives BOTH PATH_INFO Controller calls AND the v2
// REST endpoints, so this single login replaces the prior
// `/api.php/v2/users/login` flow entirely.
func (c *Client) Login(ctx context.Context) error {
	sid, err := c.loginStep1(ctx)
	if err != nil {
		return err
	}
	token, err := c.loginStep2(ctx, sid)
	if err != nil {
		return err
	}
	c.tokenMu.Lock()
	c.token = token
	c.tokenMu.Unlock()
	return nil
}

func (c *Client) loginStep1(ctx context.Context) (string, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api-getsessionid.json"

	body, status, err := c.simpleGET(ctx, endpoint.String())
	if err != nil {
		return "", fmt.Errorf("login: getsessionid: %w", err)
	}
	if status == http.StatusUnauthorized {
		return "", fmt.Errorf("login: getsessionid: %w (http=401 body=%s)", ErrUnauthorized, string(body))
	}
	if status >= 400 {
		return "", fmt.Errorf("login: getsessionid: %s", &APIError{HTTPStatus: status, RawBody: body})
	}

	var env getsessionidWire
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("login: getsessionid: parse response: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		reason := zentaoFailReason(env.Error, env.Message, env.Reason)
		return "", fmt.Errorf("login: getsessionid: %s",
			&APIError{HTTPStatus: status, ZentaoStatus: env.Status, Reason: reason, RawBody: body})
	}
	var inner getsessionidInner
	if err := DecodeData(CtrlEnvelope{Data: env.Data}, &inner); err != nil {
		return "", fmt.Errorf("login: getsessionid: %w (body=%s)", err, string(body))
	}
	if inner.SessionID == "" {
		return "", fmt.Errorf("login: getsessionid: empty sessionID (body=%s)", string(body))
	}
	return inner.SessionID, nil
}

func (c *Client) loginStep2(ctx context.Context, sid string) (string, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/user-login.json"
	q := url.Values{}
	q.Set("account", c.account)
	// Credentials in query string is a known wart of the v1 apilogin
	// flow; ZenTao expects them this way and the alternative POST form
	// is not consistently supported across versions. Operators should
	// avoid logging full URLs from this client at the access-log layer.
	q.Set("password", c.password)
	q.Set("zentaosid", sid)
	endpoint.RawQuery = q.Encode()

	body, status, err := c.simpleGET(ctx, endpoint.String())
	if err != nil {
		return "", fmt.Errorf("login: apilogin: %w", err)
	}
	if status == http.StatusUnauthorized {
		return "", fmt.Errorf("login: apilogin: %w (http=401 body=%s)", ErrUnauthorized, string(body))
	}
	if status >= 400 {
		return "", &APIError{HTTPStatus: status, RawBody: body}
	}

	var wire loginV1Wire
	if err := json.Unmarshal(body, &wire); err != nil {
		return "", fmt.Errorf("login: apilogin: parse response: %w (body=%s)", err, string(body))
	}
	if wire.Status != "success" {
		reason := wire.failReason()
		if isUnauthorizedReason(reason) {
			return "", fmt.Errorf("login: %w (reason=%s)", ErrUnauthorized, reason)
		}
		return "", &APIError{HTTPStatus: status, ZentaoStatus: wire.Status, Reason: reason, RawBody: body}
	}
	if wire.Token == "" {
		return "", fmt.Errorf("login: apilogin: empty token in response (body=%s)", string(body))
	}
	return wire.Token, nil
}

// simpleGET is a one-shot GET that bypasses doRequest's session-refresh
// machinery — Login itself must not recurse into refreshSession. It
// reuses the same http.Client (and thus the cookiejar + redirect
// suppression) so cookies set by step 1 ride into step 2.
func (c *Client) simpleGET(ctx context.Context, urlStr string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// isSessionExpired detects "session is gone" across both transport
// flavours: V2 endpoints answer with HTTP 401, while Controller routes
// either redirect (302 → /user-login*) or carry a 200-OK envelope with
// reason "please login" / "请重新登录".
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
