package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Product represents a ZenTao product.
type Product struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name"`

	Program  int      `json:"program,omitempty"`
	Line     int      `json:"line,omitempty"`
	Type     string   `json:"type,omitempty"`
	Desc     string   `json:"desc,omitempty"`
	ACL      string   `json:"acl,omitempty"`
	PO       string   `json:"PO,omitempty"`
	QD       string   `json:"QD,omitempty"`
	RD       string   `json:"RD,omitempty"`
	Reviewer []string `json:"reviewer,omitempty"`

	Code        string `json:"-"`
	Status      string `json:"-"`
	CreatedBy   string `json:"-"`
	CreatedDate string `json:"-"`
	ProgramName string `json:"-"`
}

type productV2Wire struct {
	ID          json.Number        `json:"id"`
	Name        string             `json:"name"`
	Code        string             `json:"code"`
	Program     json.Number        `json:"program"`
	Line        json.Number        `json:"line"`
	Type        string             `json:"type"`
	Status      string             `json:"status"`
	Desc        string             `json:"desc"`
	ACL         string             `json:"acl"`
	PO          string             `json:"PO"`
	QD          string             `json:"QD"`
	RD          string             `json:"RD"`
	Reviewer    flexibleStringList `json:"reviewer"`
	CreatedBy   string             `json:"createdBy"`
	CreatedDate string             `json:"createdDate"`
	ProgramName string             `json:"programName"`
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

func (w productV2Wire) toProduct() (*Product, error) {
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	program, err := jsonNumberToInt(w.Program, "program")
	if err != nil {
		return nil, err
	}
	line, err := jsonNumberToInt(w.Line, "line")
	if err != nil {
		return nil, err
	}
	return &Product{
		ID:          id,
		Name:        w.Name,
		Code:        w.Code,
		Program:     program,
		Line:        line,
		Type:        w.Type,
		Status:      w.Status,
		Desc:        w.Desc,
		ACL:         w.ACL,
		PO:          w.PO,
		QD:          w.QD,
		RD:          w.RD,
		Reviewer:    []string(w.Reviewer),
		CreatedBy:   w.CreatedBy,
		CreatedDate: w.CreatedDate,
		ProgramName: w.ProgramName,
	}, nil
}

func jsonNumberToInt(n json.Number, field string) (int, error) {
	if n == "" {
		return 0, nil
	}
	v, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("decode %s %q: %w", field, n.String(), err)
	}
	return int(v), nil
}

func productPath(id int) string {
	return productsPath + "/" + strconv.Itoa(id)
}

const productsPath = apiV2PathPrefix + "products"

func (c *Client) GetProduct(ctx context.Context, id int) (*Product, error) {
	body, status, err := c.doV2Request(ctx, http.MethodGet, productPath(id), nil, nil)
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
		if isNotFoundReason(resp.ZentaoFailReason()) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	return resp.Product.toProduct()
}

func (c *Client) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	body, status, err := c.doV2Request(ctx, http.MethodPost, productsPath, nil, p)
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

func (c *Client) UpdateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProduct: missing id")
	}
	body, status, err := c.doV2Request(ctx, http.MethodPut, productPath(p.ID), nil, p)
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
		if isNotFoundReason(resp.ZentaoFailReason()) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	return c.GetProduct(ctx, p.ID)
}

func (c *Client) DeleteProduct(ctx context.Context, id int) error {
	body, status, err := c.doV2Request(ctx, http.MethodDelete, productPath(id), nil, nil)
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
	if isNotFoundReason(resp.ZentaoFailReason()) {
		return nil
	}
	return apiError(status, body)
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
