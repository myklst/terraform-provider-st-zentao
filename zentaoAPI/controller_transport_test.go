package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
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

func TestDoControllerForm_POST_FormEncoded(t *testing.T) {
	var gotMethod, gotPath, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","message":"saved","load":"/zentao/foo"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	form := make(map[string][]string)
	form["account"] = []string{"alice"}
	form["password"] = []string{"P@ssw0rd"}
	form["realname"] = []string{"Alice"}
	form["visions"] = []string{"rnd"}

	body, status, err := c.doControllerForm(context.Background(), "user", "create", nil, nil, form)
	if err != nil {
		t.Fatalf("doControllerForm: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/user-create.json" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q", gotCT)
	}
	for _, want := range []string{"account=alice", "password=P%40ssw0rd", "realname=Alice", "visions=rnd"} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("body %q missing %q", gotBody, want)
		}
	}
	if !strings.Contains(string(body), `"result":"success"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDoControllerForm_QueryPlumbed(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	form := map[string][]string{"x": {"1"}}
	q := map[string]string{"confirm": "yes"}
	if _, _, err := c.doControllerForm(context.Background(), "user", "delete", []string{"5"}, q, form); err != nil {
		t.Fatalf("doControllerForm: %v", err)
	}
	if !strings.Contains(gotQuery, "confirm=yes") {
		t.Fatalf("query = %q, want confirm=yes", gotQuery)
	}
	// zentaosid must also be auto-injected (Controller path)
	if !strings.Contains(gotQuery, "zentaosid=tok-1") {
		t.Fatalf("query = %q, want zentaosid=tok-1", gotQuery)
	}
}

func TestDoControllerForm_RefreshAndReplay(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("zentaosid") == "old-tok" {
				w.Header().Set("Location", "/zentao/user-login.html")
				w.WriteHeader(http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"saved"}`))
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	form := map[string][]string{"name": {"x"}}
	_, status, err := c.doControllerForm(context.Background(), "user", "create", nil, nil, form)
	if err != nil {
		t.Fatalf("doControllerForm: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d (post-replay should be 200)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

func TestDoControllerForm_RefreshExhausted(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{
		sessionID: "still-expired",
		handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/zentao/user-login.html")
			w.WriteHeader(http.StatusFound)
		},
	})
	defer srv.Close()

	c := newTestClient(t, "old-tok", srv.URL)
	form := map[string][]string{"name": {"x"}}
	_, _, err := c.doControllerForm(context.Background(), "user", "create", nil, nil, form)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session refresh exhausted") {
		t.Fatalf("error = %v, want session refresh exhausted", err)
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

// --- zentaosid query plumbing (auto-inject / empty-token / caller-supplied) ---

func TestDoController_InjectsZentaosidQueryWhenTokenSet(t *testing.T) {
	var sawSid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-xyz", srv.URL)
	if _, _, err := c.doController(context.Background(), "user", "view", []string{"admin"}, nil, nil); err != nil {
		t.Fatalf("doController: %v", err)
	}
	if sawSid != "tok-xyz" {
		t.Fatalf("zentaosid query = %q, want tok-xyz (auto-injected from c.token)", sawSid)
	}
}

func TestDoController_OmitsZentaosidWhenTokenEmpty(t *testing.T) {
	var sawSid string
	var sawSidPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawSidPresent = r.URL.Query()["zentaosid"]
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "", srv.URL) // empty token: bootstrap window before Login completes
	if _, _, err := c.doController(context.Background(), "product", "all", nil, nil, nil); err != nil {
		t.Fatalf("doController: %v", err)
	}
	if sawSidPresent || sawSid != "" {
		t.Fatalf("expected no zentaosid query when token empty, got %q (present=%v)", sawSid, sawSidPresent)
	}
}

func TestDoController_RespectsCallerSuppliedZentaosid(t *testing.T) {
	var sawSid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSid = r.URL.Query().Get("zentaosid")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-from-client", srv.URL)
	q := map[string]string{"zentaosid": "tok-from-caller"}
	if _, _, err := c.doController(context.Background(), "user", "view", []string{"admin"}, q, nil); err != nil {
		t.Fatalf("doController: %v", err)
	}
	if sawSid != "tok-from-caller" {
		t.Fatalf("zentaosid = %q, want caller-supplied to win", sawSid)
	}
}

// --- 200 + please-login envelope drives refresh on Controller transport ---

func TestDoController_SessionExpiry_ViaPleaseLoginBody(t *testing.T) {
	var apiCalls, loginCalls atomic.Int32
	srv := newV1LoginServer(t, v1Opts{
		sessionID:  "new-tok",
		loginCalls: &loginCalls,
		apiCalls:   &apiCalls,
		handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("zentaosid") == "old-tok" {
				_, _ = w.Write([]byte(`{"status":"failed","reason":"please login"}`))
				return
			}
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
		t.Fatalf("status = %d (post-replay)", status)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("api calls = %d, want 2", apiCalls.Load())
	}
}

// --- isControllerSessionExpired table ---

func TestIsControllerSessionExpired(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		location string
		want     bool
	}{
		{"302 → user-login (controller)", 302, "", "/zentao/user-login-L3plbnRhby8=.html", true},
		{"302 → user-login.html plain", 302, "", "https://example/user-login.html", true},
		{"301 → user-login", 301, "", "/zentao/user-login.html", true},
		{"303 → user-login", 303, "", "/zentao/user-login.html", true},
		{"302 → unrelated redirect", 302, "", "/some/other/place", false},
		{"302 → no Location header", 302, "", "", false},
		{"200 + please login reason", 200, `{"status":"failed","reason":"please login"}`, "", true},
		{"200 + 请重新登录", 200, `{"status":"failed","reason":"请重新登录"}`, "", true},
		{"200 + business success", 200, `{"status":"success","data":"x"}`, "", false},
		{"200 + unrelated failure", 200, `{"status":"fail","reason":"name exists"}`, "", false},
		{"200 + empty body", 200, "", "", false},
		{"401 NOT recognised on Controller", 401, "", "", false},
		{"404 not expired", 404, "", "", false},
		{"500 not expired", 500, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isControllerSessionExpired(tc.status, []byte(tc.body), tc.location)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
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

// ============================================================================
// Controller wire envelope tests + classifiers + DecodeData + reason helper
// ============================================================================

func TestCtrlEnvelope_FailReason(t *testing.T) {
	cases := []struct {
		name string
		env  CtrlEnvelope
		want string
	}{
		{"error field", CtrlEnvelope{Error: "duplicate"}, "duplicate"},
		{"message field", CtrlEnvelope{Message: "user not exist"}, "user not exist"},
		{"reason field", CtrlEnvelope{Reason: "please login"}, "please login"},
		{"error wins", CtrlEnvelope{Error: "e", Message: "m", Reason: "r"}, "e"},
		{"empty", CtrlEnvelope{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.env.ZentaoFailReason(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestCtrlSimpleResponse_IsSuccess(t *testing.T) {
	cases := []struct {
		name string
		r    CtrlSimpleResponse
		want bool
	}{
		{"success", CtrlSimpleResponse{Result: "success"}, true},
		{"fail", CtrlSimpleResponse{Result: "fail"}, false},
		{"empty result", CtrlSimpleResponse{}, false},
		{"weird value", CtrlSimpleResponse{Result: "maybe"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.IsSuccess(); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCtrlSimpleResponse_FieldErrors(t *testing.T) {
	t.Run("flat string message", func(t *testing.T) {
		r := CtrlSimpleResponse{
			Result:  "fail",
			Message: json.RawMessage(`"系统用户人数已达授权的上限"`),
		}
		flat, fields := r.FieldErrors()
		if flat != "系统用户人数已达授权的上限" {
			t.Fatalf("flat = %q", flat)
		}
		if fields != nil {
			t.Fatalf("fields should be nil for flat message, got %v", fields)
		}
	})

	t.Run("per-field map message", func(t *testing.T) {
		r := CtrlSimpleResponse{
			Result:  "fail",
			Message: json.RawMessage(`{"verifyPassword":["验证失败"],"visions":["『界面类型』不能为空。"]}`),
		}
		flat, fields := r.FieldErrors()
		if flat != "" {
			t.Fatalf("flat should be empty for map message, got %q", flat)
		}
		if got := fields["verifyPassword"]; len(got) != 1 || got[0] != "验证失败" {
			t.Fatalf("verifyPassword field = %v", got)
		}
		if got := fields["visions"]; len(got) != 1 || got[0] != "『界面类型』不能为空。" {
			t.Fatalf("visions field = %v", got)
		}
	})

	t.Run("absent message", func(t *testing.T) {
		r := CtrlSimpleResponse{Result: "success"}
		flat, fields := r.FieldErrors()
		if flat != "" || fields != nil {
			t.Fatalf("absent message should yield empty: flat=%q fields=%v", flat, fields)
		}
	})

	t.Run("malformed message", func(t *testing.T) {
		r := CtrlSimpleResponse{
			Result:  "fail",
			Message: json.RawMessage(`12345`),
		}
		flat, fields := r.FieldErrors()
		if flat == "" && fields == nil {
			t.Log("acceptable as long as it doesn't crash")
		}
	})
}

func TestDecodeData(t *testing.T) {
	type sample struct {
		Title    string         `json:"title"`
		Products map[string]any `json:"products"`
	}

	t.Run("V1 string-encoded JSON object", func(t *testing.T) {
		raw := `"{\"title\":\"产品\",\"products\":{\"218\":\"ds_x\"}}"`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "产品" || got.Products["218"] != "ds_x" {
			t.Fatalf("unexpected decode: %+v", got)
		}
	})

	t.Run("V2 direct object", func(t *testing.T) {
		raw := `{"title":"x","products":{"1":"a"}}`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "x" {
			t.Fatalf("unexpected decode: %+v", got)
		}
	})

	t.Run("direct array", func(t *testing.T) {
		raw := `[1,2,3]`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got []int
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 1 {
			t.Fatalf("unexpected decode: %v", got)
		}
	})

	t.Run("null data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`null`)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "" {
			t.Fatalf("expected zero-value, got %+v", got)
		}
	})

	t.Run("absent data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success"}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty string data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`""`)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-JSON data is error", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`garbage`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected error, got nil with %+v", got)
		}
	})

	t.Run("string-encoded with malformed inner JSON", func(t *testing.T) {
		// Outer is a valid quoted string, but inner content isn't JSON.
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`"not json"`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected inner-decode error")
		}
	})

	t.Run("direct object with type mismatch", func(t *testing.T) {
		// target expects object, data is array — should error from unmarshal.
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`[1,2,3]`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected unmarshal type error")
		}
	})
}

func TestClassifyCtrlError(t *testing.T) {
	t.Run("not found via message", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Message: "User does not exist"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("unauthorized via reason", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "wrong password"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("unauthorized chinese 密码错误", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "用户名或密码错误"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("generic fail → APIError", func(t *testing.T) {
		raw := []byte(`{"status":"fail","message":"validation failed"}`)
		env := CtrlEnvelope{Status: "fail", Message: "validation failed"}
		err := classifyCtrlError(200, env, raw)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *APIError, got %v", err)
		}
		if apiErr.ZentaoStatus != "fail" || apiErr.Reason != "validation failed" {
			t.Fatalf("unexpected APIError: %+v", apiErr)
		}
	})

	t.Run("login redirect ≠ unauthorized", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "please login"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		// session-expired reason should NOT collapse into ErrUnauthorized;
		// it's a transport-layer concern handled before classifyCtrlError.
		if errors.Is(err, ErrUnauthorized) {
			t.Fatalf("please-login should not be classified as ErrUnauthorized: %v", err)
		}
	})
}

func TestIsLoginRedirectReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"please login", true},
		{"Please Login", true},
		{"PLEASE LOGIN.", true},
		{"请重新登录", true},
		{"请登录", true},
		{"session expired", true},
		{"Product does not exist.", false},
		{"name exists", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			if got := isLoginRedirectReason(tc.reason); got != tc.want {
				t.Fatalf("isLoginRedirectReason(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}
