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

// Product UnmarshalJSON round-trip — mirrors the inner.product payload from
// product-view-{id}.json on the probed Max 8.1 instance. Numerics use the
// real wire types — a mix of int and string-quoted, exercised by the
// json.Number locals in (*Product).UnmarshalJSON. Multi-value reviewer
// arrives as the comma-joined string per filter:join in form.php.
func TestProduct_UnmarshalJSON_FullPayload(t *testing.T) {
	raw := `{
		"id": 42,
		"program": "7",
		"line": "3",
		"name": "Alpha",
		"code": "alpha-code",
		"type": "normal",
		"status": "normal",
		"desc": "a desc",
		"acl": "private",
		"PO": "po-user",
		"QD": "qd-user",
		"RD": "rd-user",
		"reviewer": "r1,r2",
		"groups": "1,2",
		"whitelist": "admin,PM",
		"deleted": "0"
	}`
	var p Product
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := &Product{
		ID:        int64ptr(42),
		Name:      strptr("Alpha"),
		Code:      strptr("alpha-code"),
		Program:   int64ptr(7),
		Line:      int64ptr(3),
		Type:      strptr("normal"),
		Status:    strptr("normal"),
		Desc:      strptr("a desc"),
		ACL:       strptr("private"),
		PO:        strptr("po-user"),
		QD:        strptr("qd-user"),
		RD:        strptr("rd-user"),
		Reviewers:  strSlicePtr([]string{"r1", "r2"}),
		Whitelist: strSlicePtr([]string{"admin", "PM"}),
		Deleted:   boolptr(false),
	}
	if !reflect.DeepEqual(&p, want) {
		t.Fatalf("Product mismatch:\n got %+v\nwant %+v", &p, want)
	}
}

func TestGetProduct_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/product-view-42.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected req %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":{\"id\":42,\"program\":7,\"name\":\"Alpha\",\"code\":\"alpha\",\"type\":\"normal\",\"status\":\"normal\",\"desc\":\"d\",\"acl\":\"private\",\"PO\":\"po\",\"QD\":\"qd\",\"RD\":\"rd\",\"reviewer\":\"r1,r2\",\"groups\":\"\",\"whitelist\":\"\",\"deleted\":\"0\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if deref(p.ID) != 42 || deref(p.Name) != "Alpha" || deref(p.Program) != 7 {
		t.Fatalf("got %+v", p)
	}
	if !reflect.DeepEqual(deref(p.Reviewers), []string{"r1", "r2"}) {
		t.Fatalf("Reviewer = %v", deref(p.Reviewers))
	}
}

// Some ZenTao deployments persist name through htmlspecialchars(ENT_QUOTES),
// so a written apostrophe reads back as `&#039;` (and `&`→`&amp;`, `<`→`&lt;`).
// GetProduct must decode it to the canonical form, otherwise Terraform's
// create round-trip trips "inconsistent result after apply": config `'` vs
// state `&#039;`.
func TestGetProduct_DecodesHTMLEntitiesInName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":{\"id\":42,\"name\":\"PG | Game | [0161] Werewolf&#039;s Hunt &amp; &lt;tag&gt;\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	want := "PG | Game | [0161] Werewolf's Hunt & <tag>"
	if deref(p.Name) != want {
		t.Fatalf("Name = %q, want %q", deref(p.Name), want)
	}
}

func TestGetProduct_ReviewerEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":{\"id\":1,\"reviewer\":\"\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	p, err := c.GetProduct(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	// Empty wire string decodes to a non-nil pointer to an empty slice
	// (an explicit "the column is empty" signal), distinct from nil
	// ("wire omitted the column").
	if p.Reviewers == nil {
		t.Fatalf("Reviewer pointer is nil, want non-nil empty slice")
	}
	if len(*p.Reviewers) != 0 {
		t.Fatalf("Reviewers = %v, want empty slice", *p.Reviewers)
	}
}

// product-view's missing-id reply is a CtrlSimpleResponse-shaped envelope:
//
//	{"result":"success","load":{"alert":"对象不存在！","locate":"/zentao/product-all.json"}}
//
// no `data` field, no `status` field. GetProduct must sniff the
// discriminator and surface ErrNotFound.
func TestGetProduct_MissingID_LoadAlertEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","load":{"alert":"对象不存在！","locate":"/zentao/product-all.json"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (load.alert envelope)", err)
	}
}

func TestGetProduct_NotFound_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetProduct_NotFound_ProductFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":false}"}`))
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
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetProduct(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

// CreateProduct: form-encoded POST to product-create.json. Response
// echoes the new id directly; wrapper then refetches via GET.
func TestCreateProduct_BodyShapeAndRefetch(t *testing.T) {
	var posts, gets int
	var postPath, postCT, postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/product-create.json":
			posts++
			postPath = r.URL.Path
			postCT = r.Header.Get("Content-Type")
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","id":77}`))
		case r.Method == http.MethodGet && r.URL.Path == "/product-view-77.json":
			gets++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":{\"id\":77,\"name\":\"Beta\",\"program\":1,\"PO\":\"po1\",\"reviewer\":\"r1\",\"acl\":\"open\",\"type\":\"normal\",\"status\":\"normal\",\"deleted\":\"0\"}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	in := &Product{
		Name:     strptr("Beta"),
		Program:  int64ptr(1),
		Line:     int64ptr(0),
		Type:     strptr("normal"),
		Desc:     strptr("d"),
		ACL:      strptr("open"),
		PO:       strptr("po1"),
		QD:       strptr("qd1"),
		RD:       strptr("rd1"),
		Reviewers: strSlicePtr([]string{"r1"}),
		// Code is server-managed — caller-set value is ignored by toForm
		// (key absent from form.php).
		Code:   strptr("should-not-go"),
		Status: strptr("normal"),
	}
	out, err := c.CreateProduct(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if deref(out.ID) != 77 {
		t.Fatalf("id = %d, want 77", deref(out.ID))
	}
	if posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d, want 1/1", posts, gets)
	}
	if postPath != "/product-create.json" {
		t.Fatalf("post path = %q", postPath)
	}
	if !strings.HasPrefix(postCT, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q, want form-urlencoded", postCT)
	}
	for _, mustSend := range []string{"name=Beta", "program=1", "PO=po1", "QD=qd1", "RD=rd1", "type=normal", "acl=open", "desc=d"} {
		if !strings.Contains(postBody, mustSend) {
			t.Errorf("body missing %q: %s", mustSend, postBody)
		}
	}
	// reviewer must be sent as reviewer[]=r1 (URL-encoded `[]=`).
	if !strings.Contains(postBody, "reviewer%5B%5D=r1") {
		t.Errorf("reviewer must be sent as reviewer[]=r1 (URL-encoded), got body=%s", postBody)
	}
	// Server-managed / non-writable fields must NOT leak into the form.
	for _, mustNotSend := range []string{"code="} {
		if strings.Contains(postBody, mustNotSend) {
			t.Errorf("body must NOT carry %q: %s", mustNotSend, postBody)
		}
	}
}

func TestCreateProduct_PreflightValidation(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name string
		in   *Product
		want string
	}{
		{"nil", nil, "nil"},
		{"missing name", &Product{Program: int64ptr(1)}, "name required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.CreateProduct(context.Background(), tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestCreateProduct_ZeroIDIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","id":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: strptr("X")})
	if err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("err = %v, want empty-id error", err)
	}
}

func TestCreateProduct_FailEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"name already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.CreateProduct(context.Background(), &Product{Name: strptr("dup")})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Reason, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

// UpdateProduct does GET-baseline → POST-merged → GET-refetch.
// Verifies that fields not touched by the caller (program/PO/desc/reviewer
// in this case) are preserved from the baseline — without M-Z merge, the
// server would reset them to form.php defaults (probe §4a).
func TestUpdateProduct_BaselineMergeThenRefetch(t *testing.T) {
	var posts, gets int
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/product-edit-5.json":
			posts++
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			postBody = string(buf[:n])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"success","message":"保存成功","load":"/zentao/product-view-5.json"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/product-view-5.json":
			gets++
			w.Header().Set("Content-Type", "application/json")
			// Baseline has program=2, PO=alice, desc=old, reviewer=r1,r2
			// — none of which the caller mentions in the input.
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"product\":{\"id\":5,\"name\":\"OldName\",\"program\":2,\"PO\":\"alice\",\"QD\":\"bob\",\"RD\":\"carol\",\"desc\":\"old\",\"reviewer\":\"r1,r2\",\"type\":\"normal\",\"status\":\"normal\",\"acl\":\"private\",\"deleted\":\"0\"}}"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	out, err := c.UpdateProduct(context.Background(), &Product{ID: int64ptr(5), Name: strptr("NewName")})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if posts != 1 || gets != 2 {
		t.Fatalf("posts=%d gets=%d, want 1/2", posts, gets)
	}
	if !strings.Contains(postBody, "name=NewName") {
		t.Errorf("POST body missing name override: %s", postBody)
	}
	for _, mustPreserve := range []string{"program=2", "PO=alice", "QD=bob", "RD=carol", "desc=old", "type=normal", "acl=private"} {
		if !strings.Contains(postBody, mustPreserve) {
			t.Errorf("POST body missing baseline preservation %q: %s", mustPreserve, postBody)
		}
	}
	// reviewer is multi-value; baseline has [r1,r2] which must be re-emitted as `reviewer[]=r1&reviewer[]=r2`.
	if !strings.Contains(postBody, "reviewer%5B%5D=r1") || !strings.Contains(postBody, "reviewer%5B%5D=r2") {
		t.Errorf("POST body must preserve baseline reviewer list: %s", postBody)
	}
	if deref(out.ID) != 5 {
		t.Fatalf("got %+v", out)
	}
}

func TestUpdateProduct_MissingID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	_, err := c.UpdateProduct(context.Background(), &Product{Name: strptr("x")})
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
	_, err := c.UpdateProduct(context.Background(), &Product{ID: int64ptr(99), Name: strptr("x")})
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
		_, _ = w.Write([]byte(`{"load":"/zentao/product-all.json","result":"success","message":"保存成功"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/product-delete-9.json" {
		t.Fatalf("req = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteProduct_HTTP404IsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct should be idempotent on 404: %v", err)
	}
}

func TestDeleteProduct_NotExistMessageIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"Product does not exist."}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteProduct(context.Background(), 9); err != nil {
		t.Fatalf("DeleteProduct should be idempotent on does-not-exist: %v", err)
	}
}

func TestDeleteProduct_OtherFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteProduct(context.Background(), 9)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}

// toForm always emits every form.php writable field even when the
// value is empty/0. ZenTao's product-edit POST resets any omitted field
// to its default — see probe-product-controller.md §4a.
func TestProductToForm_AlwaysSetsAllWriteableFields(t *testing.T) {
	form := (&Product{Name: strptr("x")}).toForm()
	for _, k := range []string{"name", "program", "line", "PO", "QD", "RD", "type", "desc", "acl"} {
		if _, ok := form[k]; !ok {
			t.Errorf("form must always carry key %q (always-set rule): %v", k, form)
		}
	}
	if form.Get("program") != "0" || form.Get("line") != "0" {
		t.Errorf("zero-valued ints must be explicit \"0\": program=%q line=%q", form.Get("program"), form.Get("line"))
	}
	// Multi-value keys are emitted with the [] suffix.
	for _, k := range []string{"reviewer[]", "whitelist[]"} {
		if _, ok := form[k]; !ok {
			t.Errorf("multi-value key %q must always be present (empty placeholder ok): %v", k, form)
		}
	}
}

func TestProductToForm_MultiValueExpansion(t *testing.T) {
	form := (&Product{
		Name:      strptr("x"),
		Reviewers:  strSlicePtr([]string{"r1", "r2"}),
		Whitelist: strSlicePtr([]string{"admin"}),
	}).toForm()
	if got := form["reviewer[]"]; !reflect.DeepEqual(got, []string{"r1", "r2"}) {
		t.Errorf("reviewer[] = %v, want [r1 r2]", got)
	}
	if got := form["whitelist[]"]; !reflect.DeepEqual(got, []string{"admin"}) {
		t.Errorf("whitelist[] = %v, want [admin]", got)
	}
}

func TestMergeProductBaseline_PreservesBaselineWhenInputNil(t *testing.T) {
	baseline := &Product{
		ID: int64ptr(5), Name: strptr("Base"), Program: int64ptr(2), Line: int64ptr(3),
		PO: strptr("alice"), QD: strptr("bob"), RD: strptr("carol"),
		Desc: strptr("old desc"), ACL: strptr("private"), Type: strptr("normal"), Status: strptr("normal"),
		Reviewers:  strSlicePtr([]string{"r1", "r2"}),
		Whitelist: strSlicePtr([]string{"admin"}),
	}
	cases := []struct {
		name  string
		input *Product
		check func(t *testing.T, m *Product)
	}{
		{
			"only Name set — preserve everything else",
			&Product{ID: int64ptr(5), Name: strptr("NewName")},
			func(t *testing.T, m *Product) {
				if deref(m.Name) != "NewName" {
					t.Errorf("Name not overridden: %q", deref(m.Name))
				}
				if deref(m.Program) != 2 || deref(m.PO) != "alice" || deref(m.Desc) != "old desc" || deref(m.ACL) != "private" {
					t.Errorf("baseline scalars not preserved: %+v", m)
				}
				if !reflect.DeepEqual(deref(m.Reviewers), []string{"r1", "r2"}) {
					t.Errorf("baseline slices not preserved: %+v", m)
				}
			},
		},
		{
			"override Reviewer with explicit empty slice — replace baseline",
			&Product{ID: int64ptr(5), Reviewers: strSlicePtr([]string{})},
			func(t *testing.T, m *Product) {
				if len(deref(m.Reviewers)) != 0 {
					t.Errorf("Reviewer should be cleared by explicit []string{}, got %v", deref(m.Reviewers))
				}
			},
		},
		{
			"override Program → caller wins",
			&Product{ID: int64ptr(5), Program: int64ptr(9)},
			func(t *testing.T, m *Product) {
				if deref(m.Program) != 9 {
					t.Errorf("Program not overridden: %d", deref(m.Program))
				}
				if deref(m.Name) != "Base" {
					t.Errorf("Name should still be baseline: %q", deref(m.Name))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeProductBaseline(tc.input, baseline)
			if deref(got.ID) != 5 {
				t.Errorf("ID must always be the input ID: got %d", deref(got.ID))
			}
			tc.check(t, got)
		})
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
		{"empty string", `""`, []string{}},
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
