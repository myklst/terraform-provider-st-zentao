package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProvider_Schema_HasExpectedAttributes(t *testing.T) {
	p := New()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"url", "account", "password"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if !resp.Schema.Attributes["password"].IsSensitive() {
		t.Error("password must be sensitive")
	}
}

func TestProvider_Metadata(t *testing.T) {
	p := New()
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)
	if resp.TypeName != "st-zentao" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestProductResource_Schema(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	expected := []string{
		"id", "code", "name", "program", "line", "type", "desc",
		"acl", "po", "qd", "rd", "reviewer",
		"status", "created_by", "created_date", "program_name",
	}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name must be Required")
	}
	for _, computedOnly := range []string{"id", "code", "status", "created_by", "created_date", "program_name"} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)", computedOnly, a.IsRequired(), a.IsOptional())
		}
	}
	for _, optional := range []string{"program", "line", "type", "desc", "acl", "po", "qd", "rd", "reviewer"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)", optional, a.IsOptional(), a.IsComputed())
		}
	}
}

// program_name is server-derived from `program`. Pinning it via
// UseStateForUnknown causes "Provider produced inconsistent result after apply"
// when `program` changes (state holds the old program's name; Update returns
// the new one). Regression guard for that bug.
func TestProductResource_ProgramName_NoUseStateForUnknown(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	a, ok := resp.Schema.Attributes["program_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("program_name must be a StringAttribute, got %T", resp.Schema.Attributes["program_name"])
	}
	if len(a.PlanModifiers) != 0 {
		t.Errorf("program_name must have no plan modifiers (got %d) — UseStateForUnknown causes inconsistent-after-apply when `program` changes", len(a.PlanModifiers))
	}
}

// po/qd/rd are role-username Optional+Computed fields where ZenTao backfills
// the requesting account when the request body omits the field. Pinning ""
// from state via UseStateForUnknown would force the next plan to claim
// `po=""`, while Update's refetch returns `"admin"` — Terraform then raises
// "Provider produced inconsistent result after apply". Regression guard:
// each field carries exactly one plan modifier (useStateUnlessEmpty), and
// that modifier must leave plan as Unknown when the prior state is "".
func TestProductResource_RoleFields_UseStateUnlessEmpty(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	for _, name := range []string{"po"} {
		a, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a StringAttribute, got %T", name, resp.Schema.Attributes[name])
		}
		if len(a.PlanModifiers) != 1 {
			t.Errorf("%s expected exactly 1 plan modifier, got %d", name, len(a.PlanModifiers))
			continue
		}
		// Empty state → plan must remain Unknown so server-side default wins.
		emptyResp := planmodifier.StringResponse{PlanValue: types.StringUnknown()}
		a.PlanModifiers[0].PlanModifyString(context.Background(),
			planmodifier.StringRequest{
				StateValue: types.StringValue(""),
				PlanValue:  types.StringUnknown(),
			}, &emptyResp)
		if !emptyResp.PlanValue.IsUnknown() {
			t.Errorf("%s: empty state must leave plan Unknown, got %v", name, emptyResp.PlanValue)
		}
		// Non-empty state → plan should be pinned to the prior state value.
		pinnedResp := planmodifier.StringResponse{PlanValue: types.StringUnknown()}
		a.PlanModifiers[0].PlanModifyString(context.Background(),
			planmodifier.StringRequest{
				StateValue: types.StringValue("alice"),
				PlanValue:  types.StringUnknown(),
			}, &pinnedResp)
		if pinnedResp.PlanValue.ValueString() != "alice" {
			t.Errorf("%s: non-empty state must be pinned into plan, got %v", name, pinnedResp.PlanValue)
		}
	}
}

func TestProductResource_Metadata(t *testing.T) {
	r := NewProductResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_product" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}
