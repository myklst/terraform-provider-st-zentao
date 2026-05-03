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

func TestGetProduct_FullFieldSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php/v2/products/42" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// v2 wraps numeric IDs as strings; reviewer comes back as a real array.
		_, _ = w.Write([]byte(`{
			"status":"success",
			"product":{
				"id":"42",
				"name":"Alpha",
				"code":"alpha-code",
				"program":"7",
				"line":"3",
				"type":"normal",
				"status":"normal",
				"desc":"a desc",
				"acl":"private",
				"PO":"po-user",
				"QD":"qd-user",
				"RD":"rd-user",
				"reviewer":["r1","r2"],
				"createdBy":"admin",
				"createdDate":"2026-05-03 10:00:00",
				"programName":"Program X"
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	want := &Product{
		ID:          42,
		Name:        "Alpha",
		Code:        "alpha-code",
		Program:     7,
		Line:        3,
		Type:        "normal",
		Status:      "normal",
		Description: "a desc",
		ACL:         "private",
		PO:          "po-user",
		QD:          "qd-user",
		RD:          "rd-user",
		Reviewer:    []string{"r1", "r2"},
		CreatedBy:   "admin",
		CreatedDate: "2026-05-03 10:00:00",
		ProgramName: "Program X",
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Product mismatch:\n got %+v\nwant %+v", p, want)
	}
}

func TestGetProduct_ReviewerAsCommaString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"1","reviewer":"r1,r2 ,r3"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if !reflect.DeepEqual(p.Reviewer, []string{"r1", "r2", "r3"}) {
		t.Fatalf("Reviewer = %v", p.Reviewer)
	}
}

func TestGetProduct_ReviewerEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"1","reviewer":""}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.Reviewer != nil {
		t.Fatalf("Reviewer = %v, want nil", p.Reviewer)
	}
}

func TestGetProduct_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"product not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProduct_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"db down"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Reason != "db down" {
		t.Fatalf("reason = %q", apiErr.Reason)
	}
}

func TestGetProduct_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateProduct_BodyOmitsReadOnlyFields(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":77}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Product{
		Name:        "Beta",
		Program:     1,
		Line:        2,
		Type:        "normal",
		Description: "d",
		ACL:         "open",
		PO:          "po1",
		QD:          "qd1",
		RD:          "rd1",
		Reviewer:    []string{"r1"},
		// these must be stripped by json:"-" tags (server-managed)
		Code:        "should-not-go",
		Status:      "should-not-go",
		CreatedBy:   "should-not-go",
		CreatedDate: "should-not-go",
		ProgramName: "should-not-go",
	}
	out, err := c.CreateProduct(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if out.ID != 77 {
		t.Fatalf("id = %d, want 77", out.ID)
	}
	if gotMethod != http.MethodPost || gotPath != "/api.php/v2/products" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
	for _, mustSend := range []string{"name", "program", "line", "type", "desc", "acl", "PO", "QD", "RD", "reviewer"} {
		if _, ok := gotBody[mustSend]; !ok {
			t.Errorf("body missing required-on-write field %q: %+v", mustSend, gotBody)
		}
	}
	for _, mustNotSend := range []string{"code", "status", "createdBy", "createdDate", "programName"} {
		if _, ok := gotBody[mustNotSend]; ok {
			t.Errorf("body must NOT carry server-managed field %q: %+v", mustNotSend, gotBody)
		}
	}
	// reviewer must be a JSON array per v2 docs
	if rv, ok := gotBody["reviewer"].([]any); !ok || len(rv) != 1 || rv[0] != "r1" {
		t.Errorf("reviewer should be JSON array of strings, got %T %+v", gotBody["reviewer"], gotBody["reviewer"])
	}
}

func TestCreateProduct_StringID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":"88"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProduct(context.Background(), &Product{Name: "X"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if out.ID != 88 {
		t.Fatalf("id = %d, want 88", out.ID)
	}
}

func TestCreateProduct_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: "X"})
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestCreateProduct_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","error":"name already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: "dup"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Reason, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateProduct_PutPathAndRefetch(t *testing.T) {
	var putBody map[string]any
	var gets, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api.php/v2/products/5":
			puts++
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api.php/v2/products/5":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","product":{"id":"5","name":"NewName","code":"alpha","program":"2","status":"normal","acl":"private","type":"normal","PO":"po"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProduct(context.Background(), &Product{ID: 5, Name: "NewName", Program: 2, PO: "po"})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if puts != 1 || gets != 1 {
		t.Fatalf("puts=%d gets=%d, want 1/1", puts, gets)
	}
	if putBody["name"] != "NewName" || putBody["PO"] != "po" {
		t.Fatalf("PUT body = %+v", putBody)
	}
	if out.Name != "NewName" || out.ID != 5 || out.Program != 2 || out.PO != "po" {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProduct_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProduct(context.Background(), &Product{Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("err = %v, want missing id error", err)
	}
}

func TestUpdateProduct_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateProduct(context.Background(), &Product{ID: 99, Name: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteProduct_Success(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api.php/v2/products/9" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteProduct_NotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"product not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct should be idempotent on 404: %v", err)
	}
}

func TestDeleteProduct_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteProduct(context.Background(), 9)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestFlexibleStringList_Unmarshal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"array", `["a","b"]`, []string{"a", "b"}},
		{"empty array", `[]`, []string{}},
		{"comma string", `"a,b ,c"`, []string{"a", "b", "c"}},
		{"empty string", `""`, nil},
		{"null", `null`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got flexibleStringList
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual([]string(got), tc.want) {
				t.Fatalf("got %v want %v", []string(got), tc.want)
			}
		})
	}
}

// silence: keep the import live in case future tests need it.
var _ = io.Discard
