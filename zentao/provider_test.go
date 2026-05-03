package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
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
	if !resp.Schema.Attributes["account"].IsSensitive() {
		t.Error("account must be sensitive")
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
