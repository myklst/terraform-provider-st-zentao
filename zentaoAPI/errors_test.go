package zentaoapi

import (
	"errors"
	"strings"
	"testing"
)

// errors_test.go covers the cross-transport error model defined in
// errors.go: sentinels, *APIError redaction, and the reason
// classifiers (isNotFoundReason, isUnauthorizedReason). Per-transport
// envelope tests live in their respective *_transport_test.go files.

func TestSentinels(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatalf("ErrNotFound sentinel broken")
	}
	if !errors.Is(ErrUnauthorized, ErrUnauthorized) {
		t.Fatalf("ErrUnauthorized sentinel broken")
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

func TestIsDuplicateNameReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		// observed: real ZenTao Max 8.1 reply on program-edit with a
		// same-name sibling at the target parent level
		{"name: 『项目集名称』已经有『Business Support』这条记录了。", true},
		{"『名称』已经有『X』这条记录了。", true},
		{"name: Program Name 'X' has been used.", true}, // en locale
		{"『项目集名称』不能为空。", false},                          // notempty, not unique
		{"Product does not exist.", false},
		{"please login", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			if got := isDuplicateNameReason(tc.reason); got != tc.want {
				t.Fatalf("isDuplicateNameReason(%q) = %v, want %v", tc.reason, got, tc.want)
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
		{"登录失败，请检查您的用户名或密码是否填写正确。", true}, // observed: real ZenTao Max 8.1 reply
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
