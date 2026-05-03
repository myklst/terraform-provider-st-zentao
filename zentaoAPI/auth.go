package zentaoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) Login(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api.php"
	q := endpoint.Query()
	q.Set("m", "user")
	q.Set("f", "apilogin")
	endpoint.RawQuery = q.Encode()

	form := url.Values{}
	form.Set("account", c.account)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("login: read body: %w", err)
	}

	var env ZentaoResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("login: parse envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return fmt.Errorf("login: %w (reason=%s)", ErrUnauthorized, env.Reason)
	}

	var data struct {
		SessionID string `json:"sessionID"`
	}
	if err := decodeData(&env, &data); err != nil {
		return fmt.Errorf("login: decode data: %w", err)
	}
	if data.SessionID == "" {
		return fmt.Errorf("login: empty sessionID in response")
	}

	c.sessionMu.Lock()
	c.sessionID = data.SessionID
	c.sessionMu.Unlock()
	return nil
}

// isSessionExpired detects ZenTao's two ways of signalling stale auth:
// HTTP 401 OR an HTTP 200 envelope with {"status":"failed","reason":"please login"}.
func isSessionExpired(httpStatus int, body []byte) bool {
	if httpStatus == http.StatusUnauthorized {
		return true
	}
	if httpStatus != http.StatusOK || len(body) == 0 {
		return false
	}
	var env ZentaoResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	if env.Status != "failed" {
		return false
	}
	return bytes.Contains(bytes.ToLower([]byte(env.Reason)), []byte("please login"))
}

// refreshSession re-runs Login if the caller's observed sessionID is still
// current. Concurrent callers serialize on refreshMu so only one Login is
// in flight at a time; later callers re-check the sessionID under refreshMu
// and no-op if a peer already rotated it.
func (c *Client) refreshSession(ctx context.Context, observedSID string) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.sessionMu.Lock()
	current := c.sessionID
	c.sessionMu.Unlock()
	if current != observedSID {
		return nil
	}
	return c.Login(ctx)
}
