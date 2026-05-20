package zentaoapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// groupsEnvelope wraps a stringified `{"groups":..,"pager":..}` inner body in
// the controller success envelope, matching the real workflowgroup-*.json
// wire shape (data is a JSON-encoded string).
func groupsEnvelope(inner string) string {
	return `{"status":"success","data":` + jsonString(inner) + `}`
}

func jsonString(s string) string {
	// minimal JSON string encoder for test fixtures
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// newWorkflowGroupServer asserts the request hits the catalog endpoint for the
// given controller method (product | project) and replies with the envelope.
func newWorkflowGroupServer(t *testing.T, method, inner string) *httptest.Server {
	t.Helper()
	wantPath := "/workflowgroup-" + method + ".json"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s, want GET %s", r.Method, r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(groupsEnvelope(inner)))
	}))
}

func TestListProjectWorkflowGroups_HappyPath(t *testing.T) {
	inner := `{"title":"x","groups":{` +
		`"3":{"id":3,"code":"scrumproject","name":"敏捷式项目研发","projectModel":"scrum","projectType":"project","vision":"rnd","status":"normal","deleted":0},` +
		`"2":{"id":2,"code":"scrumproduct","name":"敏捷式产品研发","projectModel":"scrum","projectType":"product","vision":"rnd","status":"normal","deleted":0}` +
		`},"pager":{"recTotal":2,"recPerPage":20,"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, "project", inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	groups, err := c.ListProjectWorkflowGroups(context.Background())
	if err != nil {
		t.Fatalf("ListProjectWorkflowGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("len = %d, want 2", len(groups))
	}
	if deref(groups[0].ID) != 2 || deref(groups[1].ID) != 3 {
		t.Fatalf("ids = [%d %d], want [2 3] ascending", deref(groups[0].ID), deref(groups[1].ID))
	}
	if deref(groups[0].Code) != "scrumproduct" || deref(groups[0].ProjectModel) != "scrum" || deref(groups[0].ProjectType) != "product" {
		t.Fatalf("decode wrong: %+v", groups[0])
	}
}

func TestListProjectWorkflowGroups_SkipsDeleted(t *testing.T) {
	inner := `{"groups":{` +
		`"2":{"id":2,"code":"scrumproduct","projectModel":"scrum","projectType":"product","deleted":0},` +
		`"99":{"id":99,"code":"gone","projectModel":"scrum","projectType":"product","deleted":1}` +
		`},"pager":{"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, "project", inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	groups, err := c.ListProjectWorkflowGroups(context.Background())
	if err != nil {
		t.Fatalf("ListProjectWorkflowGroups: %v", err)
	}
	if len(groups) != 1 || deref(groups[0].ID) != 2 {
		t.Fatalf("groups = %+v, want only id=2 (deleted skipped)", groups)
	}
}

func TestListProjectWorkflowGroups_PageTotalExceeded(t *testing.T) {
	inner := `{"groups":{"2":{"id":2,"projectModel":"scrum","projectType":"product","deleted":0}},"pager":{"pageTotal":2}}`
	srv := newWorkflowGroupServer(t, "project", inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.ListProjectWorkflowGroups(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pageTotal") {
		t.Fatalf("err = %v, want pageTotal-exceeded error", err)
	}
}

// TestListProductWorkflowGroups_HappyPath mirrors the real factory product
// catalog: a single 默认流程 row with an empty projectModel (see
// docs/superpowers/specs/probe-workflowgroup-project.md §6).
func TestListProductWorkflowGroups_HappyPath(t *testing.T) {
	inner := `{"title":"产品流程列表","groups":{` +
		`"1":{"id":1,"type":"product","projectModel":"","projectType":"project","name":"默认流程","code":"productproject","status":"normal","main":1,"vision":"rnd","deleted":0}` +
		`},"pager":{"recTotal":1,"recPerPage":20,"pageTotal":1,"methodName":"product"},"browseType":"all"}`
	srv := newWorkflowGroupServer(t, "product", inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	groups, err := c.ListProductWorkflowGroups(context.Background())
	if err != nil {
		t.Fatalf("ListProductWorkflowGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("len = %d, want 1", len(groups))
	}
	if deref(groups[0].ID) != 1 || deref(groups[0].Code) != "productproject" || deref(groups[0].ProjectModel) != "" {
		t.Fatalf("decode wrong: %+v", groups[0])
	}
}

func TestListProductWorkflowGroups_PageTotalExceeded(t *testing.T) {
	inner := `{"groups":{"1":{"id":1,"type":"product","projectModel":"","deleted":0}},"pager":{"pageTotal":2}}`
	srv := newWorkflowGroupServer(t, "product", inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.ListProductWorkflowGroups(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pageTotal") {
		t.Fatalf("err = %v, want pageTotal-exceeded error", err)
	}
}
