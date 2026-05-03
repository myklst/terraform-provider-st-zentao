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

func TestAccProductResource_requiresReplace(t *testing.T) {
	code1 := uniqueCode("acc1")
	code2 := uniqueCode("acc2")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = "X" code = %q }`, code1),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = "X" code = %q }`, code2),
				Check:  resource.TestCheckResourceAttr("st-zentao_product.p", "code", code2),
			},
		},
	})
}

func TestAccProductResource_import(t *testing.T) {
	code := uniqueCode("imp")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = "Import" code = %q }`, code),
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
	code := uniqueCode("dis")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`resource "st-zentao_product" "p" { name = "D" code = %q }`, code),
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["st-zentao_product.p"]
						if !ok {
							return errors.New("resource missing from state")
						}
						id, err := strconv.Atoi(rs.Primary.ID)
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
  name = "x"
  code = %q
}
`, os.Getenv("ZENTAO_URL"), uniqueCode("badcred")),
				ExpectError: regexpInvalidCredentials,
			},
		},
	})
}

var regexpInvalidCredentials = mustCompile("invalid credentials|wrong account")

func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
