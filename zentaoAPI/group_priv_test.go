package zentaoapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// groupEditBody returns a group-edit-<id>.json GET envelope whose inner
// group row carries the given project column. Both GetGroupPrivs and
// SetGroupPrivs lean on GetGroup for the project (and the not-found signal).
func groupEditBody(project string) string {
	return `{"status":"success","data":"{\"title\":\"x\",\"group\":{\"id\":7,\"project\":` +
		project + `,\"name\":\"g\",\"role\":\"\",\"desc\":\"\",\"vision\":\"rnd\"},\"pager\":null}"}`
}

const groupEditNullBody = `{"status":"success","data":"{\"title\":\"x\",\"group\":null,\"pager\":null}"}`

// --- GetGroupPrivs ---

func TestGetGroupPrivs_Happy_ProjectScopedPath(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			if r.URL.Path != "/group-edit-7.json" {
				t.Errorf("hit1 path = %s, want /group-edit-7.json", r.URL.Path)
			}
			_, _ = w.Write([]byte(groupEditBody("28")))
		case 2:
			if r.Method != http.MethodGet {
				t.Errorf("hit2 method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/project-managePriv-28-7.json" {
				t.Errorf("hit2 path = %s, want /project-managePriv-28-7.json", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"selectedPrivList\":[\"story-view\",\"story-tasks\"],\"allPrivList\":[\"story-view\",\"story-tasks\",\"story-bugs\"]}"}`))
		default:
			t.Errorf("unexpected hit %d to %s", hits, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.GetGroupPrivs(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetGroupPrivs: %v", err)
	}
	want := []string{"story-view", "story-tasks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("privs = %v, want %v", got, want)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
}

// System groups (project=0) reach managePriv via project id 0 — the
// group-managePriv JSON surface is dead on Max 8.x (probe §1).
func TestGetGroupPrivs_SystemGroup_ProjectZeroPath(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			_, _ = w.Write([]byte(groupEditBody("0")))
		case 2:
			if r.URL.Path != "/project-managePriv-0-7.json" {
				t.Errorf("hit2 path = %s, want /project-managePriv-0-7.json", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"selectedPrivList\":[]}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.GetGroupPrivs(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetGroupPrivs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("privs = %v, want empty", got)
	}
}

// managePriv never 404s; not-found must come from GetGroup, before any
// managePriv round trip.
func TestGetGroupPrivs_GroupNotFound_NoManagePrivCall(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits > 1 {
			t.Errorf("managePriv must not be called after group not-found (hit %d)", hits)
		}
		_, _ = w.Write([]byte(groupEditNullBody))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroupPrivs(context.Background(), 7)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

func TestGetGroupPrivs_EnvelopeFail(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(groupEditBody("28")))
			return
		}
		_, _ = w.Write([]byte(`{"status":"fail","message":"insufficient privs"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetGroupPrivs(context.Background(), 7)
	if err == nil {
		t.Fatal("err = nil, want APIError")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}

// --- SetGroupPrivs ---

func TestSetGroupPrivs_BodyShape_ProjectScoped(t *testing.T) {
	var hits int
	var gotForm url.Values
	var gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(groupEditBody("28")))
			return
		}
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = w.Write([]byte(`{"result":"success","message":"保存成功"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.SetGroupPrivs(context.Background(), 7, []string{"story-view", "story-tasks"})
	if err != nil {
		t.Fatalf("SetGroupPrivs: %v", err)
	}
	if gotPath != "/project-managePriv-28-7.json" {
		t.Errorf("path = %s, want /project-managePriv-28-7.json", gotPath)
	}
	if !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Errorf("content-type = %s, want form-urlencoded", gotCT)
	}
	if gotForm.Get("noChecked") != "1" {
		t.Errorf("noChecked = %q, want 1 (or save degrades to a form re-render)", gotForm.Get("noChecked"))
	}
	methods := gotForm["actions[story][]"]
	if !reflect.DeepEqual(methods, []string{"view", "tasks"}) {
		t.Errorf("actions[story][] = %v, want [view tasks]", methods)
	}
}

// Empty set still POSTs noChecked=1 (clears) — never an empty body, which
// the server treats as "render form", not "save".
func TestSetGroupPrivs_EmptyClears(t *testing.T) {
	var hits int
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(groupEditBody("28")))
			return
		}
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = w.Write([]byte(`{"result":"success","message":"保存成功"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.SetGroupPrivs(context.Background(), 7, nil); err != nil {
		t.Fatalf("SetGroupPrivs: %v", err)
	}
	if gotForm.Get("noChecked") != "1" {
		t.Errorf("noChecked = %q, want 1", gotForm.Get("noChecked"))
	}
	for k := range gotForm {
		if strings.HasPrefix(k, "actions[") {
			t.Errorf("unexpected actions key %q on empty clear", k)
		}
	}
}

// Format validation is zero-cost and runs before any network call.
func TestSetGroupPrivs_InvalidFormat_NoNetwork(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	for _, bad := range []string{"noseparator", "-view", "story-", ""} {
		err := c.SetGroupPrivs(context.Background(), 7, []string{bad})
		if err == nil {
			t.Errorf("priv %q: err = nil, want format error", bad)
		}
	}
	if hits != 0 {
		t.Fatalf("hits = %d, want 0 (validation must precede any round trip)", hits)
	}
}

// A priv whose method contains a hyphen is split on the FIRST hyphen only.
func TestSetGroupPrivs_SplitsOnFirstHyphen(t *testing.T) {
	var hits int
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(groupEditBody("28")))
			return
		}
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = w.Write([]byte(`{"result":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.SetGroupPrivs(context.Background(), 7, []string{"my-multi-word"}); err != nil {
		t.Fatalf("SetGroupPrivs: %v", err)
	}
	if got := gotForm["actions[my][]"]; !reflect.DeepEqual(got, []string{"multi-word"}) {
		t.Errorf("actions[my][] = %v, want [multi-word]", got)
	}
}

func TestSetGroupPrivs_FailEnvelope(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			_, _ = w.Write([]byte(groupEditBody("28")))
			return
		}
		_, _ = w.Write([]byte(`{"result":"fail","message":"insufficient privs"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.SetGroupPrivs(context.Background(), 7, []string{"story-view"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}

func TestSetGroupPrivs_GroupNotFound_NoPost(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits > 1 {
			t.Errorf("must not POST after group not-found (hit %d)", hits)
		}
		_, _ = w.Write([]byte(groupEditNullBody))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.SetGroupPrivs(context.Background(), 7, []string{"story-view"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}
