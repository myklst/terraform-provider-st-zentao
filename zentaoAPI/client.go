package zentaoapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// jsonURL is a thin alias purely for test ergonomics; consumers use *url.URL.
type jsonURL = url.URL

type Client struct {
	baseURL  *url.URL
	account  string
	password string

	sessionMu sync.Mutex
	sessionID string

	http *http.Client
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
	c := &Client{
		baseURL:  u,
		account:  account,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
	if err := c.Login(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}
