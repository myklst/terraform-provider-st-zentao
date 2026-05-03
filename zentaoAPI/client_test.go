package zentaoapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDoRequest_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotSID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotSID = r.URL.Query().Get("zentaosid")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"id":1,"name":"a"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)

	body, err := c.doRequest(context.Background(), http.MethodGet, "api.php", map[string]string{
		"m": "product",
		"f": "view",
	}, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotSID != "sid-1" {
		t.Fatalf("sessionID not propagated: %q", gotSID)
	}
	if !strings.Contains(gotPath, "m=product") || !strings.Contains(gotPath, "f=view") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody != "" {
		t.Fatalf("GET should not have body, got %q", gotBody)
	}
	var env ZentaoResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.Status != "success" {
		t.Fatalf("status = %q", env.Status)
	}
}

func TestDoRequest_PostJSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	payload := map[string]string{"name": "x"}
	if _, err := c.doRequest(context.Background(), http.MethodPost, "api.php", map[string]string{"m": "product", "f": "create"}, payload); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"name":"x"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDoRequest_SessionExpiry_RefreshAndReplay(t *testing.T) {
	var apiCalls atomic.Int32
	var loginCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "f=apilogin") {
			loginCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionID\":\"new-sid\"}"}`))
			return
		}
		apiCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("zentaosid") == "old-sid" {
			_, _ = w.Write([]byte(`{"status":"failed","reason":"please login"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"id":99}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-sid", srv.URL)
	body, err := c.doRequest(context.Background(), http.MethodGet, "api.php", map[string]string{"m": "product", "f": "view"}, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if !strings.Contains(string(body), `"id":99`) {
		t.Fatalf("expected replayed response, got %s", body)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2 (initial + replay)", apiCalls.Load())
	}
}

func TestDoRequest_SessionExpiry_RefreshExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "f=apilogin") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionID\":\"new-sid\"}"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"please login"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-sid", srv.URL)
	_, err := c.doRequest(context.Background(), http.MethodGet, "api.php", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "session refresh exhausted") {
		t.Fatalf("error = %v, want session refresh exhausted", err)
	}
}
