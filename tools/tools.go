//go:build tools
// +build tools

package tools

import (
	_ "github.com/cenkalti/backoff/v4"
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
	_ "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	_ "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)
