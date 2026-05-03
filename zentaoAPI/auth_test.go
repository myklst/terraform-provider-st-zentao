package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin_Success(t *testing.T) {
	var gotPath, gotMethod string
	var gotForm map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotMethod = r.Method
		_ = r.ParseForm()
		gotForm = map[string]string{
			"account":  r.PostForm.Get("account"),
			"password": r.PostForm.Get("password"),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionID\":\"abc123\"}"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p@ss")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.sessionID != "abc123" {
		t.Fatalf("sessionID = %q, want abc123", c.sessionID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if !strings.Contains(gotPath, "m=user") || !strings.Contains(gotPath, "f=apilogin") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotForm["account"] != "admin" || gotForm["password"] != "p@ss" {
		t.Fatalf("form = %+v", gotForm)
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"wrong account or password"}`))
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

// Used by later tests too.
func newTestClient(t *testing.T, sessionID string, srvURL string) *Client {
	t.Helper()
	c := &Client{
		baseURL:   mustParseURL(t, srvURL),
		account:   "admin",
		password:  "p",
		sessionID: sessionID,
		http:      &http.Client{},
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

// helpers used by other tests
type respondJSON struct {
	body string
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func TestIsSessionExpired(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"http 401", 401, "", true},
		{"please login body", 200, `{"status":"failed","reason":"please login"}`, true},
		{"please login mixed case", 200, `{"status":"failed","reason":"Please Login"}`, true},
		{"normal failure", 200, `{"status":"failed","reason":"name exists"}`, false},
		{"success", 200, `{"status":"success","data":{}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSessionExpired(tc.status, []byte(tc.body)); got != tc.want {
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
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionID\":\"new-sid\"}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-sid", srv.URL)

	// double-check: simulate that another goroutine already refreshed by passing
	// an oldSID that no longer matches the current sessionID
	c.sessionID = "already-refreshed"
	if err := c.refreshSession(context.Background(), "old-sid"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls != 0 {
		t.Fatalf("Login should not be called when session already changed, got %d", calls)
	}

	// first observer: oldSID matches current → Login is invoked
	c.sessionID = "old-sid"
	if err := c.refreshSession(context.Background(), "old-sid"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Login should be called once, got %d", calls)
	}
	if c.sessionID != "new-sid" {
		t.Fatalf("sessionID = %q, want new-sid", c.sessionID)
	}
}
