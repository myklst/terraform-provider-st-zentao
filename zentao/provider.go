package zentao

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = (*zentaoProvider)(nil)

type zentaoProvider struct{}

func New() provider.Provider { return &zentaoProvider{} }

func (p *zentaoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "st-zentao"
}

func (p *zentaoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{}
}

func (p *zentaoProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *zentaoProvider) Resources(_ context.Context) []func() resource.Resource { return nil }

func (p *zentaoProvider) DataSources(_ context.Context) []func() datasource.DataSource { return nil }
