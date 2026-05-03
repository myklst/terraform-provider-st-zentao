package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Status      string `json:"status,omitempty"`
	Description string `json:"desc,omitempty"`
	ACL         string `json:"acl,omitempty"`
	Type        string `json:"type,omitempty"`
}

// productV2Wire is the on-the-wire shape of a single product as returned by
// v2 endpoints. v2 returns numeric IDs as JSON strings ("id":"1"), so we
// decode through json.Number and project to Product.
type productV2Wire struct {
	ID          json.Number `json:"id"`
	Name        string      `json:"name"`
	Code        string      `json:"code"`
	Status      string      `json:"status"`
	Description string      `json:"desc"`
	ACL         string      `json:"acl"`
	Type        string      `json:"type"`
}

func (w productV2Wire) toProduct() (*Product, error) {
	id, err := w.ID.Int64()
	if err != nil && w.ID != "" {
		return nil, fmt.Errorf("decode product id %q: %w", w.ID.String(), err)
	}
	return &Product{
		ID:          int(id),
		Name:        w.Name,
		Code:        w.Code,
		Status:      w.Status,
		Description: w.Description,
		ACL:         w.ACL,
		Type:        w.Type,
	}, nil
}

func productPath(id int) string {
	return "api.php/v2/products/" + strconv.Itoa(id)
}

const productsPath = "api.php/v2/products"

// GetProduct fetches a product by ID via GET /api.php/v2/products/{id}.
// Returns ErrNotFound on HTTP 404.
func (c *Client) GetProduct(ctx context.Context, id int) (*Product, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, productPath(id), nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		ZentaoResponse
		Product productV2Wire `json:"product"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode get-product: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		return nil, apiError(status, body)
	}
	return resp.Product.toProduct()
}

// CreateProduct creates a product via POST /api.php/v2/products.
// v2 only echoes back the new id; the caller should re-fetch via
// GetProduct if it needs server-defaulted fields.
func (c *Client) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	body, status, err := c.doRequest(ctx, http.MethodPost, productsPath, nil, p)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		ZentaoResponse
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-product: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		return nil, apiError(status, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create product: empty id in response (body=%s)", string(body))
	}
	out := *p
	out.ID = int(id)
	return &out, nil
}

// UpdateProduct edits a product via PUT /api.php/v2/products/{id}. v2 only
// returns {status}, so on success we re-fetch the product to surface the
// authoritative state to the caller.
func (c *Client) UpdateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProduct: missing id")
	}
	body, status, err := c.doRequest(ctx, http.MethodPut, productPath(p.ID), nil, p)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp ZentaoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode update-product: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		return nil, apiError(status, body)
	}
	return c.GetProduct(ctx, p.ID)
}

// DeleteProduct removes a product via DELETE /api.php/v2/products/{id}.
// Idempotent: HTTP 404 (or a status:"fail" envelope referring to a missing
// row) is treated as success.
func (c *Client) DeleteProduct(ctx context.Context, id int) error {
	body, status, err := c.doRequest(ctx, http.MethodDelete, productPath(id), nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp ZentaoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode delete-product: %w (body=%s)", err, string(body))
	}
	if resp.Status == "success" {
		return nil
	}
	return apiError(status, body)
}

// apiError builds an APIError, parsing the envelope status/reason out of
// the body when possible so callers get a structured failure description.
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
