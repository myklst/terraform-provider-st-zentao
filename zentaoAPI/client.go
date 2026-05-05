package zentaoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
)

// jsonURL is a thin alias purely for test ergonomics; consumers use *url.URL.
type jsonURL = url.URL

type Client struct {
	baseURL  *url.URL
	account  string
	password string

	tokenMu sync.Mutex
	token   string

	// refreshMu serializes refreshSession attempts so that concurrent
	// session-expired signals trigger only a single Login round-trip.
	// The first goroutine to acquire it performs Login; subsequent
	// goroutines see a refreshed token and no-op.
	refreshMu sync.Mutex

	http *http.Client

	backoffMaxElapsed      time.Duration
	backoffInitialInterval time.Duration
}

func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("base URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse base URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("base URL %q must include scheme and host", raw)
	}
	return u, nil
}

func NewClient(baseURL, account, password string) (*Client, error) {
	if account == "" || password == "" {
		return nil, fmt.Errorf("account and password required")
	}
	u, err := parseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("init cookie jar: %w", err)
	}
	c := &Client{
		baseURL:  u,
		account:  account,
		password: password,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			// Suppress automatic redirect-following so 302 → /user-login
			// (the Controller "session expired" signal) is visible to
			// isSessionExpired instead of being silently followed into
			// the login page HTML.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		backoffMaxElapsed:      2 * time.Minute,
		backoffInitialInterval: 500 * time.Millisecond,
	}
	if err := c.Login(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

// doRequest performs a single API call with token-based authentication.
// On any session-expired signal (HTTP 401, 302→login, or 200+please-
// login envelope) it refreshes the token and replays the request once.
func (c *Client) doRequest(ctx context.Context, method, path string, query map[string]string, body any) ([]byte, int, error) {
	c.tokenMu.Lock()
	observedToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err := c.send(ctx, method, path, query, body, observedToken)
	if err != nil {
		return rawBody, status, err
	}
	if !isSessionExpired(status, rawBody, location) {
		return rawBody, status, nil
	}

	if err := c.refreshSession(ctx, observedToken); err != nil {
		return nil, status, fmt.Errorf("session refresh: %w", err)
	}

	c.tokenMu.Lock()
	newToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err = c.send(ctx, method, path, query, body, newToken)
	if err != nil {
		return rawBody, status, err
	}
	if isSessionExpired(status, rawBody, location) {
		return rawBody, status, fmt.Errorf("session refresh exhausted: still expired after replay")
	}
	return rawBody, status, nil
}

func (c *Client) send(ctx context.Context, method, path string, query map[string]string, body any, token string) ([]byte, int, string, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	q := endpoint.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	endpoint.RawQuery = q.Encode()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, "", fmt.Errorf("marshal body: %w", err)
		}
	}

	var (
		rawBody  []byte
		status   int
		location string
	)

	op := func() error {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reqBody)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("build request: %w", err))
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Token", token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err // retry on transient network errors
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		rawBody = b
		status = resp.StatusCode
		location = resp.Header.Get("Location")

		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error %d", resp.StatusCode)
		}
		return nil
	}

	expo := backoff.NewExponentialBackOff()
	expo.InitialInterval = c.backoffInitialInterval
	expo.MaxElapsedTime = c.backoffMaxElapsed

	if err := backoff.Retry(op, backoff.WithContext(expo, ctx)); err != nil {
		return rawBody, status, location, err
	}
	return rawBody, status, location, nil
}
