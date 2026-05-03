package zentaoapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRequest_HappyPath_TokenHeader(t *testing.T) {
	var gotMethod, gotPath, gotToken, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("Token")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"1","name":"a"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	body, status, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products/1", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotToken != "tok-1" {
		t.Fatalf("token header = %q, want tok-1", gotToken)
	}
	if gotPath != "/api.php/v2/products/1" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody != "" {
		t.Fatalf("GET should not have body, got %q", gotBody)
	}
	if !strings.Contains(string(body), `"status":"success"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDoRequest_NoTokenHeaderWhenEmpty(t *testing.T) {
	var sawToken bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawToken = r.Header["Token"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "", srv.URL)
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products", nil, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if sawToken {
		t.Fatal("expected no Token header when client has empty token")
	}
}

func TestDoRequest_PostJSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":1}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	payload := map[string]string{"name": "x"}
	if _, _, err := c.doRequest(context.Background(), http.MethodPost, "api.php/v2/products", nil, payload); err != nil {
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
		if r.URL.Path == "/api.php/v2/users/login" {
			loginCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","token":"new-tok"}`))
			return
		}
		apiCalls.Add(1)
		if r.Header.Get("Token") == "old-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"token expired"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"99"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	body, status, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products/99", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(string(body), `"id":"99"`) {
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
		if r.URL.Path == "/api.php/v2/users/login" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","token":"new-tok"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, _, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products/1", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "session refresh exhausted") {
		t.Fatalf("error = %v, want session refresh exhausted", err)
	}
}

func TestSend_BackoffOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	c.backoffMaxElapsed = 5 * time.Second
	c.backoffInitialInterval = 10 * time.Millisecond

	body, status, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	if !strings.Contains(string(body), "success") {
		t.Fatalf("body = %s", body)
	}
}

func TestSend_NoBackoffOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	c.backoffMaxElapsed = 5 * time.Second
	c.backoffInitialInterval = 10 * time.Millisecond

	// 4xx: send returns (body, 400, nil); doRequest returns (body, 400, nil) since 400 != 401.
	// Caller (e.g., GetProduct) maps 4xx to APIError. The point of this test is that 4xx is NOT retried.
	if _, status, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products", nil, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	} else if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on 4xx)", calls.Load())
	}
}

func TestDoRequest_ConcurrentExpiry_SingleLogin(t *testing.T) {
	var loginCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api.php/v2/users/login" {
			loginCalls.Add(1)
			time.Sleep(50 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","token":"new-tok"}`))
			return
		}
		if r.Header.Get("Token") == "old-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"1"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products/1", nil, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("goroutine err: %v", err)
		}
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
}

// silence unused-import warning when readAllString is unused.
var _ = readAllString
