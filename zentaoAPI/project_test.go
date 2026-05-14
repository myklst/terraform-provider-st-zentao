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

func TestGetProject_FullFieldSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != projectsPath+"/95" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Trimmed-down version of the real GET body (probe-project-v2.md §5):
		// keeps every field this client surfaces, drops the long tail of
		// internals we deliberately skip.
		_, _ = w.Write([]byte(`{
			"status":"success",
			"project":{
				"id":"95","name":"Alpha","code":"alpha-code",
				"model":"scrum","type":"project",
				"begin":"2026-05-09","end":"2026-12-31",
				"parent":"7","workflowGroup":"2",
				"status":"wait","acl":"private",
				"PM":"pm-user","PO":"po-user","QD":"qd-user","RD":"rd-user",
				"desc":"a desc","lifetime":"",
				"openedBy":"admin","openedDate":"2026-05-09 02:00:29",
				"lastEditedBy":"admin",
				"realBegan":"","realEnd":"",
				"progress":"0.00","teamCount":"1",
				"budget":"0.00","budgetUnit":"CNY"
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProject(context.Background(), 95)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	want := &Project{
		ID:            95,
		Name:          "Alpha",
		Code:          "alpha-code",
		Model:         "scrum",
		Type:          "project",
		Begin:         "2026-05-09",
		End:           "2026-12-31",
		Parent:        7,
		WorkflowGroup: 2,
		Status:        "wait",
		ACL:           "private",
		PM:            "pm-user",
		PO:            "po-user",
		QD:            "qd-user",
		RD:            "rd-user",
		Desc:          "a desc",
		Lifetime:      "",
		OpenedBy:      "admin",
		OpenedDate:    "2026-05-09 02:00:29",
		LastEditedBy:  "admin",
		RealBegan:     "",
		RealEnd:       "",
		Progress:      "0.00",
		TeamCount:     "1",
		Budget:        "0.00",
		BudgetUnit:    "CNY",
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Project mismatch:\n got %+v\nwant %+v", p, want)
	}
}

// Probe §1 + §4: real ZenTao Max returns HTTP 200 + envelope-fail rather
// than 404, but we still treat HTTP 404 as ErrNotFound for forward
// compatibility with future versions.
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

// This is the canonical missing-row shape on Max 8.x (probe §4).
func TestGetProject_NotFound_DoesNotExistMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Project does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// `zt_project` is shared with sprint/program rows. If a row's type
// drifts to "sprint" via out-of-band edit, this resource must treat
// the row as gone so Read → RemoveResource (matches grill §3.4).
func TestGetProject_TypeMismatchIsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","project":{"id":"50","type":"sprint","model":"scrum"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProject(context.Background(), 50)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (type=sprint), got %v", err, err)
	}
}

func TestGetProject_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"db down"}`))
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

// Verifies (a) all writeable fields go on the wire, (b) read-only fields
// are stripped by `json:"-"`, and (c) `type:"project"` is force-set even
// when the caller passed a different value.
func TestCreateProject_BodyShape(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":77,"message":"保存成功"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Project{
		Name:          "Beta",
		Model:         "scrum",
		Type:          "sprint", // must be overwritten to "project"
		Begin:         "2026-05-09",
		End:           "2026-12-31",
		Parent:        3,
		Products:      []int64{1, 2},
		WorkflowGroup: 2,
		ACL:           "private",
		PM:            "pm",
		Desc:          "d",
		// these must be stripped by json:"-"
		Code:         "should-not-go",
		Status:       "should-not-go",
		Lifetime:     "should-not-go",
		OpenedBy:     "should-not-go",
		Progress:     "should-not-go",
		TeamCount:    "should-not-go",
		Budget:       "should-not-go",
		BudgetUnit:   "should-not-go",
		LastEditedBy: "should-not-go",
	}
	out, err := c.CreateProject(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if out.ID != 77 {
		t.Fatalf("id = %d, want 77", out.ID)
	}
	if out.Type != "project" {
		t.Fatalf("returned Type = %q, want \"project\"", out.Type)
	}
	if gotMethod != http.MethodPost || gotPath != projectsPath {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
	if gotBody["type"] != "project" {
		t.Fatalf("wire type = %v, want \"project\" (must override caller-supplied)", gotBody["type"])
	}
	for _, mustSend := range []string{"name", "model", "type", "begin", "end", "parent", "products", "workflowGroup", "acl", "PM", "desc"} {
		if _, ok := gotBody[mustSend]; !ok {
			t.Errorf("body missing required-on-write field %q: %+v", mustSend, gotBody)
		}
	}
	for _, mustNotSend := range []string{"code", "status", "lifetime", "openedBy", "progress", "teamCount", "budget", "budgetUnit", "lastEditedBy"} {
		if _, ok := gotBody[mustNotSend]; ok {
			t.Errorf("body must NOT carry server-managed field %q: %+v", mustNotSend, gotBody)
		}
	}
	if products, ok := gotBody["products"].([]any); !ok || len(products) != 2 {
		t.Errorf("products should be JSON array of length 2, got %T %+v", gotBody["products"], gotBody["products"])
	}
}

func TestCreateProject_StringID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":"88"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProject(context.Background(), &Project{Name: "X", Model: "scrum"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if out.ID != 88 {
		t.Fatalf("id = %d, want 88", out.ID)
	}
}

func TestCreateProject_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProject(context.Background(), &Project{Name: "X", Model: "scrum"})
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestCreateProject_FailEnvelope_StatusKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":{"productsBox":"最少关联一个产品"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProject(context.Background(), &Project{Name: "x", Model: "scrum"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}

// Probe §4 documented a different envelope: {"result":"fail","message":...}
// instead of {"status":"fail",...}. Decoder must reject this as failure.
func TestCreateProject_FailEnvelope_ResultKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"end":["『计划完成』不能为空。"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProject(context.Background(), &Project{Name: "x", Model: "scrum"})
	if err == nil {
		t.Fatal("expected error on result=fail envelope")
	}
}

func TestUpdateProject_PutPathAndRefetch(t *testing.T) {
	var putBody map[string]any
	var gets, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == projectsPath+"/5":
			puts++
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","message":"保存成功"}`))
		case r.Method == http.MethodGet && r.URL.Path == projectsPath+"/5":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","project":{"id":"5","name":"NewName","model":"scrum","type":"project","parent":"3","workflowGroup":"2","acl":"private","PM":"pm","status":"wait"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProject(context.Background(), &Project{
		ID:    5,
		Name:  "NewName",
		Model: "scrum",
		PM:    "pm",
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if puts != 1 || gets != 1 {
		t.Fatalf("puts=%d gets=%d, want 1/1", puts, gets)
	}
	if putBody["name"] != "NewName" || putBody["PM"] != "pm" || putBody["type"] != "project" {
		t.Fatalf("PUT body = %+v", putBody)
	}
	if out.Name != "NewName" || out.ID != 5 || out.PM != "pm" || out.Type != "project" {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProject_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProject(context.Background(), &Project{Name: "x", Model: "scrum"})
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("err = %v, want missing id error", err)
	}
}

func TestUpdateProject_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateProject(context.Background(), &Project{ID: 99, Name: "x", Model: "scrum"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateProject_NotFound_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Project does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateProject(context.Background(), &Project{ID: 99, Name: "x", Model: "scrum"})
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
		_, _ = w.Write([]byte(`{"status":"success","closeModal":true,"load":"/zentao/zentao/index.php?m=project&f=browse&t=json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != projectsPath+"/9" {
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

// Real Max 8.x DELETE on already-gone row (probe §4): HTTP 200 + envelope-fail.
// Must still be a no-op so destroy + re-apply doesn't fail.
func TestDeleteProject_NotExistMessageIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Project does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProject(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProject should be idempotent on 200+message: %v", err)
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
