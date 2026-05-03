package zentaoapi

import (
	"context"
	"errors"
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
