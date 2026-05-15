package zentao

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProductDataSource_Schema(t *testing.T) {
	d := NewProductDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range []string{
		"code", "name", "program", "line", "type", "desc",
		"acl", "po", "qd", "rd", "reviewer", "groups", "whitelist",
		"status",
	} {
		a := resp.Schema.Attributes[attr]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must not be Required/Optional (Computed-only)", attr)
		}
	}
}

func TestProductDataSource_Metadata(t *testing.T) {
	d := NewProductDataSource()
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_product" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestAccProductDataSource_basic(t *testing.T) {
	name := uniqueName("ds")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "src" {
  name = %q
}

data "st-zentao_product" "by_id" {
  id = st-zentao_product.src.id
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.st-zentao_product.by_id", "name", name),
					resource.TestCheckResourceAttrPair("data.st-zentao_product.by_id", "id", "st-zentao_product.src", "id"),
					resource.TestCheckResourceAttrSet("data.st-zentao_product.by_id", "status"),
					resource.TestCheckResourceAttrSet("data.st-zentao_product.by_id", "type"),
					resource.TestCheckResourceAttrSet("data.st-zentao_product.by_id", "acl"),
				),
			},
		},
	})
}
