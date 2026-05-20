package zentaoapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// groupsEnvelope wraps a stringified `{"groups":..,"pager":..}` inner body in
// the controller success envelope, matching the real workflowgroup-project.json
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

func newWorkflowGroupServer(t *testing.T, inner string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflowgroup-project.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(groupsEnvelope(inner)))
	}))
}

func TestListWorkflowGroups_HappyPath(t *testing.T) {
	inner := `{"title":"x","groups":{` +
		`"3":{"id":3,"code":"scrumproject","name":"敏捷式项目研发","projectModel":"scrum","projectType":"project","vision":"rnd","status":"normal","deleted":0},` +
		`"2":{"id":2,"code":"scrumproduct","name":"敏捷式产品研发","projectModel":"scrum","projectType":"product","vision":"rnd","status":"normal","deleted":0}` +
		`},"pager":{"recTotal":2,"recPerPage":20,"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	groups, err := c.ListWorkflowGroups(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflowGroups: %v", err)
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

func TestListWorkflowGroups_SkipsDeleted(t *testing.T) {
	inner := `{"groups":{` +
		`"2":{"id":2,"code":"scrumproduct","projectModel":"scrum","projectType":"product","deleted":0},` +
		`"99":{"id":99,"code":"gone","projectModel":"scrum","projectType":"product","deleted":1}` +
		`},"pager":{"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	groups, err := c.ListWorkflowGroups(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflowGroups: %v", err)
	}
	if len(groups) != 1 || deref(groups[0].ID) != 2 {
		t.Fatalf("groups = %+v, want only id=2 (deleted skipped)", groups)
	}
}

func TestListWorkflowGroups_PageTotalExceeded(t *testing.T) {
	inner := `{"groups":{"2":{"id":2,"projectModel":"scrum","projectType":"product","deleted":0}},"pager":{"pageTotal":2}}`
	srv := newWorkflowGroupServer(t, inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.ListWorkflowGroups(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pageTotal") {
		t.Fatalf("err = %v, want pageTotal-exceeded error", err)
	}
}

func TestFindWorkflowGroup_UniqueMatch(t *testing.T) {
	inner := `{"groups":{` +
		`"2":{"id":2,"code":"scrumproduct","projectModel":"scrum","projectType":"product","deleted":0},` +
		`"3":{"id":3,"code":"scrumproject","projectModel":"scrum","projectType":"project","deleted":0}` +
		`},"pager":{"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, inner)
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
	inner := `{"groups":{"2":{"id":2,"projectModel":"scrum","projectType":"product","deleted":0}},"pager":{"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, inner)
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.FindWorkflowGroup(context.Background(), "waterfall", "project")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound wrapped", err)
	}
}

func TestFindWorkflowGroup_MultipleMatches(t *testing.T) {
	inner := `{"groups":{` +
		`"2":{"id":2,"projectModel":"scrum","projectType":"product","deleted":0},` +
		`"3":{"id":3,"projectModel":"scrum","projectType":"product","deleted":0}` +
		`},"pager":{"pageTotal":1}}`
	srv := newWorkflowGroupServer(t, inner)
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
