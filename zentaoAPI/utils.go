package zentaoapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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

// boolToOnOff serialises a Go bool into ZenTao's HTML-checkbox-style
// wire shape ("on" / "off"). Used by form fields whose server-side
// filter coerces anything other than the literal "on" to falsy —
// see docs/superpowers/specs/probe-project-controller.md §2 for the
// project `multiple` field that motivated this helper (probe showed
// `multiple=1`/`multiple=yes` silently stored as `0`, only `multiple=on`
// flips it). DB column remains integer 0/1; only the form filter cares.
func boolToOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// int64SliceFromIDKeyedMap extracts the integer keys of a map keyed by
// numeric string ids (e.g. `{"1": {...}, "3": {...}}` from ZenTao's
// `.data.products`, `.data.linkedProducts`, etc.) and returns them as a
// sorted ascending []int64. Map iteration order is non-deterministic;
// sorting keeps Terraform plan diffs stable.
func int64SliceFromIDKeyedMap(m map[string]json.RawMessage) ([]int64, error) {
	if len(m) == 0 {
		return []int64{}, nil
	}
	out := make([]int64, 0, len(m))
	for k := range m {
		var n int64
		if _, err := fmt.Sscanf(k, "%d", &n); err != nil {
			return nil, fmt.Errorf("id-keyed map: non-integer key %q: %w", k, err)
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
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
			*f = []string{}
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
