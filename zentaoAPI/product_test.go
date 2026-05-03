package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetProduct_Found(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","product":{"id":"42","name":"Alpha","code":"alpha","status":"normal","desc":"d","acl":"private","type":"normal"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/api.php/v2/products/42" {
		t.Fatalf("path = %q", gotPath)
	}
	if p.ID != 42 || p.Name != "Alpha" || p.Code != "alpha" || p.Status != "normal" || p.Description != "d" || p.ACL != "private" || p.Type != "normal" {
		t.Fatalf("got %+v", p)
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

func TestCreateProduct_ReturnsID(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":77}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p := &Product{Name: "Beta", Code: "beta", ACL: "private", Type: "normal"}
	out, err := c.CreateProduct(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if out.ID != 77 {
		t.Fatalf("id = %d, want 77", out.ID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/api.php/v2/products" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"name":"Beta"`) {
		t.Fatalf("body missing name: %s", gotBody)
	}
}

func TestCreateProduct_StringID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":"88"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.CreateProduct(context.Background(), &Product{Name: "X", Code: "x"})
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
	_, err := c.CreateProduct(context.Background(), &Product{Name: "X", Code: "x"})
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
	_, err := c.CreateProduct(context.Background(), &Product{Name: "dup", Code: "dup"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Reason, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateProduct_PutPathAndRefetch(t *testing.T) {
	var putBody string
	var gets, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api.php/v2/products/5":
			puts++
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api.php/v2/products/5":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","product":{"id":"5","name":"NewName","code":"alpha","status":"normal","acl":"private","type":"normal"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProduct(context.Background(), &Product{ID: 5, Name: "NewName", Code: "alpha"})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if puts != 1 || gets != 1 {
		t.Fatalf("puts=%d gets=%d, want 1/1", puts, gets)
	}
	if !strings.Contains(putBody, `"name":"NewName"`) {
		t.Fatalf("PUT body = %s", putBody)
	}
	if out.Name != "NewName" || out.ID != 5 {
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
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api.php/v2/products/9" {
		t.Fatalf("path = %q", gotPath)
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

// guard: keep encoding/json import live so test-time JSON helpers are easy to add.
var _ = json.Marshal
