package zentaoapi

import (
	"bytes"
	"context"
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
			// isControllerSessionExpired instead of being silently
			// followed into the login page HTML.
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

// ============================================================================
// Shared HTTP helpers — used by every transport
// ============================================================================

// sendHTTP performs a single API request with exponential 5xx retry.
// It joins path onto c.baseURL, merges query, optionally injects the
// `zentaosid` query parameter (Controller transport's auth carrier),
// sets Accept/Content-Type/Token headers, executes with backoff, and
// returns body + status + Location header verbatim for the caller's
// per-transport expiry detector to classify.
//
// The injectZentaosid flag is the ONLY transport-specific dial here:
//   - V1 / V2 transports pass false — they authenticate via Token header
//     exclusively, and Max 8.x mis-parses zentaosid on V2 PUT (observed:
//     "Unknown column" SQL error from a unique-check using the long
//     hex value as a record id).
//   - Controller transport passes true — PATH_INFO routes authenticate
//     EXCLUSIVELY via this query parameter; cookie + Token header alone
//     yields 302 → /user-login.
//
// A caller-supplied query["zentaosid"] always wins over auto-injection;
// an empty token (Login still in flight) emits no auth at all. Body is
// taken pre-encoded (JSON-marshalled or form.Encode()'d) — this helper
// is content-type agnostic.
func (c *Client) sendHTTP(
	ctx context.Context,
	method, path string,
	query map[string]string,
	body []byte,
	contentType, token string,
	injectZentaosid bool,
) (rawBody []byte, status int, location string, err error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	q := endpoint.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	if injectZentaosid && token != "" && q.Get("zentaosid") == "" {
		q.Set("zentaosid", token)
	}
	endpoint.RawQuery = q.Encode()

	op := func() error {
		var bodyReader io.Reader
		if len(body) > 0 {
			bodyReader = bytes.NewReader(body)
		}
		req, reqErr := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
		if reqErr != nil {
			return backoff.Permanent(fmt.Errorf("build request: %w", reqErr))
		}
		// Content-Type is only meaningful when a body is present;
		// setting it on a body-less GET would be nonsensical and
		// could trigger weird server-side dispatch.
		if len(body) > 0 && contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Token", token)
		}
		resp, doErr := c.http.Do(req)
		if doErr != nil {
			return doErr // retry on transient network errors
		}
		defer func() { _ = resp.Body.Close() }()
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

	if err = backoff.Retry(op, backoff.WithContext(expo, ctx)); err != nil {
		return rawBody, status, location, err
	}
	return rawBody, status, location, nil
}

// doWithRefresh implements the "send → detect expiry → refresh once →
// replay" pattern shared by every transport. The transport supplies its
// own expiry detector (V1: 401/403, V2: 401, Controller: 302/please-login)
// and a send closure that knows how to actually issue the request with
// the supplied token.
//
// Refresh contention is handled inside refreshSession: concurrent callers
// observing the same expired token serialize on refreshMu and only one
// Login round-trip fires; the rest no-op once the token has rotated.
func (c *Client) doWithRefresh(
	ctx context.Context,
	isExpired func(status int, body []byte, location string) bool,
	send func(token string) (body []byte, status int, location string, err error),
) ([]byte, int, error) {
	c.tokenMu.Lock()
	observedToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err := send(observedToken)
	if err != nil {
		return rawBody, status, err
	}
	if !isExpired(status, rawBody, location) {
		return rawBody, status, nil
	}

	if err := c.refreshSession(ctx, observedToken); err != nil {
		return nil, status, fmt.Errorf("session refresh: %w", err)
	}

	c.tokenMu.Lock()
	newToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err = send(newToken)
	if err != nil {
		return rawBody, status, err
	}
	if isExpired(status, rawBody, location) {
		return rawBody, status, fmt.Errorf("session refresh exhausted: still expired after replay")
	}
	return rawBody, status, nil
}
