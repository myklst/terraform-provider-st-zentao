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
	expected := []string{"id", "name", "code", "status", "description", "acl", "type"}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range []string{"name", "code", "status", "description", "acl", "type"} {
		if !resp.Schema.Attributes[attr].IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if resp.Schema.Attributes[attr].IsRequired() || resp.Schema.Attributes[attr].IsOptional() {
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
	code := uniqueCode("ds")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				// Create a product, then immediately look it up via the data source.
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "src" {
  name = "DS Source"
  code = %q
}

data "st-zentao_product" "by_id" {
  id = st-zentao_product.src.id
}
`, code),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.st-zentao_product.by_id", "name", "DS Source"),
					resource.TestCheckResourceAttrPair("data.st-zentao_product.by_id", "id", "st-zentao_product.src", "id"),
					resource.TestCheckResourceAttrSet("data.st-zentao_product.by_id", "status"),
				),
			},
		},
	})
}
