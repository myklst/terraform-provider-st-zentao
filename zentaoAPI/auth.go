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

// Login obtains a v2 API token via POST /api.php/v2/users/login.
// The token is stored on the Client and sent as the "token" header on
// every subsequent request.
func (c *Client) Login(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api.php/v2/users/login"
	endpoint.RawQuery = ""

	payload, err := json.Marshal(map[string]string{
		"account":  c.account,
		"password": c.password,
	})
	if err != nil {
		return fmt.Errorf("login: marshal: %w", err)
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
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("login: read body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("login: %w (http=401 body=%s)", ErrUnauthorized, string(body))
	}

	var env struct {
		ZentaoResponse
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("login: parse response: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return fmt.Errorf("login: %w (reason=%s)", ErrUnauthorized, env.ZentaoFailReason())
	}
	if env.Token == "" {
		return fmt.Errorf("login: empty token in response (body=%s)", string(body))
	}

	c.tokenMu.Lock()
	c.token = env.Token
	c.tokenMu.Unlock()
	return nil
}

// isSessionExpired detects v2 token rejection. v2 returns HTTP 401 when
// the token is missing or stale; no envelope sniffing needed.
func isSessionExpired(httpStatus int) bool {
	return httpStatus == http.StatusUnauthorized
}

// refreshSession re-runs Login if the caller's observed token is still
// current. Concurrent callers serialize on refreshMu so only one Login is
// in flight at a time; later callers re-check the token under refreshMu
// and no-op if a peer already rotated it.
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
