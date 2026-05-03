package zentaoapi

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecodeData_DirectObject(t *testing.T) {
	env := &ZentaoResponse{
		Status: "success",
		Data:   json.RawMessage(`{"id":42,"name":"alpha"}`),
	}
	var got struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := decodeData(env, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 42 || got.Name != "alpha" {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeData_StringEncodedJSON(t *testing.T) {
	env := &ZentaoResponse{
		Status: "success",
		Data:   json.RawMessage(`"{\"id\":7,\"name\":\"beta\"}"`),
	}
	var got struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := decodeData(env, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 7 || got.Name != "beta" {
		t.Fatalf("got %+v", got)
	}
}

func TestAPIError_RedactsPassword(t *testing.T) {
	e := &APIError{
		HTTPStatus:   200,
		ZentaoStatus: "failed",
		Reason:       "auth failure",
		RawBody:      []byte(`{"account":"admin","password":"secret123","reason":"bad"}`),
	}
	s := e.Error()
	if containsString(s, "secret123") {
		t.Fatalf("password leaked in error string: %s", s)
	}
}

func TestSentinels(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatalf("ErrNotFound sentinel broken")
	}
	if !errors.Is(ErrUnauthorized, ErrUnauthorized) {
		t.Fatalf("ErrUnauthorized sentinel broken")
	}
}

func containsString(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
