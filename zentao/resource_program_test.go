package zentao

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProgramResource_Schema(t *testing.T) {
	r := NewProgramResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, required := range []string{"name", "begin", "end"} {
		if !resp.Schema.Attributes[required].IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}
	// Writeable per form.php — Optional+Computed so the server can apply
	// defaults when the user leaves them unset, and prior state is
	// preserved across plans (UseStateForUnknown).
	for _, optional := range []string{"pm", "desc", "acl", "budget", "budget_unit", "whitelist"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)", optional, a.IsOptional(), a.IsComputed())
		}
	}
	// Server-managed fields surfaced by program-edit-{id} that must not
	// accept user input. parent moved here when it became read-only —
	// the attachment resource is the only writer.
	for _, computedOnly := range []string{"id", "parent", "status"} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)", computedOnly, a.IsRequired(), a.IsOptional())
		}
	}
}

func TestProgramResource_Metadata(t *testing.T) {
	r := NewProgramResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_program" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestAccProgramResource_basic(t *testing.T) {
	name := uniqueName("acc-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
  desc  = "initial"
}`, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "name", name),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "begin", "2026-01-01"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "end", "2026-12-31"),
					tfresource.TestCheckResourceAttrSet("st-zentao_program.p", "id"),
					tfresource.TestCheckResourceAttrSet("st-zentao_program.p", "status"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "2026-02-01"
  end   = "2026-11-30"
  desc  = "updated"
}`, name+"-renamed"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "name", name+"-renamed"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "begin", "2026-02-01"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "desc", "updated"),
				),
			},
		},
	})
}

func TestAccProgramResource_invalidDateRejected(t *testing.T) {
	name := uniqueName("baddate")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "01/01/2026"
  end   = "2026-12-31"
}`, name),
				ExpectError: regexp.MustCompile(`(?i)YYYY-MM-DD`),
			},
		},
	})
}

func TestAccProgramResource_import(t *testing.T) {
	name := uniqueName("imp-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}`, name),
			},
			{
				ResourceName:      "st-zentao_program.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
