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

func TestControllerPath(t *testing.T) {
	cases := []struct {
		name   string
		module string
		method string
		args   []string
		want   string
	}{
		{"zero args", "user", "view", nil, "user-view.json"},
		{"empty slice args", "user", "view", []string{}, "user-view.json"},
		{"one arg", "user", "view", []string{"admin"}, "user-view-admin.json"},
		{"trailing single empty", "product", "all", []string{"noclosed", ""}, "product-all-noclosed.json"},
		{"trailing multiple empty", "product", "all", []string{"noclosed", "", ""}, "product-all-noclosed.json"},
		{"middle empty preserved", "product", "all", []string{"noclosed", "", "25", "1"}, "product-all-noclosed--25-1.json"},
		{"all empty args", "user", "view", []string{"", "", ""}, "user-view.json"},
		{"leading empty preserved", "x", "y", []string{"", "z"}, "x-y--z.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := controllerPath(tc.module, tc.method, tc.args)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDoController_GET_Happy(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotCT, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotToken = r.Header.Get("Token")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"1\"}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	body, status, err := c.doController(context.Background(), "user", "view", []string{"admin"}, nil, nil)
	if err != nil {
		t.Fatalf("doController: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET (body nil → GET)", gotMethod)
	}
	if gotPath != "/user-view-admin.json" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody != "" {
		t.Fatalf("GET should have no body, got %q", gotBody)
	}
	if gotCT != "" {
		t.Fatalf("GET should not set Content-Type, got %q", gotCT)
	}
	if gotToken != "tok-1" {
		t.Fatalf("token header = %q", gotToken)
	}
	if !strings.Contains(string(body), `"status":"success"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDoController_POST_Happy(t *testing.T) {
	var gotMethod, gotPath, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"42\"}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	payload := map[string]string{"account": "alice", "password": "p"}
	body, status, err := c.doController(context.Background(), "user", "create", nil, nil, payload)
	if err != nil {
		t.Fatalf("doController: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST (body present → POST)", gotMethod)
	}
	if gotPath != "/user-create.json" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotCT)
	}
	if !strings.Contains(gotBody, `"account":"alice"`) {
		t.Fatalf("body = %q", gotBody)
	}
	if !strings.Contains(string(body), `42`) || !strings.Contains(string(body), `"status":"success"`) {
		t.Fatalf("response body = %s", body)
	}
}

func TestDoController_QueryPlumbed(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	q := map[string]string{"confirm": "yes", "onlybody": "yes"}
	if _, _, err := c.doController(context.Background(), "user", "delete", []string{"42"}, q, nil); err != nil {
		t.Fatalf("doController: %v", err)
	}
	if !strings.Contains(gotQuery, "confirm=yes") || !strings.Contains(gotQuery, "onlybody=yes") {
		t.Fatalf("query = %q, want both params", gotQuery)
	}
}

func TestDoController_RedirectToLogin_TriggersRefreshSmoke(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Token") == "old-tok" {
				w.Header().Set("Location", "/zentao/user-login.html")
				w.WriteHeader(http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"7\"}"}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	_, status, err := c.doController(context.Background(), "user", "view", []string{"admin"}, nil, nil)
	if err != nil {
		t.Fatalf("doController: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d (post-replay should be 200)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2 (initial + replay)", apiCalls.Load())
	}
}

func TestCallController_PublicHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/product-all.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"products\":{\"1\":\"a\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	body, status, err := c.CallController(context.Background(), "product", "all", nil, nil, nil)
	if err != nil {
		t.Fatalf("CallController: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(string(body), `"status":"success"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestCallController_EnvelopeFailureFlowsThrough(t *testing.T) {
	// CallController is a transport-level escape hatch — it should surface
	// the raw body + status without classifying business failures. Caller
	// is expected to parse via CtrlEnvelope + classifyCtrlError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","reason":"name exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	body, status, err := c.CallController(context.Background(), "product", "create", nil, nil, map[string]string{"name": "dup"})
	if err != nil {
		t.Fatalf("CallController should not error on business failure: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(string(body), "name exists") {
		t.Fatalf("body should carry envelope verbatim, got %s", body)
	}
}
