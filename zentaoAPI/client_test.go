package zentaoapi

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- NewClient construction sanity ---

func TestNewClient_HasCookieJar(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{})
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.http.Jar == nil {
		t.Fatal("expected http.Client.Jar to be initialised")
	}
}

func TestNewClient_CheckRedirectDisabled(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{})
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.http.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set (block auto-follow)")
	}
	got := c.http.CheckRedirect(nil, nil)
	if got != http.ErrUseLastResponse {
		t.Fatalf("CheckRedirect = %v, want http.ErrUseLastResponse", got)
	}
}

// --- doRequest happy paths ---

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

// --- session-expiry refresh & replay (three signals) ---

func TestDoRequest_SessionExpiry_Via401(t *testing.T) {
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

func TestDoRequest_SessionExpiry_Via302LoginRedirect(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Token") == "old-tok" {
				w.Header().Set("Location", "/zentao/user-login-L3plbnRhby8=.html")
				w.WriteHeader(http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"42\"}"}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, status, err := c.doRequest(context.Background(), http.MethodGet, "user-view-admin.json", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (post-replay)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

func TestDoRequest_SessionExpiry_ViaPleaseLoginBody(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Token") == "old-tok" {
				_, _ = w.Write([]byte(`{"status":"failed","reason":"please login"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"7\"}"}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, status, err := c.doRequest(context.Background(), http.MethodGet, "user-view-admin.json", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

func TestDoRequest_SessionExpiry_RefreshExhausted(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		sessionID: "new-tok",
		handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
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

func TestDoRequest_ConcurrentExpiry_SingleLogin(t *testing.T) {
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

// --- backoff & 4xx semantics (unchanged from V2 path) ---

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

	if _, status, err := c.doRequest(context.Background(), http.MethodGet, "api.php/v2/products", nil, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	} else if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on 4xx)", calls.Load())
	}
}

// --- cookiejar persistence verification ---

func TestDoRequest_CookieJarPersistsAcrossRequests(t *testing.T) {
	var firstCookie, secondCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/first" {
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
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "api/first", nil, nil); err != nil {
		t.Fatalf("first req: %v", err)
	}
	if firstCookie != "" {
		t.Fatalf("first req should not carry cookie yet, got %q", firstCookie)
	}
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "api/second", nil, nil); err != nil {
		t.Fatalf("second req: %v", err)
	}
	if secondCookie != "from-first" {
		t.Fatalf("second req cookie = %q, want from-first (jar persistence broken)", secondCookie)
	}
}

func TestSend_InjectsZentaosidQueryWhenTokenSet(t *testing.T) {
	var sawSid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-xyz", srv.URL)
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "user-view-admin.json", nil, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if sawSid != "tok-xyz" {
		t.Fatalf("zentaosid query = %q, want tok-xyz", sawSid)
	}
}

func TestSend_OmitsZentaosidWhenTokenEmpty(t *testing.T) {
	var sawSid string
	var sawSidPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawSidPresent = r.URL.Query()["zentaosid"]
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "", srv.URL) // empty token: bootstrap window
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "product-all.json", nil, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if sawSidPresent || sawSid != "" {
		t.Fatalf("expected no zentaosid query when token empty, got %q (present=%v)", sawSid, sawSidPresent)
	}
}

func TestSend_OmitsZentaosidOnV2Path(t *testing.T) {
	var sawSidPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawSidPresent = r.URL.Query()["zentaosid"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-xyz", srv.URL)
	if _, _, err := c.doRequest(context.Background(), http.MethodPut, "api.php/v2/programs/77",
		nil, map[string]string{"name": "x"}); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if sawSidPresent {
		t.Fatal("V2 path must NOT receive zentaosid query (server mis-parses on PUT)")
	}
}

func TestSend_RespectsCallerSuppliedZentaosid(t *testing.T) {
	var sawSid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-from-client", srv.URL)
	if _, _, err := c.doRequest(context.Background(), http.MethodGet, "x.json",
		map[string]string{"zentaosid": "tok-from-caller"}, nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if sawSid != "tok-from-caller" {
		t.Fatalf("zentaosid = %q, want caller-supplied to win", sawSid)
	}
}

func TestDoRequest_AutoRedirectDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/somewhere" {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, status, err := c.doRequest(context.Background(), http.MethodGet, "somewhere", nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want 302 (auto-follow should be disabled)", status)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (no follow)", hits.Load())
	}
}

// --- shared test helpers ---

func newTestClient(t *testing.T, token string, srvURL string) *Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &Client{
		baseURL:                mustParseURL(t, srvURL),
		account:                "admin",
		password: "p",
		token:                  token,
		http: &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		backoffMaxElapsed:      2 * time.Second,
		backoffInitialInterval: 1 * time.Millisecond,
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

// silence unused-import warning when readAllString is unused.
var _ = readAllString
