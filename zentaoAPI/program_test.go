package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
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
		ID:         int64ptr(7),
		Parent:     int64ptr(0),
		Path:       strptr(",7,"),
		Name:       strptr("Smart Home"),
		PM:         strptr("pm-user"),
		Budget:     strptr("100000"),
		BudgetUnit: strptr("CNY"),
		Begin:      strptr("2026-01-01"),
		End:        strptr("2026-12-31"),
		Desc:       strptr("a desc"),
		Status:     strptr("doing"),
		ACL:        strptr("private"),
		Whitelist:  strptr(""),
		Deleted:    boolptr(false),
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
		Name:       strptr("Beta"),
		Begin:      strptr("2026-01-01"),
		End:        strptr("2026-12-31"),
		PM:         strptr("pm"),
		Desc:       strptr("d"),
		ACL:        strptr("private"),
		Budget:     strptr("100"),
		BudgetUnit: strptr("CNY"),
		// Status is on the struct (read back via GET) but toForm never
		// emits it — verified by the mustNotSend assertion below.
		Status: strptr("should-not-go"),
	}
	out, err := c.CreateProgram(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID == nil || *out.ID != 42 {
		t.Fatalf("id = %v, want 42", out.ID)
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
	_, err := c.CreateProgram(context.Background(), &Program{Name: strptr("x"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31")})
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
		{"missing name", &Program{Begin: strptr("2026-01-01"), End: strptr("2026-12-31")}, "name required"},
		{"missing begin", &Program{Name: strptr("x"), End: strptr("2026-12-31")}, "begin required"},
		{"missing end", &Program{Name: strptr("x"), Begin: strptr("2026-01-01")}, "end required"},
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

// UpdateProgram now does GET-baseline → POST-merged → GET-refetch.
// Verifies the merged form preserves baseline ACL/Budget/Whitelist when
// the input only changes Name/Desc.
func TestUpdateProgram_BaselineMergeThenRefetch(t *testing.T) {
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
			// Baseline carries acl=private, budget=1000, whitelist=u1,u2,
			// parent=9 — none of which the caller mentions in the input.
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"program\":{\"id\":5,\"name\":\"OldName\",\"begin\":\"2026-01-01\",\"end\":\"2026-12-31\",\"PM\":\"alice\",\"desc\":\"old\",\"acl\":\"private\",\"budget\":\"1000\",\"budgetUnit\":\"USD\",\"whitelist\":\"u1,u2\",\"parent\":9,\"status\":\"doing\"}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProgram(context.Background(), &Program{ID: int64ptr(5), Name: strptr("NewName"), Desc: strptr("fresh")})
	if err != nil {
		t.Fatalf("UpdateProgram: %v", err)
	}
	// 2 GETs: baseline fetch + post-update refetch. 1 POST.
	if puts != 1 || gets != 2 {
		t.Fatalf("posts=%d gets=%d, want 1/2", puts, gets)
	}
	// Caller-supplied changes must surface on the wire.
	for _, mustSend := range []string{"name=NewName", "desc=fresh"} {
		if !strings.Contains(postBody, mustSend) {
			t.Errorf("POST body missing override %q: %s", mustSend, postBody)
		}
	}
	// Baseline values for fields the caller did NOT touch must be sent
	// (otherwise ZenTao would reset them to defaults — F2 in §8).
	for _, mustPreserve := range []string{"acl=private", "budget=1000", "budgetUnit=USD", "whitelist=u1%2Cu2", "parent=9", "PM=alice", "begin=2026-01-01", "end=2026-12-31"} {
		if !strings.Contains(postBody, mustPreserve) {
			t.Errorf("POST body missing baseline preservation %q: %s", mustPreserve, postBody)
		}
	}
	// The mock returns the same baseline body for both GETs (it's not
	// stateful), so we don't assert on out.Name here — the merge proof
	// lives in the POST body assertions above.
	if out.ID == nil || *out.ID != 5 {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProgram_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProgram(context.Background(), &Program{Name: strptr("x"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31")})
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
	_, err := c.UpdateProgram(context.Background(), &Program{ID: int64ptr(99), Name: strptr("x"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31")})
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

// toForm always emits every form.php writeable field. ZenTao's
// program-edit POST resets any omitted field to its default.
func TestProgramToForm_AlwaysSetsAllWriteableFields(t *testing.T) {
	form := (&Program{
		Name:  strptr("x"),
		Begin: strptr("2026-01-01"),
		End:   strptr("2026-12-31"),
	}).toForm()
	for _, k := range []string{"name", "begin", "end", "parent", "PM", "desc", "acl", "budget", "budgetUnit", "whitelist"} {
		if _, ok := form[k]; !ok {
			t.Errorf("form must always carry key %q (always-set rule): %v", k, form)
		}
	}
	if form.Get("parent") != "0" {
		t.Errorf("parent must be explicit \"0\" when zero, got %q", form.Get("parent"))
	}
	if form.Get("desc") != "" || form.Get("PM") != "" {
		t.Errorf("empty-string optionals must be sent as empty, got desc=%q PM=%q", form.Get("desc"), form.Get("PM"))
	}
}

func TestMergeProgramBaseline_PreservesBaselineWhenInputNil(t *testing.T) {
	baseline := &Program{
		ID: int64ptr(5), Name: strptr("Base"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31"),
		Parent: int64ptr(9), PM: strptr("alice"), Desc: strptr("old desc"), ACL: strptr("private"),
		Budget: strptr("1000"), BudgetUnit: strptr("USD"), Whitelist: strptr("u1,u2"),
	}
	cases := []struct {
		name  string
		input *Program
		check func(t *testing.T, m *Program)
	}{
		{
			"only Name set — preserve everything else",
			&Program{ID: int64ptr(5), Name: strptr("NewName")},
			func(t *testing.T, m *Program) {
				if derefString(m.Name) != "NewName" {
					t.Errorf("Name not overridden: %q", derefString(m.Name))
				}
				if derefInt64(m.Parent) != 9 || derefString(m.PM) != "alice" || derefString(m.Desc) != "old desc" || derefString(m.ACL) != "private" || derefString(m.Budget) != "1000" || derefString(m.BudgetUnit) != "USD" || derefString(m.Whitelist) != "u1,u2" {
					t.Errorf("baseline not preserved: %+v", m)
				}
			},
		},
		{
			"only Desc set — preserve PM/ACL/Budget",
			&Program{ID: int64ptr(5), Desc: strptr("new desc")},
			func(t *testing.T, m *Program) {
				if derefString(m.Desc) != "new desc" {
					t.Errorf("Desc not overridden: %q", derefString(m.Desc))
				}
				if derefString(m.PM) != "alice" || derefString(m.ACL) != "private" || derefString(m.Budget) != "1000" {
					t.Errorf("non-Desc fields wiped: %+v", m)
				}
			},
		},
		{
			"only Parent set — overrides parent, preserves others",
			&Program{ID: int64ptr(5), Parent: int64ptr(13)},
			func(t *testing.T, m *Program) {
				if derefInt64(m.Parent) != 13 {
					t.Errorf("Parent not overridden: %d", derefInt64(m.Parent))
				}
				if derefString(m.Name) != "Base" || derefString(m.PM) != "alice" {
					t.Errorf("non-Parent fields wiped: %+v", m)
				}
			},
		},
		{
			"all fields set — overrides everything",
			&Program{ID: int64ptr(5), Name: strptr("N2"), Begin: strptr("2027-01-01"), End: strptr("2027-12-31"), Parent: int64ptr(1), PM: strptr("bob"), Desc: strptr("d2"), ACL: strptr("open"), Budget: strptr("2"), BudgetUnit: strptr("CNY"), Whitelist: strptr("x")},
			func(t *testing.T, m *Program) {
				if derefString(m.Name) != "N2" || derefString(m.Begin) != "2027-01-01" || derefString(m.End) != "2027-12-31" || derefInt64(m.Parent) != 1 || derefString(m.PM) != "bob" || derefString(m.Desc) != "d2" || derefString(m.ACL) != "open" || derefString(m.Budget) != "2" || derefString(m.BudgetUnit) != "CNY" || derefString(m.Whitelist) != "x" {
					t.Errorf("override incomplete: %+v", m)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged := mergeProgramBaseline(tc.input, baseline)
			tc.check(t, merged)
			if derefString(baseline.Name) != "Base" {
				t.Errorf("baseline mutated: %+v", baseline)
			}
		})
	}
}

// ----- SetProgramParent -----

// programParentMockServer wires a small fake controller. It serves
// program-edit-{id}.json GET from the rows map and program-edit-{id}.json
// POST by capturing the form body.
type programParentMockServer struct {
	rows           map[int]string // id -> JSON for "program" inner field
	rowsAfterPost  map[int]string // id -> JSON to swap in after the next POST (simulates server path recompute)
	postBody       string
	postPath       string
	postCount      int
	getCounts      map[int]int
	postRespStatus int
	postRespBody   string
}

func newProgramParentMockServer() *programParentMockServer {
	return &programParentMockServer{
		rows:           map[int]string{},
		rowsAfterPost:  map[int]string{},
		getCounts:      map[int]int{},
		postRespStatus: 200,
		postRespBody:   `{"result":"success","message":"保存成功","locate":"/program-browse.json"}`,
	}
}

func (m *programParentMockServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Match "/program-edit-<id>.json"
		path := r.URL.Path
		if !strings.HasPrefix(path, "/program-edit-") || !strings.HasSuffix(path, ".json") {
			t.Errorf("unexpected path %s", path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/program-edit-"), ".json")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			t.Errorf("bad id in path %s", path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m.getCounts[id]++
			row, ok := m.rows[id]
			var inner string
			if !ok {
				inner = `{"program":false}`
			} else {
				inner = `{"program":` + row + `}`
			}
			// data is a JSON-string-encoded payload — marshal to get
			// proper backslash-escaping of the inner quotes.
			payload, _ := json.Marshal(map[string]any{"status": "success", "data": inner})
			_, _ = w.Write(payload)
		case http.MethodPost:
			m.postCount++
			m.postPath = path
			buf := make([]byte, 8192)
			n, _ := r.Body.Read(buf)
			m.postBody = string(buf[:n])
			// Simulate server-side row mutation (zt_project.path recompute
			// happens inside program-edit on the real server).
			if next, ok := m.rowsAfterPost[id]; ok {
				m.rows[id] = next
			}
			w.WriteHeader(m.postRespStatus)
			_, _ = w.Write([]byte(m.postRespBody))
		default:
			t.Errorf("unexpected method %s %s", r.Method, path)
		}
	}
}

func TestSetProgramParent_PreflightRejects(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name    string
		child   int64
		parent  int64
		wantErr error
		wantSub string
	}{
		{"child zero", 0, 5, nil, "childID must be positive"},
		{"child negative", -1, 5, nil, "childID must be positive"},
		{"parent negative", 5, -1, nil, "parentID cannot be negative"},
		{"self-attach", 7, 7, ErrCycleDetected, "self-attach"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.SetProgramParent(context.Background(), tc.child, tc.parent)
			if err == nil {
				t.Fatalf("err = nil, want error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want errors.Is %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestSetProgramParent_HappyAttach(t *testing.T) {
	mock := newProgramParentMockServer()
	mock.rows[10] = `{"id":10,"name":"child","begin":"2026-01-01","end":"2026-12-31","parent":0,"path":",10,","PM":"alice","desc":"d","acl":"private","budget":"500","budgetUnit":"USD","whitelist":"u1"}`
	mock.rows[5] = `{"id":5,"name":"parent","begin":"2026-01-01","end":"2026-12-31","parent":0,"path":",5,"}`
	// After the attach POST, the server recomputes zt_project.path so that
	// the child's ancestry now leads with its new parent. SetProgramParent's
	// post-POST GetProgram must surface that new path back to the caller.
	mock.rowsAfterPost[10] = `{"id":10,"name":"child","begin":"2026-01-01","end":"2026-12-31","parent":5,"path":",5,10,","PM":"alice","desc":"d","acl":"private","budget":"500","budgetUnit":"USD","whitelist":"u1"}`
	srv := httptest.NewServer(mock.handler(t))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.SetProgramParent(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf("SetProgramParent: %v", err)
	}
	if mock.postCount != 1 || mock.postPath != "/program-edit-10.json" {
		t.Fatalf("posts=%d path=%s", mock.postCount, mock.postPath)
	}
	if mock.getCounts[10] != 2 || mock.getCounts[5] != 1 {
		t.Fatalf("get counts: child=%d parent=%d", mock.getCounts[10], mock.getCounts[5])
	}
	// Form must override parent and preserve every other writeable field.
	for _, want := range []string{"parent=5", "name=child", "PM=alice", "desc=d", "acl=private", "budget=500", "budgetUnit=USD", "whitelist=u1"} {
		if !strings.Contains(mock.postBody, want) {
			t.Errorf("POST body missing %q: %s", want, mock.postBody)
		}
	}
	// Returned *Program must reflect the post-attach ancestry.
	if out == nil {
		t.Fatalf("returned program is nil")
	}
	if got := derefInt64(out.Parent); got != 5 {
		t.Errorf("returned Parent = %d, want 5", got)
	}
	if got := derefString(out.Path); got != ",5,10," {
		t.Errorf("returned Path = %q, want %q", got, ",5,10,")
	}
}

func TestSetProgramParent_DetachWritesParentZero(t *testing.T) {
	mock := newProgramParentMockServer()
	mock.rows[10] = `{"id":10,"name":"child","begin":"2026-01-01","end":"2026-12-31","parent":5,"path":",5,10,"}`
	// After detach, the server collapses path back to the root-only form.
	mock.rowsAfterPost[10] = `{"id":10,"name":"child","begin":"2026-01-01","end":"2026-12-31","parent":0,"path":",10,"}`
	srv := httptest.NewServer(mock.handler(t))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.SetProgramParent(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("SetProgramParent: %v", err)
	}
	// Detach must not look up a parent (parentID=0 short-circuits the cycle check).
	// Child is fetched twice: baseline before POST, then re-read after POST to return *Program.
	if mock.getCounts[10] != 2 || len(mock.getCounts) != 1 {
		t.Fatalf("unexpected GETs: %v", mock.getCounts)
	}
	if !strings.Contains(mock.postBody, "parent=0") {
		t.Errorf("detach must send parent=0: %s", mock.postBody)
	}
	// Returned *Program must reflect the detached ancestry.
	if out == nil {
		t.Fatalf("returned program is nil")
	}
	if got := derefInt64(out.Parent); got != 0 {
		t.Errorf("returned Parent = %d, want 0", got)
	}
	if got := derefString(out.Path); got != ",10," {
		t.Errorf("returned Path = %q, want %q", got, ",10,")
	}
}

func TestSetProgramParent_RejectsCycleViaPath(t *testing.T) {
	// Setup: child=10, prospective parent=20. parent.path=",10,20,"
	// (10 is already an ancestor of 20). Attaching 10 under 20 would
	// form a cycle.
	mock := newProgramParentMockServer()
	mock.rows[10] = `{"id":10,"name":"c","begin":"2026-01-01","end":"2026-12-31","parent":0,"path":",10,"}`
	mock.rows[20] = `{"id":20,"name":"p","begin":"2026-01-01","end":"2026-12-31","parent":10,"path":",10,20,"}`
	srv := httptest.NewServer(mock.handler(t))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.SetProgramParent(context.Background(), 10, 20)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("err = %v, want errors.Is(ErrCycleDetected)", err)
	}
	// No POST should have been issued.
	if mock.postCount != 0 {
		t.Errorf("posts=%d, expected zero (cycle should reject before POST)", mock.postCount)
	}
}

func TestSetProgramParent_ChildNotFound(t *testing.T) {
	mock := newProgramParentMockServer()
	// Empty rows map -> any GET returns program:false -> ErrNotFound.
	srv := httptest.NewServer(mock.handler(t))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.SetProgramParent(context.Background(), 999, 5)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want errors.Is(ErrNotFound)", err)
	}
}

func TestSetProgramParent_ParentNotFound(t *testing.T) {
	mock := newProgramParentMockServer()
	mock.rows[10] = `{"id":10,"name":"c","begin":"2026-01-01","end":"2026-12-31","parent":0,"path":",10,"}`
	// Parent 999 missing.
	srv := httptest.NewServer(mock.handler(t))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.SetProgramParent(context.Background(), 10, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want errors.Is(ErrNotFound)", err)
	}
}

var _ = json.Number("0")

// --- Restore-on-soft-deleted (CreateProgram with caller-supplied id) ---
//
// Matrix per SKILL.md §6a-bis (simplified):
//   p.ID == 0                → fresh create POST (covered by TestCreateProgram_BodyShape)
//   p.ID, lookup ErrNotFound → fresh create POST, caller's id ignored
//   p.ID, baseline returned  → single edit POST with merged input + deleted=0;
//                              row is undeleted in the same request when needed.
//   No name-compare. No alive/deleted branching. The form's deleted=0 makes
//   the operation idempotent on already-alive rows and restorative on soft-
//   deleted ones.

// Helper: build a stateful program-edit handler that:
//   GET → returns the row carrying `deleted` per the test's stub
//   POST → records the body and returns success
type programReplaceMock struct {
	gets, posts int
	postBody    string
	getResponse string // returned from every GET (lets the test seed deleted=0/1)
	t           *testing.T
}

func (m *programReplaceMock) handler(id int64) http.HandlerFunc {
	editPath := fmt.Sprintf("/program-edit-%d.json", id)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == editPath:
			m.gets++
			_, _ = w.Write([]byte(m.getResponse))
		case r.Method == http.MethodPost && r.URL.Path == editPath:
			m.posts++
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			m.postBody = string(buf[:n])
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","locate":"/program-browse.json"}`))
		default:
			m.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// Soft-deleted row + caller-supplied id → single edit POST that overwrites
// the row with the merged input AND flips deleted back to 0 via form.
func TestCreateProgram_RestoreOnSoftDeleted_HappyPath(t *testing.T) {
	mock := &programReplaceMock{
		t: t,
		// Soft-deleted baseline. After restore, GetProgram (final refetch)
		// would normally return ErrNotFound, but our stub returns deleted=0
		// to simulate the post-restore state — same response for both GETs
		// to keep the mock stateless.
		getResponse: `{"status":"success","data":"{\"program\":{\"id\":42,\"name\":\"Beta\",\"begin\":\"2026-01-01\",\"end\":\"2026-12-31\",\"parent\":0,\"PM\":\"alice\",\"desc\":\"old\",\"acl\":\"private\",\"budget\":\"100\",\"budgetUnit\":\"CNY\",\"whitelist\":\"\",\"deleted\":0}}"}`,
	}
	srv := httptest.NewServer(mock.handler(42))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Program{
		ID:    int64ptr(42), // caller-supplied (residual TF state after destroy)
		Name:  strptr("Beta"),
		Begin: strptr("2026-01-01"),
		End:   strptr("2026-12-31"),
		Desc:  strptr("fresh"),
	}
	out, err := c.CreateProgram(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID == nil || *out.ID != 42 {
		t.Fatalf("returned id = %v, want 42 (id preserved across restore)", out.ID)
	}
	// One pre-flight Any GET + one post-replace refetch GET. One edit POST.
	if mock.posts != 1 {
		t.Fatalf("edit POSTs = %d, want 1", mock.posts)
	}
	if mock.gets != 2 {
		t.Fatalf("edit GETs = %d, want 2 (Any-baseline + refetch)", mock.gets)
	}
	// Caller override + baseline preservation + deleted=0 must all be in the form.
	for _, want := range []string{"name=Beta", "desc=fresh", "PM=alice", "acl=private", "budget=100", "deleted=0"} {
		if !strings.Contains(mock.postBody, want) {
			t.Errorf("POST body missing %q: %s", want, mock.postBody)
		}
	}
}

// Hard-gone row (Max 8.1's `program:false` shape) → caller's id ignored,
// fresh create POST issued.
func TestCreateProgram_RestoreOnSoftDeleted_NotFound_FallsThroughToCreate(t *testing.T) {
	var preflights, creates int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/program-edit-42.json":
			preflights++
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"program\":false}"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/program-create.json":
			creates++
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","id":99,"load":"/program-browse.json"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProgram(context.Background(), &Program{ID: int64ptr(42), Name: strptr("Beta"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31")})
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if preflights != 1 || creates != 1 {
		t.Fatalf("preflights=%d creates=%d, want 1/1", preflights, creates)
	}
	if out.ID == nil || *out.ID != 99 {
		t.Fatalf("returned id = %v, want 99 (fresh server-assigned)", out.ID)
	}
}

// Alive row + caller-supplied id → still goes through the replace path
// (no preflight rejection). Form carries deleted=0 (no-op on alive rows).
func TestCreateProgram_RestoreOnSoftDeleted_AliveRow_Replaces(t *testing.T) {
	mock := &programReplaceMock{
		t:           t,
		getResponse: `{"status":"success","data":"{\"program\":{\"id\":42,\"name\":\"Beta\",\"begin\":\"2026-01-01\",\"end\":\"2026-12-31\",\"parent\":0,\"PM\":\"alice\",\"desc\":\"old\",\"acl\":\"private\",\"budget\":\"100\",\"budgetUnit\":\"CNY\",\"whitelist\":\"\",\"deleted\":0}}"}`,
	}
	srv := httptest.NewServer(mock.handler(42))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProgram(context.Background(), &Program{
		ID: int64ptr(42), Name: strptr("Beta"), Begin: strptr("2026-01-01"), End: strptr("2026-12-31"), Desc: strptr("overwrite"),
	})
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID == nil || *out.ID != 42 || mock.posts != 1 {
		t.Fatalf("out.ID=%v posts=%d, want 42/1", out.ID, mock.posts)
	}
	if !strings.Contains(mock.postBody, "desc=overwrite") || !strings.Contains(mock.postBody, "deleted=0") {
		t.Errorf("POST body missing override or deleted=0: %s", mock.postBody)
	}
}
