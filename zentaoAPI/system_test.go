package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// system-edit-{id} GET returns the full row inside the data wrapper. The
// wrapper must populate every field the resource / data source exposes,
// tolerating ZenTao's mixed int / quoted-number wire shapes.
func TestGetSystem_FullFieldSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system-edit-685.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"Edit\",\"system\":{\"id\":685,\"name\":\"app-x\",\"product\":1,\"integrated\":0,\"latestRelease\":0,\"latestDate\":\"\",\"children\":\"686,687\",\"status\":\"active\",\"desc\":\"a desc\",\"createdBy\":\"admin\",\"createdDate\":\"2026-05-22 15:36:56\",\"editedBy\":\"admin\",\"editedDate\":\"2026-05-22 19:32:37\",\"deleted\":\"0\"},\"systemList\":{}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.GetSystem(context.Background(), 685)
	if err != nil {
		t.Fatalf("GetSystem: %v", err)
	}
	want := &System{
		ID:          int64ptr(685),
		Product:     int64ptr(1),
		Name:        strptr("app-x"),
		Desc:        strptr("a desc"),
		Integrated:  int64ptr(0),
		Children:    strptr("686,687"),
		Status:      strptr("active"),
		CreatedBy:   strptr("admin"),
		CreatedDate: strptr("2026-05-22 15:36:56"),
		EditedBy:    strptr("admin"),
		EditedDate:  strptr("2026-05-22 19:32:37"),
		Deleted:     boolptr(false),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("System mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// A never-existed id returns HTTP 200 with `system:false`. Surface as
// ErrNotFound so Terraform Read clears the resource.
func TestGetSystem_NotFound_SystemFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"Edit\",\"system\":false,\"systemList\":{}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetSystem(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A soft-deleted row (deleted=1) is still returned by the GET endpoint;
// the wrapper must treat it as ErrNotFound — ZenTao has no hard delete.
func TestGetSystem_NotFound_DeletedTombstone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"system\":{\"id\":685,\"name\":\"app-x\",\"product\":1,\"status\":\"active\",\"deleted\":\"1\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetSystem(context.Background(), 685)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (deleted tombstone)", err)
	}
}

// CreateSystem posts to system-create-{productID}, then — because the
// create response carries no id — discovers the id via system-showAll
// filtered by name+product, and refetches the full row.
func TestCreateSystem_Happy_CreateLookupRefetch(t *testing.T) {
	var createPath string
	var createBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/system-create-") && r.Method == http.MethodPost:
			createPath = r.URL.Path
			createBody = readAllString(r.Body)
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"保存成功"}`))
		case r.URL.Path == "/system-showAll.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"appList\":{\"685\":{\"id\":685,\"name\":\"app-x\",\"product\":1,\"integrated\":0,\"children\":\"\",\"status\":\"active\",\"desc\":\"\",\"deleted\":0}}}"}`))
		case r.URL.Path == "/system-edit-685.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"system\":{\"id\":685,\"name\":\"app-x\",\"product\":1,\"integrated\":0,\"children\":\"\",\"status\":\"active\",\"desc\":\"hi\",\"createdBy\":\"admin\",\"createdDate\":\"2026-05-22 15:36:56\",\"deleted\":0}}"}`))
		default:
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	got, err := c.CreateSystem(context.Background(), &System{Product: int64ptr(1), Name: strptr("app-x"), Desc: strptr("hi")})
	if err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}
	if createPath != "/system-create-1.json" {
		t.Errorf("create path = %q, want /system-create-1.json (productID is URL arg)", createPath)
	}
	if got.ID == nil || *got.ID != 685 {
		t.Errorf("returned id = %v, want 685", got.ID)
	}
	form, _ := url.ParseQuery(createBody)
	if form.Get("name") != "app-x" {
		t.Errorf("create form name = %q, want app-x", form.Get("name"))
	}
}

func TestCreateSystem_Validation(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name string
		in   *System
	}{
		{"nil", nil},
		{"no name", &System{Product: int64ptr(1)}},
		{"no product", &System{Name: strptr("x")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.CreateSystem(context.Background(), tc.in); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// UpdateSystem must M-Z-merge: an input with Children nil preserves the
// baseline's children list — edit-POST resets any omitted form key, and
// children is owned by the attachment resource, not this Update.
func TestUpdateSystem_PreservesChildrenWhenInputNil(t *testing.T) {
	var editBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"system\":{\"id\":685,\"name\":\"old\",\"product\":1,\"children\":\"686,687\",\"status\":\"active\",\"desc\":\"d\",\"deleted\":0}}"}`))
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodPost:
			editBody = readAllString(r.Body)
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"保存成功"}`))
		default:
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateSystem(context.Background(), &System{ID: int64ptr(685), Name: strptr("new-name")})
	if err != nil {
		t.Fatalf("UpdateSystem: %v", err)
	}
	form, _ := url.ParseQuery(editBody)
	if form.Get("name") != "new-name" {
		t.Errorf("edit form name = %q, want new-name", form.Get("name"))
	}
	children := form["children[]"]
	if !reflect.DeepEqual(children, []string{"686", "687"}) {
		t.Errorf("edit form children[] = %v, want [686 687] (baseline preserved)", children)
	}
}

// Changing status routes to the dedicated active/inactive endpoint, not
// the edit form (status is not an edit-form key).
func TestUpdateSystem_StatusChange_CallsInactiveEndpoint(t *testing.T) {
	var hitInactive bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"system\":{\"id\":685,\"name\":\"app\",\"product\":1,\"children\":\"\",\"status\":\"active\",\"desc\":\"\",\"deleted\":0}}"}`))
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"ok"}`))
		case r.URL.Path == "/system-inactive-685.json" && r.Method == http.MethodPost:
			hitInactive = true
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"ok"}`))
		default:
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateSystem(context.Background(), &System{ID: int64ptr(685), Name: strptr("app"), Status: strptr("inactive")})
	if err != nil {
		t.Fatalf("UpdateSystem: %v", err)
	}
	if !hitInactive {
		t.Error("expected system-inactive-685.json to be called for status active->inactive")
	}
}

// When status is unchanged from baseline, the active/inactive endpoint
// must NOT be called.
func TestUpdateSystem_StatusUnchanged_NoToggle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"system\":{\"id\":685,\"name\":\"app\",\"product\":1,\"children\":\"\",\"status\":\"active\",\"desc\":\"\",\"deleted\":0}}"}`))
		case r.URL.Path == "/system-edit-685.json" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"ok"}`))
		case strings.Contains(r.URL.Path, "active"):
			t.Errorf("status toggle endpoint must not be called: %s", r.URL.Path)
		default:
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if _, err := c.UpdateSystem(context.Background(), &System{ID: int64ptr(685), Name: strptr("app"), Status: strptr("active")}); err != nil {
		t.Fatalf("UpdateSystem: %v", err)
	}
}

func TestSetSystemStatus_Routing(t *testing.T) {
	cases := []struct {
		status   string
		wantPath string
		wantErr  bool
	}{
		{"active", "/system-active-685.json", false},
		{"inactive", "/system-inactive-685.json", false},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"ok"}`))
			}))
			defer srv.Close()
			c := newTestClient(t, "tok-1", srv.URL)
			err := c.SetSystemStatus(context.Background(), 685, tc.status)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for bogus status")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetSystemStatus: %v", err)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
		})
	}
}

// Delete is idempotent: a second delete (or delete of a missing row)
// still returns result:success and must surface as nil.
func TestDeleteSystem_Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system-delete-685.json" || r.Method != http.MethodPost {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"load":true,"result":"success","message":"保存成功"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteSystem(context.Background(), 685); err != nil {
		t.Fatalf("DeleteSystem: %v", err)
	}
}

// toForm splits the comma-string children baseline into repeated
// children[] keys (the array wire shape ZenTao expects), and emits the
// writeable form fields.
func TestSystemToForm_ChildrenSplit(t *testing.T) {
	s := &System{Name: strptr("app"), Desc: strptr("d"), Children: strptr("686,687")}
	form := s.toForm()
	if form.Get("name") != "app" || form.Get("desc") != "d" {
		t.Errorf("name/desc = %q/%q", form.Get("name"), form.Get("desc"))
	}
	if !reflect.DeepEqual(form["children[]"], []string{"686", "687"}) {
		t.Errorf("children[] = %v, want [686 687]", form["children[]"])
	}
}

func TestSystemToForm_EmptyChildren_NoKey(t *testing.T) {
	s := &System{Name: strptr("app"), Children: strptr("")}
	if got := s.toForm()["children[]"]; got != nil {
		t.Errorf("children[] = %v, want nil for empty children", got)
	}
}

// Exercise UnmarshalJSON's number tolerance for the showAll row shape
// (id/product/integrated as bare ints, deleted as quoted string).
func TestSystem_UnmarshalJSON_NumberTolerance(t *testing.T) {
	var s System
	if err := json.Unmarshal([]byte(`{"id":"685","product":2,"integrated":"1","deleted":1,"name":"x"}`), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if deref(s.ID) != 685 || deref(s.Product) != 2 || deref(s.Integrated) != 1 || !deref(s.Deleted) {
		t.Errorf("got id=%v product=%v integrated=%v deleted=%v", deref(s.ID), deref(s.Product), deref(s.Integrated), deref(s.Deleted))
	}
}
