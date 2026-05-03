package zentaoapi

import (
	"errors"
	"strings"
	"testing"
)

func TestZentaoFailReason_PrefersError(t *testing.T) {
	cases := []struct {
		name string
		env  ZentaoResponse
		want string
	}{
		{"v2 error field", ZentaoResponse{Error: "name exists"}, "name exists"},
		{"v1 reason field", ZentaoResponse{Reason: "please login"}, "please login"},
		{"v2 wins when both present", ZentaoResponse{Error: "v2 msg", Reason: "v1 msg"}, "v2 msg"},
		{"empty", ZentaoResponse{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.env.ZentaoFailReason(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestAPIError_RedactsPassword(t *testing.T) {
	e := &APIError{
		HTTPStatus:   401,
		ZentaoStatus: "fail",
		Reason:       "auth failure",
		RawBody:      []byte(`{"account":"admin","password":"secret123","reason":"bad"}`),
	}
	s := e.Error()
	if strings.Contains(s, "secret123") {
		t.Fatalf("password leaked in error string: %s", s)
	}
	if !strings.Contains(s, `"password":"***"`) {
		t.Fatalf("expected redaction marker in %s", s)
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
