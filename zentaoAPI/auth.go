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
