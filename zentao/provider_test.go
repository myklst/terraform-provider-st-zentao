package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

func TestProductResource_Metadata(t *testing.T) {
	r := NewProductResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_product" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}
