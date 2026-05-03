package zentaoapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetProduct_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "m=product") || !strings.Contains(r.URL.RawQuery, "f=view") {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("productID") != "42" {
			t.Errorf("productID = %q", r.URL.Query().Get("productID"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Alpha","code":"alpha","status":"normal"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.ID != 42 || p.Name != "Alpha" || p.Code != "alpha" || p.Status != "normal" {
		t.Fatalf("got %+v", p)
	}
}

func TestGetProduct_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// ZenTao often returns 200 + status:failed for "not found"
		_, _ = w.Write([]byte(`{"status":"failed","reason":"the product does not exist"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProduct_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"db down"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
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

func TestCreateProduct_ReturnsID(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		// ZenTao create can return data either as the new id or as a string-encoded object.
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"id\":\"77\"}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	p := &Product{Name: "Beta", Code: "beta", ACL: "private", Type: "normal"}
	out, err := c.CreateProduct(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if out.ID != 77 {
		t.Fatalf("id = %d, want 77", out.ID)
	}
	if !strings.Contains(gotPath, "m=product") || !strings.Contains(gotPath, "f=create") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"name":"Beta"`) {
		t.Fatalf("body missing name: %s", gotBody)
	}
}

func TestCreateProduct_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: "X", Code: "x"})
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestCreateProduct_ValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"name already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: "dup", Code: "dup"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Reason, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateProduct_SendsFullPayload(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("productID") != "5" {
			t.Errorf("productID = %q", r.URL.Query().Get("productID"))
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"id":5,"name":"NewName","code":"alpha"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	out, err := c.UpdateProduct(context.Background(), &Product{ID: 5, Name: "NewName", Code: "alpha"})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if out.Name != "NewName" {
		t.Fatalf("name = %q", out.Name)
	}
	if !strings.Contains(gotBody, `"name":"NewName"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestDeleteProduct_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("productID") != "9" {
			t.Errorf("productID = %q", r.URL.Query().Get("productID"))
		}
		if r.URL.Query().Get("confirm") != "yes" {
			t.Errorf("confirm = %q", r.URL.Query().Get("confirm"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
}

func TestDeleteProduct_NotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"product does not exist"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct should be idempotent on not-found: %v", err)
	}
}

func TestDeleteProduct_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","reason":"forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	err := c.DeleteProduct(context.Background(), 9)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Reason != "forbidden" {
		t.Fatalf("err = %v", err)
	}
}
