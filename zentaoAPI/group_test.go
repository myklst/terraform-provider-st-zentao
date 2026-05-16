package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// --- Group.UnmarshalJSON ---

// Mirrors the inner.group object from group-edit-<id>.json GET on the
// lek-ws ZenTao Max 8.x instance (probe-group-controller.md §2.3).
func TestGroup_UnmarshalJSON_FullPayload(t *testing.T) {
	raw := `{
		"id": 10000002,
		"project": 28,
		"name": "tf-probe-pg-1778265940",
		"role": "",
		"desc": "tf-probe project group desc",
		"acl": [],
		"developer": 0,
		"vision": "rnd"
	}`
	var g Group
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deref(g.ID) != 10000002 {
		t.Fatalf("ID = %d", deref(g.ID))
	}
	if deref(g.Project) != 28 {
		t.Fatalf("Project = %d", deref(g.Project))
	}
	if deref(g.Name) != "tf-probe-pg-1778265940" {
		t.Fatalf("Name = %q", deref(g.Name))
	}
	if g.Role == nil || *g.Role != "" {
		t.Fatalf("Role = %v (want pointer to empty string)", g.Role)
	}
	if deref(g.Desc) != "tf-probe project group desc" {
		t.Fatalf("Desc = %q", deref(g.Desc))
	}
	if deref(g.Vision) != "rnd" {
		t.Fatalf("Vision = %q", deref(g.Vision))
	}
}

// project-group-<projectID>.json list view returns acl as "" (string),
// not [] (array). Numeric ids may arrive as quoted strings. Both shapes
// must decode without error; the acl column itself is intentionally
// dropped (Q2: NULL on this deployment).
func TestGroup_UnmarshalJSON_StringNumbersAndAclIgnored(t *testing.T) {
	raw := `{
		"id": "10000002",
		"project": "28",
		"name": "g",
		"role": "dev",
		"desc": "",
		"acl": "",
		"developer": "1",
		"vision": "rnd"
	}`
	var g Group
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deref(g.ID) != 10000002 || deref(g.Project) != 28 || deref(g.Developer) != 1 {
		t.Fatalf("numerics decoded wrong: %+v", g)
	}
	if deref(g.Role) != "dev" {
		t.Fatalf("Role = %q", deref(g.Role))
	}
}

// Absent fields must stay nil so M-Z merge can distinguish "wire omitted"
// from "wire said empty string".
func TestGroup_UnmarshalJSON_AbsentFieldsStayNil(t *testing.T) {
	raw := `{"id": 7}`
	var g Group
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deref(g.ID) != 7 {
		t.Fatalf("ID = %d", deref(g.ID))
	}
	for label, field := range map[string]any{
		"Project":   g.Project,
		"Name":      g.Name,
		"Role":      g.Role,
		"Desc":      g.Desc,
		"Vision":    g.Vision,
		"Developer": g.Developer,
	} {
		// reflect-free nil check by re-asserting via the underlying typed pointer.
		switch v := field.(type) {
		case *int64:
			if v != nil {
				t.Errorf("%s should be nil, got %v", label, *v)
			}
		case *string:
			if v != nil {
				t.Errorf("%s should be nil, got %q", label, *v)
			}
		}
	}
}

func TestGroup_UnmarshalJSON_BadID(t *testing.T) {
	var g Group
	if err := json.Unmarshal([]byte(`{"id":"not-a-number"}`), &g); err == nil {
		t.Fatal("expected error for malformed id")
	}
}

// --- GetGroup ---

func TestGetGroup_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/group-edit-7.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"x\",\"group\":{\"id\":7,\"project\":28,\"name\":\"alpha\",\"role\":\"dev\",\"desc\":\"d\",\"acl\":[],\"developer\":0,\"vision\":\"rnd\"},\"pager\":null}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	g, err := c.GetGroup(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if deref(g.ID) != 7 || deref(g.Project) != 28 || deref(g.Name) != "alpha" || deref(g.Role) != "dev" || deref(g.Desc) != "d" {
		t.Fatalf("group = %+v", g)
	}
}

// Sole observed not-found shape from the probe (§2.3).
func TestGetGroup_NotFound_NullInner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"x\",\"group\":null,\"pager\":null}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroup(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetGroup_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroup(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetGroup_EnvelopeFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Group does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroup(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (via classifyCtrlError)", err)
	}
}

func TestGetGroup_MalformedEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroup(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "decode get-group envelope") {
		t.Fatalf("err = %v, want envelope decode error", err)
	}
}

// --- CreateGroup ---

// project>0 → list via project-group-<projectID>.json, then GetGroup
// re-fetches the final row. Total: 3 round trips.
func TestCreateGroup_Happy_LookupThenRefetch_ProjectScoped(t *testing.T) {
	var hits int
	var createCT string
	var createForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			if r.Method != http.MethodPost {
				t.Errorf("hit1 method = %s", r.Method)
			}
			if r.URL.Path != "/group-create.json" {
				t.Errorf("hit1 path = %s", r.URL.Path)
			}
			createCT = r.Header.Get("Content-Type")
			_ = r.ParseForm()
			createForm = r.PostForm
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"保存成功"}`))
		case 2:
			if r.URL.Path != "/project-group-28.json" {
				t.Errorf("hit2 path = %s, want /project-group-28.json", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"groups\":[{\"id\":42,\"project\":28,\"name\":\"alpha\",\"role\":\"dev\",\"desc\":\"d\",\"acl\":\"\",\"developer\":0,\"vision\":\"rnd\"}]}"}`))
		case 3:
			if r.URL.Path != "/group-edit-42.json" {
				t.Errorf("hit3 path = %s, want /group-edit-42.json (re-fetch)", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":42,\"project\":28,\"name\":\"alpha\",\"role\":\"dev\",\"desc\":\"d\",\"acl\":[],\"developer\":0,\"vision\":\"rnd\"}}"}`))
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	g := &Group{Project: int64ptr(28), Name: strptr("alpha"), Role: strptr("dev"), Desc: strptr("d")}
	got, err := c.CreateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if deref(got.ID) != 42 {
		t.Fatalf("ID = %d, want 42 (resolved via list, refetched via GET)", deref(got.ID))
	}
	if deref(got.Project) != 28 || deref(got.Name) != "alpha" || deref(got.Role) != "dev" {
		t.Fatalf("got = %+v", got)
	}
	if hits != 3 {
		t.Fatalf("expected 3 server hits (create + list + refetch), got %d", hits)
	}
	if createCT != "application/x-www-form-urlencoded" {
		t.Fatalf("create content-type = %q", createCT)
	}
	for k, want := range map[string]string{
		"name":    "alpha",
		"project": "28",
		"role":    "dev",
		"desc":    "d",
		"vision":  "rnd", // default applied
	} {
		if createForm.Get(k) != want {
			t.Errorf("form[%q] = %q, want %q", k, createForm.Get(k), want)
		}
	}
}

// project=0 must list via /group-browse.json (system view), not the
// per-project endpoint. Project-scoped rows are not returned by
// group-browse on Max 8.x — verified by probe.
func TestCreateGroup_Happy_LookupThenRefetch_SystemScoped(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			if r.URL.Path != "/group-create.json" {
				t.Errorf("hit1 path = %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"load":true,"result":"success"}`))
		case 2:
			if r.URL.Path != "/group-browse.json" {
				t.Errorf("hit2 path = %s, want /group-browse.json (system flavour)", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"groups\":[{\"id\":99,\"project\":0,\"name\":\"sysadmin\",\"role\":\"admin\",\"desc\":\"\",\"acl\":[],\"developer\":1,\"vision\":\"rnd\"}]}"}`))
		case 3:
			if r.URL.Path != "/group-edit-99.json" {
				t.Errorf("hit3 path = %s, want /group-edit-99.json", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":99,\"project\":0,\"name\":\"sysadmin\",\"role\":\"admin\",\"desc\":\"\",\"acl\":[],\"developer\":1,\"vision\":\"rnd\"}}"}`))
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.CreateGroup(context.Background(), &Group{Project: int64ptr(0), Name: strptr("sysadmin"), Role: strptr("admin")})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if deref(got.ID) != 99 {
		t.Fatalf("ID = %d, want 99", deref(got.ID))
	}
	if deref(got.Project) != 0 {
		t.Fatalf("Project = %d, want 0", deref(got.Project))
	}
}

func TestCreateGroup_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"name":["不能为空"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateGroup(context.Background(), &Group{Project: int64ptr(28), Name: strptr("x")})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Reason, "name") {
		t.Fatalf("reason should mention field, got %q", apiErr.Reason)
	}
}

// Create succeeded but list returns no match — extremely defensive,
// mostly guards against name mismatch caused by server-side trimming.
func TestCreateGroup_LookupReturnsEmpty_Surfaced(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(`{"result":"success","load":true}`))
		} else {
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"groups\":[]}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateGroup(context.Background(), &Group{Project: int64ptr(28), Name: strptr("alpha")})
	if err == nil {
		t.Fatal("expected error when post-create lookup yields no match")
	}
}

func TestCreateGroup_PreflightRejection(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name string
		g    *Group
	}{
		{"nil group", nil},
		{"negative project", &Group{Project: int64ptr(-1), Name: strptr("a")}},
		{"missing name", &Group{Project: int64ptr(28)}},
		{"empty name", &Group{Project: int64ptr(28), Name: strptr("")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.CreateGroup(context.Background(), tc.g); err == nil {
				t.Fatal("expected pre-flight error")
			}
		})
	}
}

// --- UpdateGroup ---

// M-Z merge: baseline GET fills in fields the caller did not set; the
// caller's non-nil overrides win. Total: GET baseline + POST + GET refetch.
func TestUpdateGroup_BaselineMergeThenRefetch(t *testing.T) {
	var posts, gets int
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/group-edit-7.json":
			posts++
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			_, _ = w.Write([]byte(`{"load":true,"closeModal":true,"result":"success","message":"保存成功"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/group-edit-7.json":
			gets++
			// Baseline has role=engineer, desc=oldDesc, vision=rnd, developer=1 —
			// none of which the caller mentions in the input.
			if gets == 1 {
				_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":7,\"project\":28,\"name\":\"oldName\",\"role\":\"engineer\",\"desc\":\"oldDesc\",\"acl\":[],\"developer\":1,\"vision\":\"rnd\"}}"}`))
			} else {
				_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":7,\"project\":28,\"name\":\"renamed\",\"role\":\"engineer\",\"desc\":\"oldDesc\",\"acl\":[],\"developer\":1,\"vision\":\"rnd\"}}"}`))
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateGroup(context.Background(), &Group{ID: int64ptr(7), Name: strptr("renamed")})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if posts != 1 || gets != 2 {
		t.Fatalf("posts=%d gets=%d, want 1/2 (baseline + POST + refetch)", posts, gets)
	}
	if !strings.Contains(postBody, "name=renamed") {
		t.Errorf("POST body missing name override: %s", postBody)
	}
	for _, mustPreserve := range []string{"project=28", "role=engineer", "desc=oldDesc", "vision=rnd", "developer=1"} {
		if !strings.Contains(postBody, mustPreserve) {
			t.Errorf("POST body missing baseline preservation %q: %s", mustPreserve, postBody)
		}
	}
	if deref(out.ID) != 7 || deref(out.Name) != "renamed" {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateGroup_FailEnvelope(t *testing.T) {
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			gets++
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":7,\"project\":28,\"name\":\"x\",\"role\":\"\",\"desc\":\"\",\"acl\":[],\"developer\":0,\"vision\":\"rnd\"}}"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":"fail","message":"name is required"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateGroup(context.Background(), &Group{ID: int64ptr(7), Project: int64ptr(28), Name: strptr("x")})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if gets != 1 {
		t.Fatalf("expected baseline GET before POST, gets=%d", gets)
	}
}

func TestUpdateGroup_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	if _, err := c.UpdateGroup(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil")
	}
	if _, err := c.UpdateGroup(context.Background(), &Group{Project: int64ptr(28), Name: strptr("x")}); err == nil {
		t.Fatal("expected error for ID=nil")
	}
}

// With M-Z merge the baseline GET fires first; a missing row surfaces
// as ErrNotFound without ever sending the POST.
func TestUpdateGroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Error("POST must not fire when baseline GET returns not-found")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":null}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateGroup(context.Background(), &Group{ID: int64ptr(99999), Project: int64ptr(28), Name: strptr("x")})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// --- DeleteGroup ---

func TestDeleteGroup_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Probe verified: NO confirm=yes required (§0.1).
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/group-delete-7.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","message":"","load":"/zentao/group-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteGroup(context.Background(), 7); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
}

func TestDeleteGroup_HTTP404_Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteGroup(context.Background(), 999); err != nil {
		t.Fatalf("delete on missing should be idempotent: %v", err)
	}
}

func TestDeleteGroup_FailEnvelope_Surfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"insufficient privs"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteGroup(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}

func TestDeleteGroup_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"unknown":"shape"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteGroup(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError fallback", err)
	}
}

// --- Group.toForm ---

func TestGroup_ToForm_AppliesVisionDefault(t *testing.T) {
	g := &Group{Project: int64ptr(28), Name: strptr("g")}
	form := g.toForm()
	if form.Get("vision") != "rnd" {
		t.Fatalf("default vision = %q, want rnd", form.Get("vision"))
	}
	if form.Get("project") != "28" {
		t.Fatalf("project = %q", form.Get("project"))
	}
	if form.Get("name") != "g" {
		t.Fatalf("name = %q", form.Get("name"))
	}
}

func TestGroup_ToForm_HonoursExplicitVision(t *testing.T) {
	g := &Group{Project: int64ptr(28), Name: strptr("g"), Vision: strptr("lite")}
	form := g.toForm()
	if form.Get("vision") != "lite" {
		t.Fatalf("explicit vision lost: %q", form.Get("vision"))
	}
}

func TestGroup_ToForm_SystemScopedSendsProjectZero(t *testing.T) {
	g := &Group{Project: int64ptr(0), Name: strptr("sysadmin"), Role: strptr("admin")}
	form := g.toForm()
	if form.Get("project") != "0" {
		t.Fatalf("project = %q, want 0 (system flavour)", form.Get("project"))
	}
}

// developer != 0 must round-trip through toForm so baseline merge can
// preserve it. The default-zero case omits the field entirely (matching
// historical behavior, since the column defaults to 1 server-side).
func TestGroup_ToForm_DeveloperRoundTrip(t *testing.T) {
	withDev := &Group{Project: int64ptr(28), Name: strptr("g"), Developer: int64ptr(1)}
	if got := withDev.toForm().Get("developer"); got != "1" {
		t.Fatalf("developer = %q, want 1", got)
	}
	withoutDev := &Group{Project: int64ptr(28), Name: strptr("g")}
	if _, ok := withoutDev.toForm()["developer"]; ok {
		t.Fatalf("developer key must be absent when nil/zero")
	}
}
