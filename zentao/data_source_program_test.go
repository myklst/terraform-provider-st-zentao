package zentao

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProgramDataSource_Schema(t *testing.T) {
	d := NewProgramDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	expected := []string{
		"id", "code", "name", "begin", "end", "pm", "description",
		"status", "parent", "type", "category", "acl", "po", "qd", "rd",
		"budget", "budget_unit", "opened_by", "opened_date",
		"real_began", "real_end", "progress", "team_count",
	}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range expected {
		if attr == "id" {
			continue
		}
		a := resp.Schema.Attributes[attr]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must not be Required/Optional (Computed-only)", attr)
		}
	}
}

func TestProgramDataSource_Metadata(t *testing.T) {
	d := NewProgramDataSource()
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_program" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestAccProgramDataSource_basic(t *testing.T) {
	name := uniqueName("ds-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "src" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}

data "st-zentao_program" "by_id" {
  id = st-zentao_program.src.id
}
`, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("data.st-zentao_program.by_id", "name", name),
					tfresource.TestCheckResourceAttr("data.st-zentao_program.by_id", "begin", "2026-01-01"),
					tfresource.TestCheckResourceAttrPair("data.st-zentao_program.by_id", "id", "st-zentao_program.src", "id"),
					tfresource.TestCheckResourceAttrSet("data.st-zentao_program.by_id", "status"),
					tfresource.TestCheckResourceAttrSet("data.st-zentao_program.by_id", "opened_by"),
				),
			},
		},
	})
}
