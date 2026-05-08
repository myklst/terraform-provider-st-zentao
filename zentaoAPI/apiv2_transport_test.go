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

// --- doV2Request happy paths ---

func TestDoV2Request_HappyPath_TokenHeader(t *testing.T) {
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
	body, status, err := c.doV2Request(context.Background(), http.MethodGet, productsPath+"/1", nil, nil)
	if err != nil {
		t.Fatalf("doV2Request: %v", err)
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
	if gotPath != productsPath+"/1" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody != "" {
		t.Fatalf("GET should not have body, got %q", gotBody)
	}
	if !strings.Contains(string(body), `"status":"success"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDoV2Request_NoTokenHeaderWhenEmpty(t *testing.T) {
	var sawToken bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawToken = r.Header["Token"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "", srv.URL)
	if _, _, err := c.doV2Request(context.Background(), http.MethodGet, productsPath, nil, nil); err != nil {
		t.Fatalf("doV2Request: %v", err)
	}
	if sawToken {
		t.Fatal("expected no Token header when client has empty token")
	}
}

func TestDoV2Request_PostJSONBody(t *testing.T) {
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
	if _, _, err := c.doV2Request(context.Background(), http.MethodPost, productsPath, nil, payload); err != nil {
		t.Fatalf("doV2Request: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"name":"x"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

// --- session-expiry refresh & replay (V2 transport: 401-driven) ---

func TestDoV2Request_SessionExpiry_Via401(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Token") == "old-tok" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"token expired"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","product":{"id":"99"}}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	body, status, err := c.doV2Request(context.Background(), http.MethodGet, productsPath+"/99", nil, nil)
	if err != nil {
		t.Fatalf("doV2Request: %v", err)
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

func TestDoV2Request_SessionExpiry_RefreshExhausted(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		sessionID: "new-tok",
		handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, _, err := c.doV2Request(context.Background(), http.MethodGet, productsPath+"/1", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "session refresh exhausted") {
		t.Fatalf("error = %v, want session refresh exhausted", err)
	}
}

func TestDoV2Request_ConcurrentExpiry_SingleLogin(t *testing.T) {
	var loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Token") == "old-tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","product":{"id":"1"}}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	// Slow down login a touch so 10 goroutines actually pile up before refresh completes.
	c.backoffInitialInterval = 1 * time.Millisecond

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.doV2Request(context.Background(), http.MethodGet, productsPath+"/1", nil, nil)
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

// --- backoff & 4xx semantics (sendHTTP shared across transports;
//     exercised via doV2Request) ---

func TestSendHTTP_BackoffOn5xx(t *testing.T) {
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

	body, status, err := c.doV2Request(context.Background(), http.MethodGet, productsPath, nil, nil)
	if err != nil {
		t.Fatalf("doV2Request: %v", err)
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

func TestSendHTTP_NoBackoffOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	c.backoffMaxElapsed = 5 * time.Second
	c.backoffInitialInterval = 10 * time.Millisecond

	if _, status, err := c.doV2Request(context.Background(), http.MethodGet, productsPath, nil, nil); err != nil {
		t.Fatalf("doV2Request: %v", err)
	} else if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on 4xx)", calls.Load())
	}
}

// --- cookiejar persistence (verified via V2 transport, behaviour shared) ---

func TestSendHTTP_CookieJarPersistsAcrossRequests(t *testing.T) {
	var firstCookie, secondCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiV2PathPrefix+"first" {
			http.SetCookie(w, &http.Cookie{Name: "marker", Value: "from-first", Path: "/"})
			if c, err := r.Cookie("marker"); err == nil {
				firstCookie = c.Value
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
			return
		}
		if c, err := r.Cookie("marker"); err == nil {
			secondCookie = c.Value
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if _, _, err := c.doV2Request(context.Background(), http.MethodGet, apiV2PathPrefix+"first", nil, nil); err != nil {
		t.Fatalf("first req: %v", err)
	}
	if firstCookie != "" {
		t.Fatalf("first req should not carry cookie yet, got %q", firstCookie)
	}
	if _, _, err := c.doV2Request(context.Background(), http.MethodGet, apiV2PathPrefix+"second", nil, nil); err != nil {
		t.Fatalf("second req: %v", err)
	}
	if secondCookie != "from-first" {
		t.Fatalf("second req cookie = %q, want from-first (jar persistence broken)", secondCookie)
	}
}

// --- V2 transport never injects zentaosid (regression guard) ---

func TestDoV2Request_NeverInjectsZentaosid(t *testing.T) {
	var sawSidPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawSidPresent = r.URL.Query()["zentaosid"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-xyz", srv.URL)
	if _, _, err := c.doV2Request(context.Background(), http.MethodPut, programsPath+"/77",
		nil, map[string]string{"name": "x"}); err != nil {
		t.Fatalf("doV2Request: %v", err)
	}
	if sawSidPresent {
		t.Fatal("V2 path must NOT receive zentaosid query (server mis-parses on PUT)")
	}
}

// --- redirect suppression (CheckRedirect plumbing) ---

func TestDoV2Request_AutoRedirectDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == apiV2PathPrefix+"somewhere" {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, status, err := c.doV2Request(context.Background(), http.MethodGet, apiV2PathPrefix+"somewhere", nil, nil)
	if err != nil {
		t.Fatalf("doV2Request: %v", err)
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want 302 (auto-follow should be disabled)", status)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (no follow)", hits.Load())
	}
}

// --- V2 envelope: ZentaoResponse.ZentaoFailReason ---

func TestZentaoResponse_FailReason(t *testing.T) {
	cases := []struct {
		name string
		env  ZentaoResponse
		want string
	}{
		{"v2 error field", ZentaoResponse{Error: "name exists"}, "name exists"},
		{"v2 message field", ZentaoResponse{Message: "Product does not exist."}, "Product does not exist."},
		{"v1 reason field", ZentaoResponse{Reason: "please login"}, "please login"},
		{"error wins over message", ZentaoResponse{Error: "e", Message: "m", Reason: "r"}, "e"},
		{"message wins over reason", ZentaoResponse{Message: "m", Reason: "r"}, "m"},
		{"empty", ZentaoResponse{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.env.ZentaoFailReason(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// --- isV2SessionExpired table ---

func TestIsV2SessionExpired(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"http 401 → expired", 401, true},
		{"http 200 → fine", 200, false},
		{"http 403 → not recognised on V2 (only 401 is documented)", 403, false},
		{"http 404 → not expired", 404, false},
		{"http 400 → not expired", 400, false},
		{"http 500 → not expired (server fault, not auth)", 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isV2SessionExpired(tc.status, nil, "")
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
