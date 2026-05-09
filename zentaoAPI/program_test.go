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

func TestGetProgram_FullFieldSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/program-edit-7.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Trimmed-down version of the real controller envelope (probe
		// 2026-05-09): outer status/data wrapper, inner program object
		// living inside data after string-unwrap. Numerics use the
		// real wire types — a mix of int and string-quoted, exercised
		// by json.Number on the wire struct.
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":"{\"title\":\"Edit\",\"charters\":[],\"charter\":0,\"pmUsers\":{},\"program\":{\"id\":7,\"name\":\"Smart Home\",\"code\":\"SH\",\"begin\":\"2026-01-01\",\"end\":\"2026-12-31\",\"parent\":0,\"status\":\"doing\",\"type\":\"program\",\"category\":\"rnd\",\"lifetime\":\"\",\"vision\":\"rnd\",\"attribute\":\"\",\"model\":\"\",\"path\":\",7,\",\"grade\":1,\"multiple\":1,\"parallel\":0,\"enabled\":\"on\",\"frozen\":\"\",\"deleted\":0,\"hasProduct\":1,\"workflowGroup\":0,\"storyType\":\"story\",\"pri\":1,\"version\":1,\"parentVersion\":1,\"days\":0,\"firstEnd\":\"\",\"subStatus\":\"\",\"desc\":\"a desc\",\"acl\":\"private\",\"whitelist\":\"\",\"budget\":\"100000\",\"budgetUnit\":\"CNY\",\"openedBy\":\"admin\",\"openedDate\":\"2026-01-01 09:00:00\",\"lastEditedBy\":\"admin\",\"lastEditedDate\":\"\",\"realBegan\":\"2026-01-02\",\"realEnd\":\"\",\"closedBy\":\"\",\"closedDate\":\"\",\"closedReason\":\"\",\"canceledBy\":\"\",\"canceledDate\":\"\",\"suspendedDate\":\"\",\"PM\":\"pm-user\",\"PO\":\"po-user\",\"QD\":\"qd-user\",\"RD\":\"rd-user\",\"team\":\"\",\"order\":5,\"progress\":\"30\",\"percent\":\"0.00\",\"estimate\":\"0\",\"consumed\":\"0\",\"left\":\"0\",\"teamCount\":5}}"
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProgram(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetProgram: %v", err)
	}
	want := &Program{
		ID:             7,
		Name:           "Smart Home",
		Code:           "SH",
		Begin:          "2026-01-01",
		End:            "2026-12-31",
		Parent:         0,
		Status:         "doing",
		Type:           "program",
		Category:       "rnd",
		Lifetime:       "",
		Vision:         "rnd",
		Attribute:      "",
		Model:          "",
		Path:           ",7,",
		Grade:          1,
		Multiple:       "1",
		Parallel:       "0",
		Enabled:        "on",
		Frozen:         "",
		Deleted:        "0",
		HasProduct:     1,
		WorkflowGroup:  0,
		StoryType:      "story",
		Pri:            1,
		Version:        1,
		ParentVersion:  1,
		Days:           0,
		FirstEnd:       "",
		SubStatus:      "",
		Desc:           "a desc",
		ACL:            "private",
		Whitelist:      "",
		Budget:         "100000",
		BudgetUnit:     "CNY",
		OpenedBy:       "admin",
		OpenedDate:     "2026-01-01 09:00:00",
		LastEditedBy:   "admin",
		LastEditedDate: "",
		RealBegan:      "2026-01-02",
		RealEnd:        "",
		ClosedBy:       "",
		ClosedDate:     "",
		ClosedReason:   "",
		CanceledBy:     "",
		CanceledDate:   "",
		SuspendedDate:  "",
		PM:             "pm-user",
		PO:             "po-user",
		QD:             "qd-user",
		RD:             "rd-user",
		Team:           "",
		Order:          5,
		Progress:       "30",
		Percent:        "0.00",
		Estimate:       "0",
		Consumed:       "0",
		Left:           "0",
		TeamCount:      5,
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Program mismatch:\n got %+v\nwant %+v", p, want)
	}
}

// On Max 8.1, a missing-id GET returns HTTP 200 with `program:false`
// inside the data wrapper. The wrapper must surface this as ErrNotFound
// so Terraform Read clears the resource from state.
func TestGetProgram_NotFound_ProgramFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"title\":\"Edit\",\"program\":false,\"parents\":{}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProgram(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProgram_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProgram(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Soft-deleted rows still come back from program-edit-{id} (probe
// 2026-05-09: DELETE only flips `deleted=1`, the row is not removed).
// GetProgram must treat them as gone so Terraform Read clears state.
func TestGetProgram_SoftDeletedIsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"program\":{\"id\":7,\"name\":\"old\",\"deleted\":1}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProgram(context.Background(), 7)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (deleted=1 row), got %v", err, err)
	}
}

// Envelope-fail with a "does not exist" reason also collapses to ErrNotFound.
func TestGetProgram_NotFound_EnvelopeFailReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Program does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProgram(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Real Max 8.1 create response shape: {result, message, id, load}.
// Wrapper extracts `id` and returns the input *Program with ID set.
func TestCreateProgram_BodyShape(t *testing.T) {
	var gotPath, gotMethod, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","id":42,"load":"/program-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Program{
		Name:       "Beta",
		Begin:      "2026-01-01",
		End:        "2026-12-31",
		PM:         "pm",
		Desc:       "d",
		ACL:        "private",
		Budget:     "100",
		BudgetUnit: "CNY",
		// these must be stripped by programToForm (read-only inputs)
		Code:     "should-not-go",
		Status:   "should-not-go",
		Type:     "should-not-go",
		Category: "should-not-go",
		PO:       "should-not-go",
	}
	out, err := c.CreateProgram(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID != 42 {
		t.Fatalf("id = %d, want 42", out.ID)
	}
	if gotMethod != http.MethodPost || gotPath != "/program-create.json" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q, want form-urlencoded", gotCT)
	}
	for _, mustSend := range []string{"name=Beta", "begin=2026-01-01", "end=2026-12-31", "PM=pm", "desc=d", "acl=private", "budget=100", "budgetUnit=CNY"} {
		if !strings.Contains(gotBody, mustSend) {
			t.Errorf("body missing %q: %s", mustSend, gotBody)
		}
	}
	for _, mustNotSend := range []string{"code=", "status=", "type=", "category=", "PO="} {
		if strings.Contains(gotBody, mustNotSend) {
			t.Errorf("body must NOT carry server-managed field %q: %s", mustNotSend, gotBody)
		}
	}
}

// Real validation envelope (probed 2026-05-09 by omitting required fields):
//
//	{"result":"fail","message":{"name":["..."],"end":["..."]}}
//
// Must surface as *APIError with composed reason from per-field map.
func TestCreateProgram_FailEnvelope_FieldErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"name":["『项目集名称』不能为空。"],"end":["『计划完成』不能为空。"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProgram(context.Background(), &Program{Name: "x", Begin: "2026-01-01", End: "2026-12-31"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Reason, "name") || !strings.Contains(apiErr.Reason, "end") {
		t.Fatalf("reason should mention both fields, got %q", apiErr.Reason)
	}
}

func TestCreateProgram_PreflightValidation(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name string
		in   *Program
		want string
	}{
		{"nil", nil, "nil"},
		{"missing name", &Program{Begin: "2026-01-01", End: "2026-12-31"}, "name required"},
		{"missing begin", &Program{Name: "x", End: "2026-12-31"}, "begin required"},
		{"missing end", &Program{Name: "x", Begin: "2026-01-01"}, "end required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.CreateProgram(context.Background(), tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUpdateProgram_PostFormThenRefetch(t *testing.T) {
	var puts, gets int
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/program-edit-5.json":
			puts++
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","locate":"/program-browse.json"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/program-edit-5.json":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"program\":{\"id\":5,\"name\":\"NewName\",\"begin\":\"2026-02-01\",\"end\":\"2026-11-30\",\"PM\":\"pm\",\"status\":\"doing\"}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProgram(context.Background(), &Program{ID: 5, Name: "NewName", Begin: "2026-02-01", End: "2026-11-30", PM: "pm"})
	if err != nil {
		t.Fatalf("UpdateProgram: %v", err)
	}
	if puts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d, want 1/1", puts, gets)
	}
	if !strings.Contains(postBody, "name=NewName") || !strings.Contains(postBody, "begin=2026-02-01") || !strings.Contains(postBody, "PM=pm") {
		t.Fatalf("POST body = %q", postBody)
	}
	if out.Name != "NewName" || out.ID != 5 || out.Begin != "2026-02-01" {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProgram_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProgram(context.Background(), &Program{Name: "x", Begin: "2026-01-01", End: "2026-12-31"})
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Fatalf("err = %v, want id-required error", err)
	}
}

func TestUpdateProgram_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateProgram(context.Background(), &Program{ID: 99, Name: "x", Begin: "2026-01-01", End: "2026-12-31"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Probe 2026-05-09: program-delete-{id}-yes.json on a real row returns
// {result:success, load:true}. Both real-row delete and missing-row
// delete take the same shape — wrapper must be idempotent on missing.
func TestDeleteProgram_Success(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","load":true}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/program-delete-9-yes.json" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteProgram_MissingIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Probe 2026-05-09: missing-id delete returned the same success
		// envelope as a real delete; both flows are idempotent here.
		_, _ = w.Write([]byte(`{"result":"success","load":true}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 99999); err != nil {
		t.Fatalf("DeleteProgram on missing should be no-op: %v", err)
	}
}

func TestDeleteProgram_HTTP404IsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram should be idempotent on 404: %v", err)
	}
}

func TestDeleteProgram_NotExistReasonIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"Program does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram should be idempotent on does-not-exist: %v", err)
	}
}

func TestDeleteProgram_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteProgram(context.Background(), 9)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

// programToForm only emits non-empty optional fields, but always emits
// the required name/begin/end (preflight rejects empty values before
// the wrapper ever calls programToForm).
func TestProgramToForm_OmitsEmptyOptionals(t *testing.T) {
	form := programToForm(&Program{
		Name:  "x",
		Begin: "2026-01-01",
		End:   "2026-12-31",
	})
	must := []string{"name", "begin", "end"}
	for _, k := range must {
		if form.Get(k) == "" {
			t.Errorf("form missing required key %q: %v", k, form)
		}
	}
	for _, k := range []string{"PM", "desc", "acl", "budget", "budgetUnit", "whitelist", "parent"} {
		if _, ok := form[k]; ok {
			t.Errorf("form should not include empty optional %q: %v", k, form)
		}
	}
}

var _ = json.Number("0")
