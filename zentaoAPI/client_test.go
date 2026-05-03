package zentaoapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoRequest_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotSID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotSID = r.URL.Query().Get("zentaosid")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"id":1,"name":"a"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)

	body, err := c.doRequest(context.Background(), http.MethodGet, "api.php", map[string]string{
		"m": "product",
		"f": "view",
	}, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotSID != "sid-1" {
		t.Fatalf("sessionID not propagated: %q", gotSID)
	}
	if !strings.Contains(gotPath, "m=product") || !strings.Contains(gotPath, "f=view") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody != "" {
		t.Fatalf("GET should not have body, got %q", gotBody)
	}
	var env ZentaoResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.Status != "success" {
		t.Fatalf("status = %q", env.Status)
	}
}

func TestDoRequest_PostJSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "sid-1", srv.URL)
	payload := map[string]string{"name": "x"}
	if _, err := c.doRequest(context.Background(), http.MethodPost, "api.php", map[string]string{"m": "product", "f": "create"}, payload); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"name":"x"`) {
		t.Fatalf("body = %q", gotBody)
	}
}
