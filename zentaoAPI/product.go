package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

func (c *Client) GetProduct(ctx context.Context, id int) (*Product, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "api.php", map[string]string{
		"m":         "product",
		"f":         "view",
		"productID": strconv.Itoa(id),
	}, nil)
	if err != nil {
		return nil, err
	}
	return parseProductResponse(body, http.StatusOK)
}

func parseProductResponse(body []byte, httpStatus int) (*Product, error) {
	var env ZentaoResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		if isNotFound(env.Reason) {
			return nil, ErrNotFound
		}
		return nil, &APIError{
			HTTPStatus:   httpStatus,
			ZentaoStatus: env.Status,
			Reason:       env.Reason,
			RawBody:      body,
		}
	}
	var p Product
	if err := decodeData(&env, &p); err != nil {
		return nil, fmt.Errorf("decode product: %w", err)
	}
	return &p, nil
}

func isNotFound(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "not exist") || strings.Contains(r, "not found")
}
