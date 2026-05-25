package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type productViewInner struct {
	Product json.RawMessage `json:"product"`
}

// Product represents a ZenTao product.
type Product struct {
	ID        *int64    `json:"id,omitempty"`
	Program   *int64    `json:"program,omitempty"`
	Name      *string   `json:"name,omitempty"`
	Code      *string   `json:"code,omitempty"`
	Shadow    *bool     `json:"shadow,omitempty"`
	Line      *int64    `json:"line,omitempty"`
	Type      *string   `json:"type,omitempty"`
	Status    *string   `json:"status,omitempty"`
	Desc      *string   `json:"desc,omitempty"`
	PO        *string   `json:"PO,omitempty"`
	QD        *string   `json:"QD,omitempty"`
	RD        *string   `json:"RD,omitempty"`
	Reviewers *[]string `json:"reviewer,omitempty"`
	ACL       *string   `json:"acl,omitempty"`
	Whitelist *[]string `json:"whitelist,omitempty"`
	Deleted   *bool     `json:"deleted,omitempty"`
}

// UnmarshalJSON decodes a product-view GET wire payload into *Product.
// ZenTao's controller surface returns id / program / line / shadow / deleted
// as a mix of JSON numbers and quoted-number strings; the json.Number locals
// tolerate both shapes. Multi-value columns (reviewer / groups / whitelist)
// arrive as either a JSON array or a comma-joined string — flexibleStringList
// handles both. Absent fields stay nil so callers can distinguish "wire
// omitted this column" from "wire said empty string".
func (p *Product) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        json.Number        `json:"id"`
		Program   json.Number        `json:"program"`
		Line      json.Number        `json:"line"`
		Name      *string            `json:"name"`
		Code      *string            `json:"code"`
		Shadow    json.Number        `json:"shadow"`
		Type      *string            `json:"type"`
		Status    *string            `json:"status"`
		Desc      *string            `json:"desc"`
		PO        *string            `json:"PO"`
		QD        *string            `json:"QD"`
		RD        *string            `json:"RD"`
		Reviewers flexibleStringList `json:"reviewer"`
		ACL       *string            `json:"acl"`
		Whitelist flexibleStringList `json:"whitelist"`
		Deleted   json.Number        `json:"deleted"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		v, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		p.ID = &v
	}
	if raw.Program != "" {
		v, err := jsonNumberToInt64(raw.Program, "program")
		if err != nil {
			return err
		}
		p.Program = &v
	}
	if raw.Line != "" {
		v, err := jsonNumberToInt64(raw.Line, "line")
		if err != nil {
			return err
		}
		p.Line = &v
	}
	if raw.Shadow != "" {
		v := jsonNumberToBool(raw.Shadow)
		p.Shadow = &v
	}
	if raw.Deleted != "" {
		v := jsonNumberToBool(raw.Deleted)
		p.Deleted = &v
	}
	p.Name = raw.Name
	p.Code = raw.Code
	p.Type = raw.Type
	p.Status = raw.Status
	p.Desc = raw.Desc
	p.PO = raw.PO
	p.QD = raw.QD
	p.RD = raw.RD
	p.ACL = raw.ACL
	if raw.Reviewers != nil {
		v := []string(raw.Reviewers)
		p.Reviewers = &v
	}
	if raw.Whitelist != nil {
		v := []string(raw.Whitelist)
		p.Whitelist = &v
	}
	return nil
}

// toForm produces a form for `/product-create.json` and `/product-edit.json`.
//
// `deleted=0` is emitted unconditionally so a single edit POST drives
// the restore-on-soft-deleted path (CreateProduct with caller-supplied
// id): if the row was deleted=1, this flips it back; if alive, it's a
// no-op. Form.php acceptance of `deleted` is integration-test-verified.
func (p *Product) toForm() url.Values {
	// Multi-value fields use `name[]=v1&name[]=v2` so PHP parses them as an
	// array; the server's filter:join then stores them comma-joined.
	addMulti := func(form url.Values, key string, values []string) {
		if len(values) == 0 {
			form.Set(key+"[]", "")
			return
		}
		for _, v := range values {
			form.Add(key+"[]", v)
		}
	}
	form := url.Values{}
	form.Set("program", strconv.FormatInt(deref(p.Program), 10))
	form.Set("name", deref(p.Name))
	form.Set("line", strconv.FormatInt(deref(p.Line), 10))
	form.Set("type", deref(p.Type))
	form.Set("desc", deref(p.Desc))
	form.Set("PO", deref(p.PO))
	form.Set("QD", deref(p.QD))
	form.Set("RD", deref(p.RD))
	addMulti(form, "reviewer", deref(p.Reviewers))
	form.Set("acl", deref(p.ACL))
	addMulti(form, "whitelist", deref(p.Whitelist))
	form.Set("deleted", boolToIntStr(deref(p.Deleted)))
	return form
}

// mergeProductBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-nil pointers).
func mergeProductBaseline(input, baseline *Product) *Product {
	out := *baseline
	if input.ID != nil {
		out.ID = input.ID
	}
	if input.Program != nil {
		out.Program = input.Program
	}
	if input.Name != nil {
		out.Name = input.Name
	}
	if input.Line != nil {
		out.Line = input.Line
	}
	if input.Type != nil {
		out.Type = input.Type
	}
	if input.Status != nil {
		out.Status = input.Status
	}
	if input.Desc != nil {
		out.Desc = input.Desc
	}
	if input.PO != nil {
		out.PO = input.PO
	}
	if input.QD != nil {
		out.QD = input.QD
	}
	if input.RD != nil {
		out.RD = input.RD
	}
	if input.Reviewers != nil {
		out.Reviewers = input.Reviewers
	}
	if input.ACL != nil {
		out.ACL = input.ACL
	}
	if input.Whitelist != nil {
		out.Whitelist = input.Whitelist
	}
	if input.Deleted != nil {
		out.Deleted = input.Deleted
	}
	return &out
}

// GetProduct reads via `/product-view-{id}.json`.
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
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-product outer: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner productViewInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-product inner: %w (body=%s)", err, string(body))
	}
	if len(inner.Product) == 0 || string(inner.Product) == "null" || string(inner.Product) == "false" {
		return nil, ErrNotFound
	}
	var prod Product
	if err := json.Unmarshal(inner.Product, &prod); err != nil {
		return nil, fmt.Errorf("decode get-product wire: %w (body=%s)", err, string(body))
	}
	prod.Name = decodeEntitiesPtr(prod.Name)
	return &prod, nil
}

// CreateProduct posts a form to product-create.json. The response echoes
// the new id directly; the wrapper re-fetches via GetProduct so callers
// receive the full row including server-derived fields.
func (c *Client) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p == nil {
		return nil, fmt.Errorf("CreateProduct: product is nil")
	}
	if p.Name == nil {
		return nil, fmt.Errorf("CreateProduct: name required")
	}
	body, status, err := c.doControllerForm(ctx, "product", "create", nil, nil, p.toForm())
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
// (M-Z merge), then submits the full form.
func (c *Client) UpdateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p == nil {
		return nil, fmt.Errorf("UpdateProduct: product is nil")
	}
	if p.ID == nil || *p.ID == 0 {
		return nil, fmt.Errorf("UpdateProduct: missing id")
	}
	id := *p.ID
	baseline, err := c.GetProduct(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("UpdateProduct: fetch baseline for merge: %w", err)
	}
	merged := mergeProductBaseline(p, baseline)
	body, status, err := c.doControllerForm(ctx, "product", "edit", []string{strconv.FormatInt(id, 10)}, nil, merged.toForm())
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
	return c.GetProduct(ctx, id)
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
