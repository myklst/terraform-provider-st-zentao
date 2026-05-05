package zentaoapi

import (
	"encoding/json"
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
		{"v2 message field", ZentaoResponse{Message: "Product does not exist."}, "Product does not exist."},
		{"v1 reason field", ZentaoResponse{Reason: "please login"}, "please login"},
		{"error wins over message", ZentaoResponse{Error: "e", Message: "m", Reason: "r"}, "e"},
		{"message wins over reason", ZentaoResponse{Message: "m", Reason: "r"}, "m"},
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

func TestCtrlEnvelope_FailReason(t *testing.T) {
	cases := []struct {
		name string
		env  CtrlEnvelope
		want string
	}{
		{"error field", CtrlEnvelope{Error: "duplicate"}, "duplicate"},
		{"message field", CtrlEnvelope{Message: "user not exist"}, "user not exist"},
		{"reason field", CtrlEnvelope{Reason: "please login"}, "please login"},
		{"error wins", CtrlEnvelope{Error: "e", Message: "m", Reason: "r"}, "e"},
		{"empty", CtrlEnvelope{}, ""},
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

func TestIsNotFoundReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"Product does not exist.", true},
		{"User not found", true},
		{"NOT EXIST", true},
		{"resource Not Found here", true},
		{"name already exists", false},
		{"please login", false},
		{"", false},
		{"some random failure", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			if got := isNotFoundReason(tc.reason); got != tc.want {
				t.Fatalf("isNotFoundReason(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestIsLoginRedirectReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"please login", true},
		{"Please Login", true},
		{"PLEASE LOGIN.", true},
		{"请重新登录", true},
		{"请登录", true},
		{"session expired", true},
		{"Product does not exist.", false},
		{"name exists", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			if got := isLoginRedirectReason(tc.reason); got != tc.want {
				t.Fatalf("isLoginRedirectReason(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestIsUnauthorizedReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"wrong password", true},
		{"Incorrect credentials", true},
		{"invalid account", true},
		{"密码错误", true},
		{"账号或密码错误", true},
		{"认证失败", true},
		{"Product does not exist.", false},
		{"please login", false}, // login redirect, not bad-creds
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			if got := isUnauthorizedReason(tc.reason); got != tc.want {
				t.Fatalf("isUnauthorizedReason(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestDecodeData(t *testing.T) {
	type sample struct {
		Title    string         `json:"title"`
		Products map[string]any `json:"products"`
	}

	t.Run("V1 string-encoded JSON object", func(t *testing.T) {
		raw := `"{\"title\":\"产品\",\"products\":{\"218\":\"ds_x\"}}"`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "产品" || got.Products["218"] != "ds_x" {
			t.Fatalf("unexpected decode: %+v", got)
		}
	})

	t.Run("V2 direct object", func(t *testing.T) {
		raw := `{"title":"x","products":{"1":"a"}}`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "x" {
			t.Fatalf("unexpected decode: %+v", got)
		}
	})

	t.Run("direct array", func(t *testing.T) {
		raw := `[1,2,3]`
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(raw)}
		var got []int
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 1 {
			t.Fatalf("unexpected decode: %v", got)
		}
	})

	t.Run("null data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`null`)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "" {
			t.Fatalf("expected zero-value, got %+v", got)
		}
	})

	t.Run("absent data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success"}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty string data", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`""`)}
		var got sample
		if err := DecodeData(env, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-JSON data is error", func(t *testing.T) {
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`garbage`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected error, got nil with %+v", got)
		}
	})

	t.Run("string-encoded with malformed inner JSON", func(t *testing.T) {
		// Outer is a valid quoted string, but inner content isn't JSON.
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`"not json"`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected inner-decode error")
		}
	})

	t.Run("direct object with type mismatch", func(t *testing.T) {
		// target expects object, data is array — should error from unmarshal.
		env := CtrlEnvelope{Status: "success", Data: json.RawMessage(`[1,2,3]`)}
		var got sample
		if err := DecodeData(env, &got); err == nil {
			t.Fatalf("expected unmarshal type error")
		}
	})
}

func TestClassifyCtrlError(t *testing.T) {
	t.Run("not found via message", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Message: "User does not exist"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("unauthorized via reason", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "wrong password"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("unauthorized chinese 密码错误", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "用户名或密码错误"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("generic fail → APIError", func(t *testing.T) {
		raw := []byte(`{"status":"fail","message":"validation failed"}`)
		env := CtrlEnvelope{Status: "fail", Message: "validation failed"}
		err := classifyCtrlError(200, env, raw)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *APIError, got %v", err)
		}
		if apiErr.ZentaoStatus != "fail" || apiErr.Reason != "validation failed" {
			t.Fatalf("unexpected APIError: %+v", apiErr)
		}
	})

	t.Run("login redirect ≠ unauthorized", func(t *testing.T) {
		env := CtrlEnvelope{Status: "fail", Reason: "please login"}
		err := classifyCtrlError(200, env, []byte(`{}`))
		// session-expired reason should NOT collapse into ErrUnauthorized;
		// it's a transport-layer concern handled before classifyCtrlError.
		if errors.Is(err, ErrUnauthorized) {
			t.Fatalf("please-login should not be classified as ErrUnauthorized: %v", err)
		}
	})
}
