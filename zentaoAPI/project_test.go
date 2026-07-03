package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestProject_UnmarshalJSON_FullPayload(t *testing.T) {
	// Mirror probe-project-controller.md §3 — inner `.project` object.
	raw := []byte(`{
		"id":28,"name":"Alpha","model":"scrum","type":"project",
		"begin":"2026-05-09","end":"2099-12-31",
		"parent":4,"workflowGroup":2,"multiple":1,
		"acl":"private","auth":"reset","PM":"pm",
		"storyType":"story,requirement","taskDateLimit":"limit",
		"desc":"d","deleted":0
	}`)
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deref(p.ID) != 28 || deref(p.Name) != "Alpha" || deref(p.Model) != "scrum" {
		t.Fatalf("id/name/model = %v/%v/%v", deref(p.ID), deref(p.Name), deref(p.Model))
	}
	if deref(p.Parent) != 4 || deref(p.WorkflowGroup) != 2 {
		t.Fatalf("parent/workflowGroup = %v/%v", deref(p.Parent), deref(p.WorkflowGroup))
	}
	if !deref(p.Multiple) {
		t.Fatalf("Multiple = %v, want true", deref(p.Multiple))
	}
	if deref(p.Deleted) {
		t.Fatalf("Deleted = %v, want false", deref(p.Deleted))
	}
	if deref(p.Auth) != "reset" {
		t.Fatalf("Auth = %q, want reset", deref(p.Auth))
	}
	if deref(p.TaskDateLimit) != "limit" {
		t.Fatalf("TaskDateLimit = %q, want limit", deref(p.TaskDateLimit))
	}
	// Wire carries storyType comma-joined; the decoder splits it.
	if !reflect.DeepEqual(deref(p.StoryTypes), []string{"story", "requirement"}) {
		t.Fatalf("StoryTypes = %v, want [story requirement]", deref(p.StoryTypes))
	}
}

// zt_project rows can hold storyType=” (observed on live Max 8.x, see
// probe-project-controller.md). The decoder must map that to a non-nil
// empty slice — distinct from "field absent" (nil).
func TestProject_UnmarshalJSON_EmptyStoryType(t *testing.T) {
	var p Project
	if err := json.Unmarshal([]byte(`{"id":28,"storyType":""}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.StoryTypes == nil {
		t.Fatalf("StoryType must be non-nil empty slice for wire \"\"")
	}
	if len(*p.StoryTypes) != 0 {
		t.Fatalf("StoryType = %v, want empty", *p.StoryTypes)
	}
}

func TestProject_UnmarshalJSON_StringNumbersOK(t *testing.T) {
	// Some ZenTao Max versions return numeric columns as quoted strings.
	raw := []byte(`{"id":"28","parent":"4","workflowGroup":"2","multiple":"1","deleted":"0"}`)
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deref(p.ID) != 28 || deref(p.Parent) != 4 || deref(p.WorkflowGroup) != 2 {
		t.Fatalf("got %+v", p)
	}
	if !deref(p.Multiple) || deref(p.Deleted) {
		t.Fatalf("bool decode wrong: multiple=%v deleted=%v", deref(p.Multiple), deref(p.Deleted))
	}
}

func TestProject_UnmarshalJSON_AbsentFieldsStayNil(t *testing.T) {
	raw := []byte(`{"id":28,"name":"Alpha"}`)
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Multiple != nil || p.Deleted != nil || p.Parent != nil || p.WorkflowGroup != nil {
		t.Fatalf("absent fields must stay nil: %+v", p)
	}
	if p.StoryTypes != nil || p.TaskDateLimit != nil {
		t.Fatalf("absent storyType/taskDateLimit must stay nil: %+v", p)
	}
}

func TestGetProject_HappyPath_RowAndProducts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project-view-28.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// .data is a JSON-encoded string carrying both .project (row) and
		// .products (id-keyed map of joined product rows). Wrapper splices
		// product ids into Project.Products.
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":28,\"name\":\"LB-Maint\",\"model\":\"scrum\",\"type\":\"project\",\"begin\":\"2026-05-09\",\"end\":\"2099-12-31\",\"parent\":4,\"workflowGroup\":2,\"multiple\":0,\"acl\":\"private\",\"PM\":\"pm\",\"desc\":\"\",\"deleted\":0},\"products\":{\"3\":{\"id\":3,\"name\":\"p3\"},\"1\":{\"id\":1,\"name\":\"p1\"}}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProject(context.Background(), 28)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if deref(p.ID) != 28 || deref(p.Name) != "LB-Maint" {
		t.Fatalf("id/name = %v/%v", deref(p.ID), deref(p.Name))
	}
	// Products keys must come back sorted ascending — map iteration order
	// is non-deterministic, but the wrapper sorts for stable plan diffs.
	if !reflect.DeepEqual(deref(p.Products), []int64{1, 3}) {
		t.Fatalf("Products = %v, want [1 3] sorted", deref(p.Products))
	}
}

// htmlspecialchars(ENT_QUOTES) on the server side encodes a written
// apostrophe to `&#039;`; GetProject must decode it so Terraform's
// round-trip stays consistent (see TestGetProduct_DecodesHTMLEntitiesInName).
func TestGetProject_DecodesHTMLEntitiesInName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":28,\"name\":\"Werewolf&#039;s Hunt &amp; &lt;tag&gt;\"},\"products\":{}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProject(context.Background(), 28)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	want := "Werewolf's Hunt & <tag>"
	if deref(p.Name) != want {
		t.Fatalf("Name = %q, want %q", deref(p.Name), want)
	}
}

func TestGetProject_NoProductsLinked(t *testing.T) {
	// project-view on a project with no linked products → .data.products
	// is an empty object. Wrapper should produce a non-nil empty slice.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":50,\"name\":\"NoProd\",\"type\":\"project\"},\"products\":{}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProject(context.Background(), 50)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Products == nil {
		t.Fatalf("Products pointer is nil, want non-nil empty slice")
	}
	if len(*p.Products) != 0 {
		t.Fatalf("Products = %v, want empty", *p.Products)
	}
}

// project-view's missing-id reply has neither `status` nor `data` —
// just `{"result":"success","load":{"alert":"...","locate":"..."}}`. The
// wrapper sniffs the missing fields and surfaces ErrNotFound. See
// probe-project-controller.md §4.
func TestGetProject_MissingID_LoadAlertEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","load":{"alert":"您无权访问该项目！","locate":"/zentao/project-browse.json"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (missing-id alert)", err)
	}
}

// Soft-deleted rows return result:fail + load.alert. Same shape signature
// as missing-id (no status, no data), same ErrNotFound result.
func TestGetProject_SoftDeleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","load":{"alert":"抱歉，您访问的项目已被删除。","locate":"/zentao/project-browse.json"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 70)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (soft-deleted alert)", err)
	}
}

func TestGetProject_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProject_ProjectFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":false}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (project:false)", err)
	}
}

func TestGetProject_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"db down","data":null}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 5)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Reason != "db down" {
		t.Fatalf("reason = %q", apiErr.Reason)
	}
}

func TestGetProject_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

// CreateProject POSTs a form to project-create.json; response carries
// id inline; wrapper refetches via GET.
func TestCreateProject_BodyShapeAndRefetch(t *testing.T) {
	var posts, gets int
	var postCT, postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/project-create.json":
			posts++
			postCT = r.Header.Get("Content-Type")
			buf := make([]byte, 8192)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","id":77}`))
		case r.Method == http.MethodGet && r.URL.Path == "/project-view-77.json":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":77,\"name\":\"Beta\",\"model\":\"scrum\",\"type\":\"project\",\"begin\":\"2026-06-01\",\"end\":\"2026-12-31\",\"parent\":1,\"workflowGroup\":2,\"multiple\":1,\"acl\":\"open\",\"PM\":\"pm\",\"deleted\":0},\"products\":{\"1\":{\"id\":1}}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Project{
		Name:          strptr("Beta"),
		Model:         strptr("scrum"),
		Begin:         strptr("2026-06-01"),
		End:           strptr("2026-12-31"),
		Parent:        int64ptr(1),
		WorkflowGroup: int64ptr(2),
		Multiple:      boolptr(true),
		ACL:           strptr("open"),
		PM:            strptr("pm"),
		Desc:          strptr("d"),
		Products:      &[]int64{1, 2},
	}
	out, err := c.CreateProject(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if deref(out.ID) != 77 {
		t.Fatalf("id = %d, want 77", deref(out.ID))
	}
	if posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d, want 1/1", posts, gets)
	}
	if !strings.HasPrefix(postCT, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q, want form-urlencoded", postCT)
	}
	for _, mustSend := range []string{
		"name=Beta", "model=scrum", "begin=2026-06-01", "end=2026-12-31",
		"parent=1", "workflowGroup=2", "acl=open", "PM=pm", "desc=d",
	} {
		if !strings.Contains(postBody, mustSend) {
			t.Errorf("body missing %q: %s", mustSend, postBody)
		}
	}
	// multiple=on (HTML checkbox semantics, not 1/0/yes)
	if !strings.Contains(postBody, "multiple=on") {
		t.Errorf("multiple must serialize as 'on' (true), got body=%s", postBody)
	}
	// products[]=1&products[]=2 (PHP-array form, URL-encoded as %5B%5D)
	if !strings.Contains(postBody, "products%5B%5D=1") || !strings.Contains(postBody, "products%5B%5D=2") {
		t.Errorf("products must serialize as products[]=N pairs, got body=%s", postBody)
	}
}

func TestCreateProject_MultipleFalseSerializesOff(t *testing.T) {
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			buf := make([]byte, 8192)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","id":3}`))
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":3},\"products\":{}}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Project{
		Name: strptr("x"), Model: strptr("scrum"),
		Begin: strptr("2026-01-01"), End: strptr("2026-12-31"),
		WorkflowGroup: int64ptr(2), ACL: strptr("open"),
		Products: &[]int64{1},
		Multiple: boolptr(false),
	}
	if _, err := c.CreateProject(context.Background(), in); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if !strings.Contains(postBody, "multiple=off") {
		t.Errorf("multiple=false must serialize as 'off', got body=%s", postBody)
	}
}

func TestCreateProject_PreflightValidation(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	full := func() *Project {
		return &Project{
			Name: strptr("x"), Model: strptr("scrum"),
			Begin: strptr("a"), End: strptr("b"),
			WorkflowGroup: int64ptr(2), ACL: strptr("open"),
			Products: &[]int64{1},
		}
	}
	cases := []struct {
		name    string
		mutate  func(p *Project)
		wantSub string
	}{
		{"nil", func(p *Project) {}, ""},
		{"missing name", func(p *Project) { p.Name = nil }, "name required"},
		{"missing begin", func(p *Project) { p.Begin = nil }, "begin required"},
		{"missing end", func(p *Project) { p.End = nil }, "end required"},
		{"missing model", func(p *Project) { p.Model = nil }, "model required"},
		{"missing workflowGroup", func(p *Project) { p.WorkflowGroup = nil }, "workflowGroup required"},
		{"missing acl", func(p *Project) { p.ACL = nil }, "acl required"},
		{"empty products", func(p *Project) { p.Products = &[]int64{} }, "product required"},
		{"nil products", func(p *Project) { p.Products = nil }, "product required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in *Project
			if tc.name != "nil" {
				in = full()
				tc.mutate(in)
			}
			_, err := c.CreateProject(context.Background(), in)
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestCreateProject_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","id":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProject(context.Background(), &Project{
		Name: strptr("x"), Model: strptr("scrum"),
		Begin: strptr("a"), End: strptr("b"),
		WorkflowGroup: int64ptr(2), ACL: strptr("open"),
		Products: &[]int64{1},
	})
	if err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("err = %v, want empty-id error", err)
	}
}

// Project validation messages come in heterogeneous shapes. The flat
// per-field-string variant `{"end":"..."}` must round-trip through the
// extended FieldErrors() and surface as a readable *APIError reason.
func TestCreateProject_FailEnvelope_FlatStringMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"end":"『计划完成』不能为空。"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProject(context.Background(), &Project{
		Name: strptr("x"), Model: strptr("scrum"),
		Begin: strptr("a"), End: strptr("b"),
		WorkflowGroup: int64ptr(2), ACL: strptr("open"),
		Products: &[]int64{1},
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Reason, "end:") {
		t.Fatalf("reason should mention end, got %q", apiErr.Reason)
	}
}

// UpdateProject does GET-baseline → POST-merged → GET-refetch. Verifies
// that fields not touched by the caller are preserved from the baseline.
func TestUpdateProject_BaselineMergeThenRefetch(t *testing.T) {
	var posts, gets int
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/project-edit-5.json":
			posts++
			buf := make([]byte, 8192)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/project-view-5.json":
			gets++
			w.Header().Set("Content-Type", "application/json")
			// Baseline: model=scrum, parent=4, wfg=2, multiple=true,
			// PM=alice, products=[1,2] — none of which input touches.
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"project\":{\"id\":5,\"name\":\"OldName\",\"model\":\"scrum\",\"type\":\"project\",\"begin\":\"2026-01-01\",\"end\":\"2026-12-31\",\"parent\":4,\"workflowGroup\":2,\"multiple\":1,\"acl\":\"private\",\"PM\":\"alice\",\"storyType\":\"story,requirement\",\"taskDateLimit\":\"limit\",\"desc\":\"old\",\"deleted\":0},\"products\":{\"1\":{\"id\":1},\"2\":{\"id\":2}}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProject(context.Background(), &Project{ID: int64ptr(5), Name: strptr("NewName")})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if posts != 1 || gets != 2 {
		t.Fatalf("posts=%d gets=%d, want 1/2", posts, gets)
	}
	if !strings.Contains(postBody, "name=NewName") {
		t.Errorf("POST body missing name override: %s", postBody)
	}
	for _, mustPreserve := range []string{
		"model=scrum", "parent=4", "workflowGroup=2",
		"acl=private", "PM=alice",
		"taskDateLimit=limit",
		"storyType%5B%5D=story", "storyType%5B%5D=requirement",
		"desc=old", "multiple=on", // multiple=true → 'on'
	} {
		if !strings.Contains(postBody, mustPreserve) {
			t.Errorf("POST body missing baseline preservation %q: %s", mustPreserve, postBody)
		}
	}
	// products[] must replay baseline list
	if !strings.Contains(postBody, "products%5B%5D=1") || !strings.Contains(postBody, "products%5B%5D=2") {
		t.Errorf("POST body must preserve baseline products: %s", postBody)
	}
	if deref(out.ID) != 5 {
		t.Fatalf("got %+v", out)
	}
}

// Multiple is create-only: even if the caller sets it on an update,
// mergeProjectBaseline must drop the override and replay baseline.
// Otherwise the wire would carry a value ZenTao silently ignores anyway,
// but the in-memory merge result would be misleading.
func TestUpdateProject_MultipleNotOverridable(t *testing.T) {
	baseline := &Project{ID: int64ptr(5), Multiple: boolptr(true)}
	input := &Project{ID: int64ptr(5), Multiple: boolptr(false)}
	merged := mergeProjectBaseline(input, baseline)
	if !deref(merged.Multiple) {
		t.Fatalf("Multiple = %v, want baseline (true) — input override must be dropped", deref(merged.Multiple))
	}
}

func TestUpdateProject_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProject(context.Background(), &Project{Name: strptr("x")})
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("err = %v, want missing id error", err)
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Baseline GET returns missing-id envelope → propagates as ErrNotFound.
		_, _ = w.Write([]byte(`{"result":"success","load":{"alert":"您无权访问该项目！"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateProject(context.Background(), &Project{ID: int64ptr(99), Name: strptr("x")})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteProject_Success(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","closeModal":true,"load":"/zentao/project-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	// project-delete does NOT take the positional `-yes` suffix that
	// program-delete requires; bare form is enough.
	if gotMethod != http.MethodGet || gotPath != "/project-delete-9.json" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteProject_HTTP404IsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProject should be idempotent on 404: %v", err)
	}
}

func TestDeleteProject_MissingIDIsIdempotent(t *testing.T) {
	// Live ZenTao returns success for both missing-id and already-deleted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","closeModal":true,"load":"/zentao/project-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 99999); err != nil {
		t.Fatalf("DeleteProject should be idempotent on missing id: %v", err)
	}
}

func TestDeleteProject_NotExistMessageIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"Project does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProject should be idempotent on does-not-exist: %v", err)
	}
}

func TestDeleteProject_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteProject(context.Background(), 9)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectToForm_AlwaysSetsAllWriteableFields(t *testing.T) {
	form := (&Project{Name: strptr("x")}).toForm()
	for _, k := range []string{"name", "model", "begin", "end", "parent", "workflowGroup", "multiple", "acl", "auth", "PM", "desc", "deleted"} {
		if _, ok := form[k]; !ok {
			t.Errorf("form must always carry key %q (always-set rule): %v", k, form)
		}
	}
	// zero-valued ints must be explicit "0", not omitted (form is non-PATCH).
	if form.Get("parent") != "0" || form.Get("workflowGroup") != "0" {
		t.Errorf("zero-valued ints must be \"0\": parent=%q wfg=%q", form.Get("parent"), form.Get("workflowGroup"))
	}
	// nil Multiple → boolToOnOff(false) → "off"
	if form.Get("multiple") != "off" {
		t.Errorf("nil Multiple should default to 'off', got %q", form.Get("multiple"))
	}
	// products[] empty placeholder
	if _, ok := form["products[]"]; !ok {
		t.Errorf("products[] must always be present (empty placeholder), got %v", form)
	}
}

func TestProjectToForm_MultipleOnOffSerialization(t *testing.T) {
	cases := []struct {
		name string
		mult *bool
		want string
	}{
		{"nil → off", nil, "off"},
		{"false → off", boolptr(false), "off"},
		{"true → on", boolptr(true), "on"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := (&Project{Multiple: tc.mult}).toForm()
			if got := form.Get("multiple"); got != tc.want {
				t.Errorf("multiple=%v → %q, want %q", tc.mult, got, tc.want)
			}
		})
	}
}

// taskDateLimit / storyType[] are conditional form fields: when the caller
// (or M-Z baseline) has no value, they are omitted so project-create falls
// back to the server defaults (auto / story). Edits always carry them
// because the baseline read is never nil.
func TestProjectToForm_TaskDateLimitAndStoryTypeConditional(t *testing.T) {
	form := (&Project{Name: strptr("x")}).toForm()
	if _, ok := form["taskDateLimit"]; ok {
		t.Errorf("nil TaskDateLimit must omit the key, got %v", form)
	}
	if _, ok := form["storyType[]"]; ok {
		t.Errorf("nil StoryType must omit the key, got %v", form)
	}

	form = (&Project{
		TaskDateLimit: strptr("limit"),
		StoryTypes:     &[]string{"story", "requirement"},
	}).toForm()
	if got := form.Get("taskDateLimit"); got != "limit" {
		t.Errorf("taskDateLimit = %q, want limit", got)
	}
	if !reflect.DeepEqual(form["storyType[]"], []string{"story", "requirement"}) {
		t.Errorf("storyType[] = %v, want [story requirement]", form["storyType[]"])
	}

	// empty slice / empty string behave like nil — omitted, server keeps its value
	form = (&Project{TaskDateLimit: strptr(""), StoryTypes: &[]string{}}).toForm()
	if _, ok := form["taskDateLimit"]; ok {
		t.Errorf("empty TaskDateLimit must omit the key")
	}
	if _, ok := form["storyType[]"]; ok {
		t.Errorf("empty StoryType must omit the key")
	}
}

func TestProjectToForm_ProductsArrayExpansion(t *testing.T) {
	form := (&Project{Products: &[]int64{3, 1, 2}}).toForm()
	got := form["products[]"]
	want := []string{"3", "1", "2"} // toForm preserves caller order — sorting happens on decode
	if !reflect.DeepEqual(got, want) {
		t.Errorf("products[] = %v, want %v", got, want)
	}
}

func TestMergeProjectBaseline_PreservesBaselineWhenInputNil(t *testing.T) {
	baseline := &Project{
		ID: int64ptr(5), Name: strptr("Base"), Model: strptr("scrum"),
		Begin: strptr("2026-01-01"), End: strptr("2026-12-31"),
		Parent: int64ptr(3), WorkflowGroup: int64ptr(2),
		Multiple: boolptr(true), ACL: strptr("private"),
		Auth:          strptr("extend"),
		PM:            strptr("alice"),
		TaskDateLimit: strptr("auto"),
		StoryTypes:     &[]string{"story"},
		Desc:          strptr("old"), Deleted: boolptr(false),
		Products: &[]int64{1, 2},
	}
	merged := mergeProjectBaseline(&Project{
		ID: int64ptr(5), Name: strptr("NewName"), Auth: strptr("reset"),
		TaskDateLimit: strptr("limit"),
		StoryTypes:     &[]string{"story", "epic"},
	}, baseline)
	if deref(merged.Name) != "NewName" {
		t.Errorf("Name override failed: %q", deref(merged.Name))
	}
	if deref(merged.Auth) != "reset" {
		t.Errorf("Auth override failed: %q", deref(merged.Auth))
	}
	if deref(merged.TaskDateLimit) != "limit" {
		t.Errorf("TaskDateLimit override failed: %q", deref(merged.TaskDateLimit))
	}
	if !reflect.DeepEqual(deref(merged.StoryTypes), []string{"story", "epic"}) {
		t.Errorf("StoryType override failed: %v", deref(merged.StoryTypes))
	}
	// Every other field comes from baseline.
	if deref(merged.Model) != "scrum" || deref(merged.Parent) != 3 || deref(merged.WorkflowGroup) != 2 {
		t.Errorf("baseline scalars not preserved: %+v", merged)
	}
	if deref(merged.PM) != "alice" || deref(merged.Desc) != "old" {
		t.Errorf("baseline string fields not preserved: %+v", merged)
	}
	if !reflect.DeepEqual(deref(merged.Products), []int64{1, 2}) {
		t.Errorf("baseline products not preserved: %v", deref(merged.Products))
	}
	if !deref(merged.Multiple) {
		t.Errorf("baseline Multiple lost: %v", deref(merged.Multiple))
	}
}
