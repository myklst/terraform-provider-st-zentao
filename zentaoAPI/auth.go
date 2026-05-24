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
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + apiV1PathPrefix + "tokens"

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

// ============================================================================
// Controller-transport session (two-step apilogin)
// ============================================================================
//
// The V1 token from POST /api.php/v1/tokens drives the V1 and V2 REST
// surfaces, but on some ZenTao deployments (observed on a Max 8.1
// HTTPS install — see probe-controller-auth.md 2026-05-24 addendum) the
// API token store and the PHP session store are SEPARATE: the V1 token
// authenticates V1/V2 but NOT the PATH_INFO Controller routes, whose
// checkPriv reads $_SESSION['user']. The two-step apilogin below
// establishes a real logged-in PHP session whose sessionID drives the
// Controller transport via ?zentaosid=. This is a second, independent
// credential (c.ctrlSID) maintained alongside c.token.

const (
	// getSessionIDPath bootstraps a fresh PHP sessionID; the value is
	// echoed inside a string-wrapped Controller envelope and also set as
	// the zentaosid cookie.
	getSessionIDPath = "/api-getsessionid.json"
	// userLoginPath performs the apilogin GET that promotes a bootstrap
	// sessionID into a logged-in one (writes $_SESSION['user']).
	userLoginPath = "/user-login.json"
)

// getSessionIDInnerResp is the inner (string-wrapped) payload of
// GET /api-getsessionid.json: {"sessionID":"<32hex>","sessionName":"zentaosid",...}.
type getSessionIDInnerResp struct {
	SessionID   string `json:"sessionID"`
	SessionName string `json:"sessionName"`
}

// userLoginResp is the response of GET /user-login.json?account&password&zentaosid.
// On success the body is a FLAT envelope carrying a logged-in user:
// {"status":"success","token":"<sid>","user":{"account":"admin",...}}.
// A failed login (or a silent form re-render) returns status:success with
// NO user object — so presence of user.account, not status alone, is the
// success signal.
type userLoginResp struct {
	Status string `json:"status"`
	User   struct {
		Account string `json:"account"`
	} `json:"user"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// loginController establishes (or re-establishes) the Controller-transport
// PHP session via ZenTao's two-step apilogin and stores its sessionID under
// ctrlMu. Like Login, it reuses c.http so the zentaosid cookie persists in
// the shared jar; it must not recurse into doWithRefresh.
func (c *Client) loginController(ctx context.Context) error {
	sid, err := c.fetchControllerSessionID(ctx)
	if err != nil {
		return fmt.Errorf("controller login: %w", err)
	}
	if err := c.controllerApilogin(ctx, sid); err != nil {
		return fmt.Errorf("controller login: %w", err)
	}
	c.ctrlMu.Lock()
	c.ctrlSID = sid
	c.ctrlMu.Unlock()
	return nil
}

// fetchControllerSessionID performs step 1 of apilogin: GET
// /api-getsessionid.json, returning the bootstrap sessionID. The matching
// zentaosid cookie lands in c.http's jar as a side effect.
func (c *Client) fetchControllerSessionID(ctx context.Context) (string, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + getSessionIDPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("getsessionid: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("getsessionid: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("getsessionid: read body: %w (http=%d)", err, resp.StatusCode)
	}

	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("getsessionid: parse response: %w (body=%s)", err, string(body))
	}
	var sess getSessionIDInnerResp
	if err := env.DecodeData(&sess); err != nil {
		return "", fmt.Errorf("getsessionid: decode data: %w", err)
	}
	if sess.SessionID == "" {
		return "", fmt.Errorf("getsessionid: empty sessionID (body=%s)", string(body))
	}
	return sess.SessionID, nil
}

// controllerApilogin performs step 2: GET /user-login.json with credentials
// and the bootstrap sessionID, promoting it to a logged-in session. Success
// is signalled by a non-empty user.account in the flat envelope; a missing
// user (form re-render / rejected credentials) is treated as a login
// failure, mapped to ErrUnauthorized when the reason looks credential-related.
func (c *Client) controllerApilogin(ctx context.Context, sid string) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + userLoginPath
	q := endpoint.Query()
	q.Set("account", c.account)
	q.Set("password", c.password)
	q.Set("zentaosid", sid)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("apilogin: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apilogin: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("apilogin: read body: %w (http=%d)", err, resp.StatusCode)
	}

	var wire userLoginResp
	if err := json.Unmarshal(body, &wire); err != nil {
		return fmt.Errorf("apilogin: parse response: %w (http=%d)", err, resp.StatusCode)
	}
	if wire.Status == "success" && wire.User.Account != "" {
		return nil
	}
	// Login did not take. Surface a credential failure as ErrUnauthorized
	// (do NOT echo the password-bearing URL); otherwise a structured error.
	reason := zentaoFailReason(wire.Error, wire.Message, wire.Reason)
	if isUnauthorizedReason(reason) {
		return fmt.Errorf("apilogin: %w (reason=%s)", ErrUnauthorized, reason)
	}
	return &APIError{HTTPStatus: resp.StatusCode, ZentaoStatus: wire.Status, Reason: reason, RawBody: body}
}

// refreshControllerSession re-runs loginController if the caller's observed
// sessionID is still current. Mirrors refreshSession's lock-and-double-check
// so concurrent Controller-transport expiry signals collapse into one
// apilogin round-trip.
func (c *Client) refreshControllerSession(ctx context.Context, observedSID string) error {
	c.ctrlRefreshMu.Lock()
	defer c.ctrlRefreshMu.Unlock()

	c.ctrlMu.Lock()
	current := c.ctrlSID
	c.ctrlMu.Unlock()
	if current != observedSID {
		return nil
	}
	return c.loginController(ctx)
}
