package zentao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
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

// uniqueName returns a name that is unlikely to collide with concurrent
// or prior test runs. ZenTao enforces name uniqueness within a program.
func uniqueName(prefix string) string {
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
	name := uniqueName("acc")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "p" {
  name        = %q
  desc        = "initial"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("st-zentao_product.p", "name", name),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "id"),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "type"),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "acl"),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "status"),
					resource.TestCheckResourceAttrSet("st-zentao_product.p", "created_by"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "p" {
  name        = %q
  desc        = "updated"
  acl         = "private"
}`, name+"-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("st-zentao_product.p", "name", name+"-renamed"),
					resource.TestCheckResourceAttr("st-zentao_product.p", "acl", "private"),
					resource.TestCheckResourceAttr("st-zentao_product.p", "desc", "updated"),
				),
			},
		},
	})
}

func TestAccProductResource_invalidEnumRejected(t *testing.T) {
	name := uniqueName("enum")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_product" "p" {
  name = %q
  type = "not-a-real-type"
}`, name),
				ExpectError: regexp.MustCompile(`(?i)must be one of`),
			},
		},
	})
}

func TestAccProductResource_import(t *testing.T) {
	name := uniqueName("imp")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = %q }`, name),
			},
			{
				ResourceName:      "st-zentao_product.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccProductResource_disappears(t *testing.T) {
	name := uniqueName("dis")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = %q }`, name),
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["st-zentao_product.p"]
						if !ok {
							return errors.New("resource missing from state")
						}
						id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
						if err != nil {
							return err
						}
						c, err := zentaoapi.NewClient(os.Getenv("ZENTAO_URL"), os.Getenv("ZENTAO_ACCOUNT"), os.Getenv("ZENTAO_PASSWORD"))
						if err != nil {
							return err
						}
						return c.DeleteProduct(context.Background(), id)
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProvider_InvalidCredentials(t *testing.T) {
	if os.Getenv("ZENTAO_URL") == "" {
		t.Skip("ZENTAO_URL must be set")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "st-zentao" {
  url      = %q
  account  = "definitely-not-a-real-user"
  password = "wrong"
}

resource "st-zentao_product" "p" {
  name = %q
}
`, os.Getenv("ZENTAO_URL"), uniqueName("badcred")),
				ExpectError: regexp.MustCompile(`invalid credentials|wrong account`),
			},
		},
	})
}
