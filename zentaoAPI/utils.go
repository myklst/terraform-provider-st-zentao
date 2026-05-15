package zentaoapi

import (
	"encoding/json"
	"fmt"
)

func deref[T any](p *T) T {
	if p == nil {
		var def T
		return def
	}
	return *p
}

// boolToIntStr serialises a Go bool into ZenTao's PHP-style wire shape
// ("0" / "1"), the inverse of jsonNumberToBool.
func boolToIntStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
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

func jsonNumberToBool(n json.Number) bool {
	return n == "1"
}

func strptr(s string) *string          { return &s }
func int64ptr(i int64) *int64          { return &i }
func boolptr(b bool) *bool             { return &b }
func strSlicePtr(s []string) *[]string { return &s }
