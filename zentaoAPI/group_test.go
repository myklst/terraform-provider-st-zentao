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

// groupCtrlWire round-trip — the wire type tolerates both shapes
// observed in the probe: ACL as "" (project-group list view) and ACL as
// [] (group-edit single view), plus stringified or native numeric ids.
func TestGroupCtrlWire_ToGroup_FullPayload_FromEdit(t *testing.T) {
	// Mirrors the inner.group object from group-edit-<id>.json GET on
	// the lek-ws ZenTao Max 8.x instance (probe-group-controller.md §2.3).
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
	var w groupCtrlWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	g, err := w.toGroup()
	if err != nil {
		t.Fatalf("toGroup: %v", err)
	}
	if g.ID != 10000002 {
		t.Fatalf("ID = %d", g.ID)
	}
	if g.Project != 28 {
		t.Fatalf("Project = %d", g.Project)
	}
	if g.Name != "tf-probe-pg-1778265940" {
		t.Fatalf("Name = %q", g.Name)
	}
	if g.Role != "" {
		t.Fatalf("Role = %q", g.Role)
	}
	if g.Desc != "tf-probe project group desc" {
		t.Fatalf("Desc = %q", g.Desc)
	}
	if g.Vision != "rnd" {
		t.Fatalf("Vision = %q", g.Vision)
	}
}

func TestGroupCtrlWire_ToGroup_AclAsString(t *testing.T) {
	// project-group-<projectID>.json list view returns acl as "" (string),
	// not [] (array). The wire RawMessage tolerates both.
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
	var w groupCtrlWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	g, err := w.toGroup()
	if err != nil {
		t.Fatalf("toGroup: %v", err)
	}
	if g.ID != 10000002 || g.Project != 28 {
		t.Fatalf("numerics decoded wrong: %+v", g)
	}
	if g.Role != "dev" {
		t.Fatalf("Role = %q", g.Role)
	}
}

func TestGroupCtrlWire_ToGroup_BadID(t *testing.T) {
	w := groupCtrlWire{ID: json.Number("not-a-number")}
	if _, err := w.toGroup(); err == nil {
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
		// data is stringified JSON containing inner.group (per probe §2.3)
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"x\",\"group\":{\"id\":7,\"project\":28,\"name\":\"alpha\",\"role\":\"dev\",\"desc\":\"d\",\"acl\":[],\"developer\":0,\"vision\":\"rnd\"},\"pager\":null}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	g, err := c.GetGroup(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if g.ID != 7 || g.Project != 28 || g.Name != "alpha" || g.Role != "dev" || g.Desc != "d" {
		t.Fatalf("group = %+v", g)
	}
}

func TestGetGroup_NotFound_NullInner(t *testing.T) {
	// Sole observed not-found shape from the probe (§2.3).
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

func TestCreateGroup_Happy_LookupAfterCreate_ProjectScoped(t *testing.T) {
	var hits int
	var createCT string
	var createForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			// create POST
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
			// project>0 → list via project-group-<projectID>.json
			if r.Method != http.MethodGet {
				t.Errorf("hit2 method = %s", r.Method)
			}
			if r.URL.Path != "/project-group-28.json" {
				t.Errorf("hit2 path = %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"groups\":[{\"id\":42,\"project\":28,\"name\":\"alpha\",\"role\":\"dev\",\"desc\":\"d\",\"acl\":\"\",\"developer\":0,\"vision\":\"rnd\",\"users\":\"\"}]}"}`))
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	g := &Group{Project: 28, Name: "alpha", Role: "dev", Desc: "d"}
	got, err := c.CreateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if got.ID != 42 {
		t.Fatalf("ID = %d, want 42 (resolved via list)", got.ID)
	}
	if got.Project != 28 || got.Name != "alpha" || got.Role != "dev" {
		t.Fatalf("got = %+v", got)
	}
	if hits != 2 {
		t.Fatalf("expected 2 server hits (create + list), got %d", hits)
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

func TestCreateGroup_Happy_LookupAfterCreate_SystemScoped(t *testing.T) {
	// project=0 must list via /group-browse.json (system view), not the
	// per-project endpoint. Project-scoped rows are not returned by
	// group-browse on Max 8.x — verified by probe.
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
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.CreateGroup(context.Background(), &Group{Project: 0, Name: "sysadmin", Role: "admin"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if got.ID != 99 {
		t.Fatalf("ID = %d, want 99", got.ID)
	}
	if got.Project != 0 {
		t.Fatalf("Project = %d, want 0", got.Project)
	}
}

func TestCreateGroup_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"name":["不能为空"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateGroup(context.Background(), &Group{Project: 28, Name: "x"})
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

func TestCreateGroup_LookupReturnsEmpty_Surfaced(t *testing.T) {
	// Create succeeded but list returns no match — extremely defensive,
	// mostly guards against name mismatch caused by server-side trimming.
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
	_, err := c.CreateGroup(context.Background(), &Group{Project: 28, Name: "alpha"})
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
		{"negative project", &Group{Project: -1, Name: "a"}},
		{"missing name", &Group{Project: 28}},
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

func TestUpdateGroup_Happy_RefetchesAfterPOST(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			if r.Method != http.MethodPost {
				t.Errorf("hit1 method = %s, want POST", r.Method)
			}
			if r.URL.Path != "/group-edit-7.json" {
				t.Errorf("hit1 path = %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"load":true,"closeModal":true,"result":"success","message":"保存成功"}`))
		case 2:
			if r.Method != http.MethodGet {
				t.Errorf("hit2 method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":7,\"project\":28,\"name\":\"renamed\",\"role\":\"\",\"desc\":\"updated\",\"acl\":[],\"developer\":0,\"vision\":\"rnd\"}}"}`))
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	g := &Group{ID: 7, Project: 28, Name: "renamed", Desc: "updated"}
	got, err := c.UpdateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if got.Name != "renamed" || got.Desc != "updated" {
		t.Fatalf("post-update = %+v", got)
	}
	if hits != 2 {
		t.Fatalf("expected 2 hits (POST + refetch), got %d", hits)
	}
}

func TestUpdateGroup_SilentNoOpDetection(t *testing.T) {
	// Critical probe finding (§0.2): POST to non-existent id returns the
	// same success envelope but the row doesn't exist. Re-read MUST
	// surface ErrNotFound.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			_, _ = w.Write([]byte(`{"load":true,"closeModal":true,"result":"success"}`))
		case 2:
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":null}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateGroup(context.Background(), &Group{ID: 99999, Project: 28, Name: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("silent no-op should surface as ErrNotFound, got %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected 2 hits, got %d", hits)
	}
}

func TestUpdateGroup_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"name is required"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateGroup(context.Background(), &Group{ID: 7, Project: 28, Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}

func TestUpdateGroup_PreflightRejection(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	if _, err := c.UpdateGroup(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil")
	}
	if _, err := c.UpdateGroup(context.Background(), &Group{Project: 28, Name: "x"}); err == nil {
		t.Fatal("expected error for ID=0")
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

// --- form helper ---

func TestGroupToForm_AppliesVisionDefault(t *testing.T) {
	g := &Group{Project: 28, Name: "g", Role: "", Desc: ""}
	form := groupToForm(g)
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

func TestGroupToForm_HonoursExplicitVision(t *testing.T) {
	g := &Group{Project: 28, Name: "g", Vision: "lite"}
	form := groupToForm(g)
	if form.Get("vision") != "lite" {
		t.Fatalf("explicit vision lost: %q", form.Get("vision"))
	}
}

func TestGroupToForm_SystemScopedSendsProjectZero(t *testing.T) {
	g := &Group{Project: 0, Name: "sysadmin", Role: "admin"}
	form := groupToForm(g)
	if form.Get("project") != "0" {
		t.Fatalf("project = %q, want 0 (system flavour)", form.Get("project"))
	}
}
