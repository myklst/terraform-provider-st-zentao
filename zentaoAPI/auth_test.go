package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin_Success(t *testing.T) {
	var gotPath, gotMethod, gotCT string
	var gotPayload struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","token":"tok-abc"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p@ss")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.token != "tok-abc" {
		t.Fatalf("token = %q, want tok-abc", c.token)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api.php/v2/users/login" {
		t.Fatalf("path = %q, want /api.php/v2/users/login", gotPath)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotCT)
	}
	if gotPayload.Account != "admin" || gotPayload.Password != "p@ss" {
		t.Fatalf("payload = %+v", gotPayload)
	}
}

func TestLogin_Unauthorized_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"wrong account or password"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_NetworkError(t *testing.T) {
	_, err := NewClient("http://127.0.0.1:1", "admin", "p")
	if err == nil {
		t.Fatalf("expected error from unreachable host")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("network error should not be ErrUnauthorized")
	}
}

func TestLogin_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","token":""}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("err = %v, want empty token error", err)
	}
}

func TestIsSessionExpired(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"http 401", 401, true},
		{"http 200", 200, false},
		{"http 404", 404, false},
		{"http 500", 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSessionExpired(tc.status); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestRefreshSession_DoubleCheck(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","token":"new-tok"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)

	// peer already refreshed: observed token no longer matches current
	c.token = "already-refreshed"
	if err := c.refreshSession(context.Background(), "old-tok"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls != 0 {
		t.Fatalf("Login should not be called when session already changed, got %d", calls)
	}

	// first observer: observed matches current → Login fires
	c.token = "old-tok"
	if err := c.refreshSession(context.Background(), "old-tok"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Login should be called once, got %d", calls)
	}
	if c.token != "new-tok" {
		t.Fatalf("token = %q, want new-tok", c.token)
	}
}

// --- shared test helpers ---

func newTestClient(t *testing.T, token string, srvURL string) *Client {
	t.Helper()
	c := &Client{
		baseURL:  mustParseURL(t, srvURL),
		account:  "admin",
		password: "p",
		token:    token,
		http:     &http.Client{},
	}
	return c
}

func mustParseURL(t *testing.T, raw string) *jsonURL {
	t.Helper()
	u, err := parseBaseURL(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

// readAllString drains an io.Reader for ergonomic test assertions.
func readAllString(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
