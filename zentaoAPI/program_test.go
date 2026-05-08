package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGetProgram_FullFieldSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != programsPath+"/7" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"program":{
				"id":"7",
				"name":"Smart Home",
				"code":"SH",
				"begin":"2026-01-01",
				"end":"2026-12-31",
				"status":"doing",
				"parent":"0",
				"type":"program",
				"category":"rnd",
				"desc":"a desc",
				"PM":"pm-user",
				"PO":"po-user",
				"QD":"qd-user",
				"RD":"rd-user",
				"acl":"open",
				"budget":"100000",
				"budgetUnit":"CNY",
				"openedBy":"admin",
				"openedDate":"2026-01-01 09:00:00",
				"lastEditedBy":"admin",
				"realBegan":"2026-01-02",
				"realEnd":"",
				"progress":"30",
				"teamCount":"5"
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProgram(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetProgram: %v", err)
	}
	want := &Program{
		ID:           7,
		Name:         "Smart Home",
		Code:         "SH",
		Begin:        "2026-01-01",
		End:          "2026-12-31",
		Status:       "doing",
		Parent:       0,
		Type:         "program",
		Category:     "rnd",
		Description:  "a desc",
		PM:           "pm-user",
		PO:           "po-user",
		QD:           "qd-user",
		RD:           "rd-user",
		ACL:          "open",
		Budget:       "100000",
		BudgetUnit:   "CNY",
		OpenedBy:     "admin",
		OpenedDate:   "2026-01-01 09:00:00",
		LastEditedBy: "admin",
		RealBegan:    "2026-01-02",
		RealEnd:      "",
		Progress:     "30",
		TeamCount:    "5",
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Program mismatch:\n got %+v\nwant %+v", p, want)
	}
}

func TestGetProgram_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"program not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProgram(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProgram_NotExistMessageIsErrNotFound(t *testing.T) {
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

func TestCreateProgram_BodyOmitsReadOnlyFields(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":42}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Program{
		Name:        "Beta",
		Begin:       "2026-01-01",
		End:         "2026-12-31",
		PM:          "pm",
		Description: "d",
		// Server-managed read-only fields must be stripped.
		Code:         "should-not-go",
		Status:       "should-not-go",
		Parent:       99,
		Type:         "should-not-go",
		Category:     "should-not-go",
		ACL:          "should-not-go",
		PO:           "should-not-go",
		QD:           "should-not-go",
		RD:           "should-not-go",
		Budget:       "should-not-go",
		BudgetUnit:   "should-not-go",
		OpenedBy:     "should-not-go",
		OpenedDate:   "should-not-go",
		LastEditedBy: "should-not-go",
		RealBegan:    "should-not-go",
		RealEnd:      "should-not-go",
		Progress:     "should-not-go",
		TeamCount:    "should-not-go",
	}
	out, err := c.CreateProgram(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID != 42 {
		t.Fatalf("id = %d, want 42", out.ID)
	}
	if gotMethod != http.MethodPost || gotPath != programsPath {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
	for _, mustSend := range []string{"name", "begin", "end", "PM", "desc"} {
		if _, ok := gotBody[mustSend]; !ok {
			t.Errorf("body missing required-on-write field %q: %+v", mustSend, gotBody)
		}
	}
	for _, mustNotSend := range []string{
		"code", "status", "parent", "type", "category", "acl",
		"PO", "QD", "RD", "budget", "budgetUnit",
		"openedBy", "openedDate", "lastEditedBy",
		"realBegan", "realEnd", "progress", "teamCount",
	} {
		if _, ok := gotBody[mustNotSend]; ok {
			t.Errorf("body must NOT carry server-managed field %q: %+v", mustNotSend, gotBody)
		}
	}
}

func TestCreateProgram_StringID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":"55"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProgram(context.Background(), &Program{Name: "X", Begin: "2026-01-01", End: "2026-12-31"})
	if err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	if out.ID != 55 {
		t.Fatalf("id = %d, want 55", out.ID)
	}
}

func TestCreateProgram_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProgram(context.Background(), &Program{Name: "X", Begin: "2026-01-01", End: "2026-12-31"})
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestCreateProgram_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"name already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProgram(context.Background(), &Program{Name: "dup", Begin: "2026-01-01", End: "2026-12-31"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Reason, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateProgram_PutPathAndRefetch(t *testing.T) {
	var putBody map[string]any
	var gets, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == programsPath+"/5":
			puts++
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case r.Method == http.MethodGet && r.URL.Path == programsPath+"/5":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","program":{"id":"5","name":"NewName","begin":"2026-02-01","end":"2026-11-30","status":"doing","PM":"pm"}}`))
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
		t.Fatalf("puts=%d gets=%d, want 1/1", puts, gets)
	}
	if putBody["name"] != "NewName" || putBody["begin"] != "2026-02-01" || putBody["PM"] != "pm" {
		t.Fatalf("PUT body = %+v", putBody)
	}
	if out.Name != "NewName" || out.ID != 5 || out.Begin != "2026-02-01" {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProgram_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProgram(context.Background(), &Program{Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("err = %v, want missing id error", err)
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

func TestDeleteProgram_Success(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != programsPath+"/9" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteProgram_NotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram should be idempotent on 404: %v", err)
	}
}

func TestDeleteProgram_NotExistMessageIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"Program does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProgram(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProgram should be idempotent on 200+message: %v", err)
	}
}

func TestDeleteProgram_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteProgram(context.Background(), 9)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

// silence: keep io import live.
var _ = io.Discard
