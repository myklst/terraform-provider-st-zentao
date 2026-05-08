package zentaoapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// --- doV1Request happy path ---

func TestDoV1Request_HappyPath_TokenHeader(t *testing.T) {
	var gotMethod, gotPath, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":1,"total":0,"limit":20,"products":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	body, status, err := c.doV1Request(context.Background(), http.MethodGet, apiV1PathPrefix+"products", nil, nil)
	if err != nil {
		t.Fatalf("doV1Request: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != apiV1PathPrefix+"products" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "tok-1" {
		t.Fatalf("token header = %q, want tok-1", gotToken)
	}
	if !strings.Contains(string(body), `"page":1`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDoV1Request_PostJSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if _, _, err := c.doV1Request(context.Background(), http.MethodPost, apiV1PathPrefix+"widgets", nil, map[string]string{"name": "x"}); err != nil {
		t.Fatalf("doV1Request: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"name":"x"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

// --- V1 expiry: 401 OR 403 trigger refresh + replay ---

func TestDoV1Request_SessionExpiry_Via401(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Token") == "old-tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"products":[]}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, status, err := c.doV1Request(context.Background(), http.MethodGet, apiV1PathPrefix+"products", nil, nil)
	if err != nil {
		t.Fatalf("doV1Request: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d (post-replay)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

func TestDoV1Request_SessionExpiry_Via403(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			// Probe 2026-05-08: V1 returns 403 for unauthenticated requests.
			if r.Header.Get("Token") == "old-tok" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"products":[]}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, status, err := c.doV1Request(context.Background(), http.MethodGet, apiV1PathPrefix+"products", nil, nil)
	if err != nil {
		t.Fatalf("doV1Request: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d (post-replay)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

// --- V1 transport never injects zentaosid query (Token-header only) ---

func TestDoV1Request_NeverInjectsZentaosid(t *testing.T) {
	var sawSidPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawSidPresent = r.URL.Query()["zentaosid"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"products":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-xyz", srv.URL)
	if _, _, err := c.doV1Request(context.Background(), http.MethodGet, apiV1PathPrefix+"products", nil, nil); err != nil {
		t.Fatalf("doV1Request: %v", err)
	}
	if sawSidPresent {
		t.Fatal("V1 path must NOT receive zentaosid query (Token header is the documented carrier)")
	}
}

// --- isV1SessionExpired table ---

func TestIsV1SessionExpired(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"http 401 → expired (REST convention, defensive)", 401, true},
		{"http 403 → expired (probe 2026-05-08 actual)", 403, true},
		{"http 200 → fine", 200, false},
		{"http 404 → not expired (resource missing)", 404, false},
		{"http 400 → not expired (validation)", 400, false},
		{"http 500 → not expired (server fault)", 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isV1SessionExpired(tc.status, nil, "")
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
