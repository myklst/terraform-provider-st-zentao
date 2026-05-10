package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Product represents a ZenTao product.
type Product struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`

	Program   int64    `json:"program,omitempty"`
	Line      int64    `json:"line,omitempty"`
	Code      string   `json:"code,omitempty"`
	PO        string   `json:"PO,omitempty"`
	QD        string   `json:"QD,omitempty"`
	RD        string   `json:"RD,omitempty"`
	Reviewer  []string `json:"reviewer,omitempty"`
	Type      string   `json:"type,omitempty"`
	Status    string   `json:"status,omitempty"`
	Desc      string   `json:"desc,omitempty"`
	ACL       string   `json:"acl,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	Whitelist []string `json:"whitelist,omitempty"`

	// Server-managed fields, surfaced on read only.
	CreatedBy   string `json:"-"`
	CreatedDate string `json:"-"`
}

type productCtrlWire struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`

	Program   json.Number        `json:"program"`
	Line      json.Number        `json:"line"`
	Code      string             `json:"code"`
	PO        string             `json:"PO"`
	QD        string             `json:"QD"`
	RD        string             `json:"RD"`
	Reviewer  flexibleStringList `json:"reviewer"`
	Type      string             `json:"type"`
	Status    string             `json:"status"`
	Desc      string             `json:"desc"`
	ACL       string             `json:"acl"`
	Groups    flexibleStringList `json:"groups"`
	Whitelist flexibleStringList `json:"whitelist"`

	CreatedBy   string      `json:"createdBy"`
	CreatedDate string      `json:"createdDate"`
	Deleted     json.Number `json:"deleted"`
}

// flexibleStringList accepts either a JSON array of strings or a
// comma-separated JSON string.
type flexibleStringList []string

func (f *flexibleStringList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = nil
		return nil
	}
	switch data[0] {
	case '[':
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*f = arr
		return nil
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*f = nil
			return nil
		}
		out := strings.Split(s, ",")
		for i := range out {
			out[i] = strings.TrimSpace(out[i])
		}
		*f = out
		return nil
	default:
		return fmt.Errorf("flexibleStringList: unexpected JSON %s", string(data))
	}
}

func (w productCtrlWire) toProduct() (*Product, error) {
	id, err := jsonNumberToInt64(w.ID, "id")
	if err != nil {
		return nil, err
	}
	program, err := jsonNumberToInt64(w.Program, "program")
	if err != nil {
		return nil, err
	}
	line, err := jsonNumberToInt64(w.Line, "line")
	if err != nil {
		return nil, err
	}
	return &Product{
		ID:          id,
		Name:        w.Name,
		Program:     program,
		Line:        line,
		Code:        w.Code,
		PO:          w.PO,
		QD:          w.QD,
		RD:          w.RD,
		Reviewer:    w.Reviewer,
		Type:        w.Type,
		Status:      w.Status,
		Desc:        w.Desc,
		ACL:         w.ACL,
		Groups:      w.Groups,
		Whitelist:   w.Whitelist,
		CreatedBy:   w.CreatedBy,
		CreatedDate: w.CreatedDate,
	}, nil
}

func jsonNumberToInt64(n json.Number, field string) (int64, error) {
	if n == "" {
		return 0, nil
	}
	v, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("decode %s %q: %w", field, n.String(), err)
	}
	return v, nil
}

// productViewInner mirrors the inner JSON-encoded payload of
// product-view-{id}.json. The view route returns ~12 sibling sections
// (title, products, workflowGroups, actions, dynamics, users, groups,
// branches, reviewers, members, pager, product) — we only consume
// `product`, the row itself.
type productViewInner struct {
	Product json.RawMessage `json:"product"`
}

// productToForm always emits every form.php writable field, even when
// the value is empty/0. ZenTao's product-edit POST is not PATCH-semantic —
// any omitted form.php field is reset to its form.php default. See
// docs/superpowers/specs/probe-product-controller.md §4a.
//
// Multi-value fields use `name[]=v1&name[]=v2` so PHP parses them as an
// array; the server's filter:join then stores them comma-joined.
func productToForm(p *Product) url.Values {
	form := url.Values{}
	form.Set("name", p.Name)
	form.Set("program", strconv.FormatInt(p.Program, 10))
	form.Set("line", strconv.FormatInt(p.Line, 10))
	form.Set("PO", p.PO)
	form.Set("QD", p.QD)
	form.Set("RD", p.RD)
	form.Set("type", p.Type)
	form.Set("status", p.Status)
	form.Set("desc", p.Desc)
	form.Set("acl", p.ACL)
	addMulti(form, "reviewer", p.Reviewer)
	addMulti(form, "groups", p.Groups)
	addMulti(form, "whitelist", p.Whitelist)
	return form
}

func addMulti(form url.Values, key string, values []string) {
	if len(values) == 0 {
		// Submit an empty placeholder so a non-PATCH edit clears the field.
		form.Set(key+"[]", "")
		return
	}
	for _, v := range values {
		form.Add(key+"[]", v)
	}
}

// mergeProductBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-zero / non-empty / non-nil). Empty
// string, 0, and nil slice are read as "preserve baseline". This is the
// M-Z merge that makes UpdateProduct safe against the non-PATCH
// product-edit semantic.
func mergeProductBaseline(input, baseline *Product) *Product {
	out := *baseline
	out.ID = input.ID
	if input.Name != "" {
		out.Name = input.Name
	}
	if input.Program != 0 {
		out.Program = input.Program
	}
	if input.Line != 0 {
		out.Line = input.Line
	}
	if input.PO != "" {
		out.PO = input.PO
	}
	if input.QD != "" {
		out.QD = input.QD
	}
	if input.RD != "" {
		out.RD = input.RD
	}
	if input.Type != "" {
		out.Type = input.Type
	}
	if input.Status != "" {
		out.Status = input.Status
	}
	if input.Desc != "" {
		out.Desc = input.Desc
	}
	if input.ACL != "" {
		out.ACL = input.ACL
	}
	if input.Reviewer != nil {
		out.Reviewer = input.Reviewer
	}
	if input.Groups != nil {
		out.Groups = input.Groups
	}
	if input.Whitelist != nil {
		out.Whitelist = input.Whitelist
	}
	return &out
}

func productCreatePath() string {
	return controllerPath("product", "create", nil)
}

func productDeletePath(id int64) string {
	return controllerPath("product", "delete", []string{strconv.FormatInt(id, 10)})
}

// GetProduct reads via product-view-{id}.json. The view route returns
// two envelope shapes:
//
//  1. Existing id → CtrlEnvelope with `status:success` and a JSON-encoded
//     `data` string carrying the product row.
//  2. Missing id → CtrlSimpleResponse-like reply with `result:success`
//     and `load.alert` containing "对象不存在！" (no `data` field).
//
// We sniff the discriminator on `status` vs `result` and dispatch.
// Soft-deleted rows (`deleted=1`) come back from view in shape #1 and
// must be surfaced as ErrNotFound so Terraform Read clears state.
//
// product-view returns 10 derived fields (programName, progress, bugs,
// stories, etc.) on top of the form.php-writable set; productCtrlWire
// intentionally ignores them — the wrapper exposes only fields that are
// either user-set or write-relevant for the M-Z merge.
func (c *Client) GetProduct(ctx context.Context, id int64) (*Product, error) {
	body, status, err := c.doController(ctx, "product", "view", []string{strconv.FormatInt(id, 10)}, nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var probe struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
		Result string          `json:"result"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("decode get-product probe: %w (body=%s)", err, string(body))
	}
	// Missing-id shape: {"result":"success","load":{"alert":"...对象不存在...","locate":"..."}}.
	// status absent + no data string → not-found.
	if probe.Status == "" && len(probe.Data) == 0 {
		return nil, ErrNotFound
	}
	var env CtrlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-product envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner productViewInner
	if err := DecodeData(env, &inner); err != nil {
		return nil, fmt.Errorf("decode get-product data: %w (body=%s)", err, string(body))
	}
	if len(inner.Product) == 0 || string(inner.Product) == "null" || string(inner.Product) == "false" {
		return nil, ErrNotFound
	}
	var wire productCtrlWire
	if err := json.Unmarshal(inner.Product, &wire); err != nil {
		return nil, fmt.Errorf("decode get-product wire: %w (body=%s)", err, string(body))
	}
	if wire.Deleted.String() == "1" {
		return nil, ErrNotFound
	}
	return wire.toProduct()
}

// CreateProduct posts a form to product-create.json. The response echoes
// the new id directly — no post-create lookup needed. The wrapper
// re-fetches via GetProduct so callers receive the full row including
// server-derived fields (CreatedBy, CreatedDate, etc).
func (c *Client) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p == nil {
		return nil, fmt.Errorf("CreateProduct: product is nil")
	}
	if p.Name == "" {
		return nil, fmt.Errorf("CreateProduct: name required")
	}
	body, status, err := c.doControllerForm(ctx, "product", "create", nil, nil, productToForm(p))
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		Result  string          `json:"result"`
		Message json.RawMessage `json:"message,omitempty"`
		ID      json.Number     `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-product: %w (body=%s)", err, string(body))
	}
	if resp.Result != "success" {
		var simple CtrlSimpleResponse
		_ = json.Unmarshal(body, &simple)
		return nil, classifyCtrlSimple(status, simple, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create product: empty id in response (body=%s)", string(body))
	}
	return c.GetProduct(ctx, id)
}

// UpdateProduct fetches the baseline, merges caller overrides on top
// (M-Z merge), then submits the full form. ZenTao's product-edit POST
// is non-PATCH — any omitted form.php field is reset to its default
// (verified empirically: name-only edit clears program/PO/QD/RD/desc
// /reviewer). See spec §4a.
func (c *Client) UpdateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p == nil {
		return nil, fmt.Errorf("UpdateProduct: product is nil")
	}
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProduct: missing id")
	}
	baseline, err := c.GetProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("UpdateProduct: fetch baseline for merge: %w", err)
	}
	merged := mergeProductBaseline(p, baseline)
	body, status, err := c.doControllerForm(ctx, "product", "edit", []string{strconv.FormatInt(p.ID, 10)}, nil, productToForm(merged))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode update-product envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetProduct(ctx, p.ID)
}

// DeleteProduct is a destructive GET. The server accepts both
// `product-delete-{id}.json` and `product-delete-{id}-yes.json` — we
// use the bare form. The wrapper is idempotent: 404 and "already
// deleted" envelopes both succeed.
func (c *Client) DeleteProduct(ctx context.Context, id int64) error {
	body, status, err := c.doController(ctx, "product", "delete", []string{strconv.FormatInt(id, 10)}, nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode delete-product envelope: %w (body=%s)", err, string(body))
	}
	if resp.IsSuccess() {
		return nil
	}
	flat, _ := resp.FieldErrors()
	if isNotFoundReason(flat) {
		return nil
	}
	return classifyCtrlSimple(status, resp, body)
}

func apiError(httpStatus int, body []byte) error {
	var env ZentaoResponse
	_ = json.Unmarshal(body, &env)
	return &APIError{
		HTTPStatus:   httpStatus,
		ZentaoStatus: env.Status,
		Reason:       env.ZentaoFailReason(),
		RawBody:      body,
	}
}

// Path helpers exposed for symmetry with other modules' wrappers.
var _ = productCreatePath
var _ = productDeletePath
