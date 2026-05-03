package zentao

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var protoV6Factories = map[string]func() (tfprotov6.ProviderServer, error){
	"st-zentao": providerserver.NewProtocol6WithError(New()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ZENTAO_URL", "ZENTAO_ACCOUNT", "ZENTAO_PASSWORD"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s must be set for acceptance tests", k)
		}
	}
}

func uniqueCode(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func providerBlock() string {
	return fmt.Sprintf(`
provider "st-zentao" {
  url      = %q
  account  = %q
  password = %q
}
`, os.Getenv("ZENTAO_URL"), os.Getenv("ZENTAO_ACCOUNT"), os.Getenv("ZENTAO_PASSWORD"))
}

func TestAccProductResource_basic(t *testing.T) {
	code := uniqueCode("acc")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "p" {
  name        = "ACC Test"
  code        = %q
  description = "initial"
}`, code),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("st-zentao_product.p", "name", "ACC Test"),
					resource.TestCheckResourceAttr("st-zentao_product.p", "code", code),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "id"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "p" {
  name        = "ACC Test Updated"
  code        = %q
  description = "updated"
}`, code),
				Check: resource.TestCheckResourceAttr("st-zentao_product.p", "name", "ACC Test Updated"),
			},
		},
	})
}
