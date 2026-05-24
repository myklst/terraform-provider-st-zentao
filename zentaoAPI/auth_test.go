package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// auth_test.go covers the credential lifecycle defined in auth.go:
// V1 Login (POST /api.php/v1/tokens) success and failure paths,
// plus refreshSession's lock-and-double-check behaviour. The V1
// login mock factory used here (newV1LoginServer + v1Opts) is also
// consumed by every transport test file.

// --- V1 Login (POST /api.php/v1/tokens) ---

func TestLogin_V1_Success(t *testing.T) {
	var hits atomic.Int32
	var gotMethod, gotPath, gotContentType string
	var gotAccount, gotPassword string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NewClient now also runs the Controller apilogin; serve those two
		// routes so construction completes and the V1 assertions can run.
		if r.URL.Path == getSessionIDPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"success","data":"{\"sessionID\":\"ctrl-sid-1\",\"sessionName\":\"zentaosid\"}"}`)
			return
		}
		if r.URL.Path == userLoginPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"success","token":"ctrl-sid-1","user":{"account":"admin"}}`)
			return
		}
		if r.URL.Path != apiV1PathPrefix+"tokens" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		var creds map[string]string
		_ = json.Unmarshal(body, &creds)
		gotAccount = creds["account"]
		gotPassword = creds["password"]
		// Real server returns 201 Created on success; we accept both 200 and 201.
		http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: "tok-final", Path: "/", HttpOnly: true})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"tok-final"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p@ss")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("login hits = %d, want 1", hits.Load())
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != apiV1PathPrefix+"tokens" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	if gotAccount != "admin" || gotPassword != "p@ss" {
		t.Fatalf("creds = %q/%q", gotAccount, gotPassword)
	}
	if c.token != "tok-final" {
		t.Fatalf("client token = %q, want tok-final", c.token)
	}
}

func TestLogin_BadCreds_ChineseReason(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginStatus: http.StatusBadRequest,
		loginBody:   `{"error":"登录失败，请检查您的用户名或密码是否填写正确。"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_BadCreds_EnglishReason(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginStatus: http.StatusBadRequest,
		loginBody:   `{"error":"wrong password"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_GenericFail_NotUnauthorized(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginStatus: http.StatusBadRequest,
		loginBody:   `{"error":"some weird thing"}`,
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

func TestLogin_HTTP401_TreatedAsUnauthorized(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginStatus: http.StatusUnauthorized,
		loginBody:   `{"error":"unauthorized"}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_EmptyToken(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginBody: `{"token":""}`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("err = %v, want empty token error", err)
	}
}

func TestLogin_MalformedResponseBody(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		loginBody: `not-json`,
	})
	defer srv.Close()

	_, err := NewClient(srv.URL, "admin", "p")
	if err == nil {
		t.Fatal("expected parse error on malformed body")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("malformed body must not be ErrUnauthorized: %v", err)
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

// --- refreshSession double-check (peer-already-refreshed no-op + matched
//     observer triggers Login). Per-transport expiry detectors live in
//     their respective *_transport_test.go files. ---

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

// --- Controller apilogin (two-step) ---

func TestLoginController_Success(t *testing.T) {
	var gotSessionIDHit, gotLoginHit bool
	var gotAccount, gotPassword, gotZentaosid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case getSessionIDPath:
			gotSessionIDHit = true
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: "sid-42", Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"success","data":"{\"sessionID\":\"sid-42\",\"sessionName\":\"zentaosid\"}"}`)
		case userLoginPath:
			gotLoginHit = true
			gotAccount = r.URL.Query().Get("account")
			gotPassword = r.URL.Query().Get("password")
			gotZentaosid = r.URL.Query().Get("zentaosid")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"success","token":"sid-42","user":{"account":"admin"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "v1-token", srv.URL)
	c.password = "s3cret"
	if err := c.loginController(context.Background()); err != nil {
		t.Fatalf("loginController: %v", err)
	}
	if !gotSessionIDHit || !gotLoginHit {
		t.Fatalf("expected both apilogin steps to fire (getsessionid=%v, login=%v)", gotSessionIDHit, gotLoginHit)
	}
	if c.ctrlSID != "sid-42" {
		t.Fatalf("ctrlSID = %q, want sid-42", c.ctrlSID)
	}
	if c.token != "v1-token" {
		t.Fatalf("loginController must not touch the V1 token, got %q", c.token)
	}
	if gotAccount != "admin" || gotPassword != "s3cret" || gotZentaosid != "sid-42" {
		t.Fatalf("apilogin query = account:%q password:%q zentaosid:%q", gotAccount, gotPassword, gotZentaosid)
	}
}

func TestLoginController_NoUser_IsFailure(t *testing.T) {
	// A silent form re-render returns status:success WITHOUT a user — must
	// be treated as a login failure rather than a phantom success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case getSessionIDPath:
			_, _ = io.WriteString(w, `{"status":"success","data":"{\"sessionID\":\"sid-1\"}"}`)
		case userLoginPath:
			_, _ = io.WriteString(w, `{"status":"success","data":"{\"title\":\"用户登录\"}"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", srv.URL)
	err := c.loginController(context.Background())
	if err == nil {
		t.Fatal("expected error when apilogin returns no user")
	}
}

func TestLoginController_EmptySessionID_IsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == getSessionIDPath {
			_, _ = io.WriteString(w, `{"status":"success","data":"{\"sessionID\":\"\"}"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", srv.URL)
	err := c.loginController(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty sessionID") {
		t.Fatalf("err = %v, want empty sessionID error", err)
	}
}

func TestNewClient_EstablishesBothCredentials(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{sessionID: "v1-tok", ctrlSID: "ctrl-sid"})
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.token != "v1-tok" {
		t.Fatalf("token = %q, want v1-tok", c.token)
	}
	if c.ctrlSID != "ctrl-sid" {
		t.Fatalf("ctrlSID = %q, want ctrl-sid (independent apilogin credential)", c.ctrlSID)
	}
}

func TestRefreshControllerSession_DoubleCheck(t *testing.T) {
	var ctrlLogins atomic.Int32
	srv := newV1LoginServer(t, v1Opts{ctrlSID: "new-sid", ctrlLoginCalls: &ctrlLogins})
	defer srv.Close()

	c := newTestClient(t, "old-sid", srv.URL)

	// Peer already rotated the sessionID → no apilogin.
	c.ctrlSID = "already-refreshed"
	if err := c.refreshControllerSession(context.Background(), "old-sid"); err != nil {
		t.Fatalf("refreshControllerSession: %v", err)
	}
	if ctrlLogins.Load() != 0 {
		t.Fatalf("apilogin should not fire when sessionID already changed, got %d", ctrlLogins.Load())
	}

	// Matched observer → apilogin fires and rotates ctrlSID.
	c.ctrlSID = "old-sid"
	if err := c.refreshControllerSession(context.Background(), "old-sid"); err != nil {
		t.Fatalf("refreshControllerSession: %v", err)
	}
	if ctrlLogins.Load() != 1 {
		t.Fatalf("apilogin should fire once, got %d", ctrlLogins.Load())
	}
	if c.ctrlSID != "new-sid" {
		t.Fatalf("ctrlSID = %q, want new-sid", c.ctrlSID)
	}
}

// --- V1 login mock factory (shared across every transport test) ---

type v1Opts struct {
	// sessionID returned as the token in the success body. Default: "tok-1".
	sessionID string
	// loginBody, when non-empty, replaces the success body for /api.php/v1/tokens.
	loginBody string
	// loginStatus, when non-zero, replaces the success status (default 201) for the login response.
	loginStatus int
	// loginCalls, when non-nil, increments on each /api.php/v1/tokens hit.
	loginCalls *atomic.Int32
	// apiCalls, when non-nil, increments on every non-login, non-apilogin path hit.
	apiCalls *atomic.Int32
	// handler, when non-nil, serves any non-login, non-apilogin path.
	handler http.HandlerFunc

	// --- Controller-transport apilogin (two-step) mock knobs ---
	// ctrlSID is the sessionID handed out by GET /api-getsessionid.json and
	// echoed by GET /user-login.json. Default: "ctrl-sid-1". Tests use a
	// value distinct from sessionID to assert which credential a request carries.
	ctrlSID string
	// ctrlLoginCalls, when non-nil, increments once per apilogin round
	// (counted on the GET /api-getsessionid.json hit).
	ctrlLoginCalls *atomic.Int32
}

// newV1LoginServer returns an httptest.Server that emulates ZenTao's
// V1 token endpoint (POST /api.php/v1/tokens). Tests that need to
// observe or perturb login behaviour configure it via v1Opts. Non-
// login paths fall through to opts.handler (or 404 if absent).
func newV1LoginServer(t *testing.T, opts v1Opts) *httptest.Server {
	t.Helper()
	if opts.sessionID == "" {
		opts.sessionID = "tok-1"
	}
	if opts.ctrlSID == "" {
		opts.ctrlSID = "ctrl-sid-1"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiV1PathPrefix+"tokens" && r.Method == http.MethodPost {
			if opts.loginCalls != nil {
				opts.loginCalls.Add(1)
			}
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: opts.sessionID, Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			if opts.loginStatus != 0 {
				w.WriteHeader(opts.loginStatus)
			} else {
				w.WriteHeader(http.StatusCreated)
			}
			if opts.loginBody != "" {
				_, _ = io.WriteString(w, opts.loginBody)
				return
			}
			_, _ = fmt.Fprintf(w, `{"token":"%s"}`, opts.sessionID)
			return
		}
		// --- Controller-transport apilogin (two-step) ---
		if r.URL.Path == getSessionIDPath {
			if opts.ctrlLoginCalls != nil {
				opts.ctrlLoginCalls.Add(1)
			}
			http.SetCookie(w, &http.Cookie{Name: "zentaosid", Value: opts.ctrlSID, Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"status":"success","data":"{\"sessionID\":\"%s\",\"sessionName\":\"zentaosid\"}"}`, opts.ctrlSID)
			return
		}
		if r.URL.Path == userLoginPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"status":"success","token":"%s","user":{"account":"admin"}}`, opts.ctrlSID)
			return
		}
		if opts.apiCalls != nil {
			opts.apiCalls.Add(1)
		}
		if opts.handler != nil {
			opts.handler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}
