package zentaoapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// --- v1 two-step Login ---

func TestLogin_TwoStep_Success(t *testing.T) {
	var step1Hits, step2Hits atomic.Int32
	var step2Account, step2Password, step2QuerySID string
	var step2CookieSID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api-getsessionid.json":
			step1Hits.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: "sid-bootstrap", Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionName\":\"zentaosid\",\"sessionID\":\"sid-bootstrap\",\"rand\":1}","md5":"x"}`))
		case "/user-login.json":
			step2Hits.Add(1)
			step2Account = r.URL.Query().Get("account")
			step2Password = r.URL.Query().Get("password")
			step2QuerySID = r.URL.Query().Get("zentaosid")
			if c, err := r.Cookie("zentaosid"); err == nil {
				step2CookieSID = c.Value
			}
			// Real server may rotate the sessionID after login; emulate that.
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: "sid-final", Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","token":"sid-final","user":{"account":"admin"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p@ss")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if step1Hits.Load() != 1 {
		t.Fatalf("step1 hits = %d, want 1", step1Hits.Load())
	}
	if step2Hits.Load() != 1 {
		t.Fatalf("step2 hits = %d, want 1", step2Hits.Load())
	}
	if step2Account != "admin" || step2Password != "p@ss" {
		t.Fatalf("step2 creds = %q/%q", step2Account, step2Password)
	}
	if step2QuerySID != "sid-bootstrap" {
		t.Fatalf("step2 zentaosid query = %q, want sid-bootstrap", step2QuerySID)
	}
	if step2CookieSID != "sid-bootstrap" {
		t.Fatalf("step2 cookie sid = %q, want sid-bootstrap (cookiejar carry-over)", step2CookieSID)
	}
	if c.token != "sid-final" {
		t.Fatalf("client token = %q, want sid-final", c.token)
	}
}

func TestLogin_Step1_BadEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","reason":"server malfunction"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil {
		t.Fatal("expected step1 failure to bubble up")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("step1 server fault should not be ErrUnauthorized: %v", err)
	}
}

func TestLogin_Step1_NoSessionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"rand\":1}"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil {
		t.Fatal("expected error on missing sessionID")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "session") {
		t.Fatalf("expected session-related error, got %v", err)
	}
}

func TestLogin_Step2_BadCreds_ChineseReason(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		step2Body: `{"status":"fail","reason":"用户名或密码错误"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_Step2_BadCreds_EnglishReason(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		step2Body: `{"status":"fail","reason":"wrong password"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_Step2_GenericFail_NotUnauthorized(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		step2Body: `{"status":"fail","reason":"some weird thing"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unrelated fail should not be ErrUnauthorized: %v", err)
	}
}

func TestLogin_Step2_HTTP401_TreatedAsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api-getsessionid.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"sessionID\":\"sid\"}"}`))
		case "/user-login.json":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad creds"}`))
		}
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_Step2_EmptyToken(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		step2Body: `{"status":"success","token":""}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("err = %v, want empty token error", err)
	}
}

func TestLogin_NetworkError(t *testing.T) {
	_, err := NewClient("http://127.0.0.1:1", "admin", "p")
	if err == nil {
		t.Fatal("expected error from unreachable host")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatal("network error must not be ErrUnauthorized")
	}
}

// --- isSessionExpired three-case table ---

func TestIsSessionExpired(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		location string
		want     bool
	}{
		{"http 401", 401, "", "", true},
		{"302 → user-login (controller)", 302, "", "/zentao/user-login-L3plbnRhby8=.html", true},
		{"302 → user-login.html plain", 302, "", "https://example/user-login.html", true},
		{"302 → unrelated redirect", 302, "", "/some/other/place", false},
		{"200 + please login reason", 200, `{"status":"failed","reason":"please login"}`, "", true},
		{"200 + 请重新登录", 200, `{"status":"failed","reason":"请重新登录"}`, "", true},
		{"200 + business success", 200, `{"status":"success","data":"x"}`, "", false},
		{"200 + unrelated failure", 200, `{"status":"fail","reason":"name exists"}`, "", false},
		{"http 200 with empty body", 200, "", "", false},
		{"http 404", 404, "", "", false},
		{"http 500", 500, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isSessionExpired(tc.status, []byte(tc.body), tc.location)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// --- refresh + double-check (v1 mocks) ---

func TestRefreshSession_DoubleCheck(t *testing.T) {
	var calls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &calls,
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)

	// Peer already refreshed → no Login call.
	c.token = "already-refreshed"
	if err := c.refreshSession(context.Background(), "old-tok"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("Login should not be called when session already changed, got %d", calls.Load())
	}

	// First observer: matches → Login fires.
	c.token = "old-tok"
	if err := c.refreshSession(context.Background(), "old-tok"); err != nil {
		t.Fatalf("refreshSession: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("Login should be called once, got %d", calls.Load())
	}
	if c.token != "new-tok" {
		t.Fatalf("token = %q, want new-tok", c.token)
	}
}

// --- v1 mock factory ---

type v1Opts struct {
	// sessionID returned to bootstrap and (when step2Body is empty) reused as token. Default: "tok-1".
	sessionID string
	// step2Body, when non-empty, replaces the success body for /user-login.json.
	step2Body string
	// step1Body, when non-empty, replaces the success body for /api-getsessionid.json.
	step1Body string
	// loginCalls, when non-nil, increments on each /user-login.json hit.
	loginCalls *atomic.Int32
	// apiCalls, when non-nil, increments on every non-login path hit.
	apiCalls *atomic.Int32
	// handler, when non-nil, serves any non-login path.
	handler http.HandlerFunc
}

// newV1LoginServer returns an httptest.Server that emulates the v1 two-step
// login flow used by Phase B's Login() rewrite. Tests that need to observe
// or perturb login behaviour configure it via v1Opts.
func newV1LoginServer(t *testing.T, opts v1Opts) *httptest.Server {
	t.Helper()
	if opts.sessionID == "" {
		opts.sessionID = "tok-1"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api-getsessionid.json":
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: opts.sessionID, Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			if opts.step1Body != "" {
				_, _ = io.WriteString(w, opts.step1Body)
				return
			}
			_, _ = fmt.Fprintf(w, `{"status":"success","data":"{\"sessionName\":\"zentaosid\",\"sessionID\":\"%s\",\"rand\":1}","md5":"x"}`, opts.sessionID)
		case "/user-login.json":
			if opts.loginCalls != nil {
				opts.loginCalls.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			if opts.step2Body != "" {
				_, _ = io.WriteString(w, opts.step2Body)
				return
			}
			_, _ = fmt.Fprintf(w, `{"status":"success","token":"%s","user":{"account":"admin"}}`, opts.sessionID)
		default:
			if opts.apiCalls != nil {
				opts.apiCalls.Add(1)
			}
			if opts.handler != nil {
				opts.handler(w, r)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
