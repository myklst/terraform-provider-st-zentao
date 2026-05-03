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

// refreshSession re-runs Login if the caller's observed sessionID is still current.
// If another goroutine has already rotated the sessionID, this is a no-op (double-check).
func (c *Client) refreshSession(ctx context.Context, observedSID string) error {
	c.sessionMu.Lock()
	if c.sessionID != observedSID {
		c.sessionMu.Unlock()
		return nil
	}
	c.sessionMu.Unlock()
	return c.Login(ctx)
}
