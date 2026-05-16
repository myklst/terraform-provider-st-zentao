package zentaoapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetWorkflowGroup_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflowgroup-view-2.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":2,\"code\":\"scrumproduct\",\"name\":\"敏捷式产品研发\",\"projectModel\":\"scrum\",\"projectType\":\"product\",\"vision\":\"rnd\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	wfg, err := c.GetWorkflowGroup(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetWorkflowGroup: %v", err)
	}
	if deref(wfg.ID) != 2 || deref(wfg.Code) != "scrumproduct" || deref(wfg.ProjectModel) != "scrum" || deref(wfg.ProjectType) != "product" {
		t.Fatalf("decode wrong: %+v", wfg)
	}
}

func TestGetWorkflowGroup_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetWorkflowGroup(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListWorkflowGroupIDs_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project-create.json" || r.Method != http.MethodPost {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Trimmed project-create form rendering. Real payload has
		// workflowGroupPairs as id→name map.
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"workflowGroupPairs\":{\"3\":\"敏捷式项目研发\",\"2\":\"敏捷式产品研发\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	ids, err := c.ListWorkflowGroupIDs(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflowGroupIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("ids = %v, want [2 3] sorted ascending", ids)
	}
}

func TestFindWorkflowGroup_UniqueMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/project-create.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"workflowGroupPairs\":{\"2\":\"P\",\"3\":\"Q\"}}"}`))
		case "/workflowgroup-view-2.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":2,\"code\":\"scrumproduct\",\"projectModel\":\"scrum\",\"projectType\":\"product\"}}"}`))
		case "/workflowgroup-view-3.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":3,\"code\":\"scrumproject\",\"projectModel\":\"scrum\",\"projectType\":\"project\"}}"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	wfg, err := c.FindWorkflowGroup(context.Background(), "scrum", "product")
	if err != nil {
		t.Fatalf("FindWorkflowGroup: %v", err)
	}
	if deref(wfg.ID) != 2 || deref(wfg.Code) != "scrumproduct" {
		t.Fatalf("matched wrong row: %+v", wfg)
	}
}

func TestFindWorkflowGroup_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/project-create.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"workflowGroupPairs\":{\"2\":\"P\"}}"}`))
		case "/workflowgroup-view-2.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":2,\"projectModel\":\"scrum\",\"projectType\":\"product\"}}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.FindWorkflowGroup(context.Background(), "waterfall", "project")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound wrapped", err)
	}
}

func TestFindWorkflowGroup_MultipleMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/project-create.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"workflowGroupPairs\":{\"2\":\"P\",\"3\":\"Q\"}}"}`))
		case "/workflowgroup-view-2.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":2,\"projectModel\":\"scrum\",\"projectType\":\"product\"}}"}`))
		case "/workflowgroup-view-3.json":
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"group\":{\"id\":3,\"projectModel\":\"scrum\",\"projectType\":\"product\"}}"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.FindWorkflowGroup(context.Background(), "scrum", "product")
	if err == nil || !strings.Contains(err.Error(), "2 groups match") {
		t.Fatalf("err = %v, want ambiguity error", err)
	}
}

func TestFindWorkflowGroup_Preflight(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	if _, err := c.FindWorkflowGroup(context.Background(), "", "product"); err == nil || !strings.Contains(err.Error(), "projectModel required") {
		t.Fatalf("err = %v, want projectModel required", err)
	}
	if _, err := c.FindWorkflowGroup(context.Background(), "scrum", ""); err == nil || !strings.Contains(err.Error(), "projectType required") {
		t.Fatalf("err = %v, want projectType required", err)
	}
}
