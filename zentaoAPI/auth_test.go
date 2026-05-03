package zentaoapi

import (
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
