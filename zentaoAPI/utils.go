package zentaoapi

import (
	"encoding/json"
	"fmt"
	"html"
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

// decodeEntitiesPtr HTML-entity-decodes the pointee in place and returns
// the same pointer (nil stays nil). ZenTao persist title/name fields through
// htmlspecialchars(ENT_QUOTES), so a written
// apostrophe `'` reads back as `&#039;` (likewise `&`→`&amp;`, `<`→`&lt;`,
// `"`→`&quot;`); Decoding on read makes Get* return the canonical un-encoded
// form, which:
//   - keeps Terraform's create/update round-trip consistent (the configured
//     `'` equals the value the provider writes to state), and
//   - keeps the M-Z merge safe: Update resubmits the decoded baseline, so a
//     partial update (e.g. SetProgramParent, which leaves Name nil) cannot
//     re-encode an already-encoded value into `&amp;#039;`.
//
// html.UnescapeString is idempotent on already-raw text, so decoding a
// non-encoding server's response is a no-op. Decode belongs here in the API
// client (not the provider mapping) precisely because the merge baseline
// must be canonical before resubmission.
func decodeEntitiesPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := html.UnescapeString(*p)
	return &s
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
