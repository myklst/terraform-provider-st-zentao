package zentao

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*zentaoProvider)(nil)

type zentaoProvider struct{}

type zentaoProviderModel struct {
	URL      types.String `tfsdk:"url"`
	Account  types.String `tfsdk:"account"`
	Password types.String `tfsdk:"password"`
}

func New() provider.Provider { return &zentaoProvider{} }

func (p *zentaoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "st-zentao"
}

func (p *zentaoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for ZenTao (self-hosted open-source / Pro / Biz / Max). " +
			"Requires url, account, and password; account/password fall back to ZENTAO_ACCOUNT and ZENTAO_PASSWORD env vars.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "Base URL of the ZenTao instance (e.g. https://zentao.example.com).",
				Required:    true,
			},
			"account": schema.StringAttribute{
				Description: "ZenTao account name. Falls back to ZENTAO_ACCOUNT env var.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "ZenTao account password. Falls back to ZENTAO_PASSWORD env var.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *zentaoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data zentaoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := Config{
		URL:      coalesce(data.URL.ValueString(), ""),
		Account:  coalesce(data.Account.ValueString(), os.Getenv("ZENTAO_ACCOUNT")),
		Password: coalesce(data.Password.ValueString(), os.Getenv("ZENTAO_PASSWORD")),
	}

	if cfg.URL == "" {
		resp.Diagnostics.AddAttributeError(path.Root("url"),
			"Missing url",
			"The url attribute is required.")
	}
	if cfg.Account == "" {
		resp.Diagnostics.AddAttributeError(path.Root("account"),
			"Missing account",
			"Provide account in the provider block or set ZENTAO_ACCOUNT.")
	}
	if cfg.Password == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"),
			"Missing password",
			"Provide password in the provider block or set ZENTAO_PASSWORD.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := cfg.Client()
	if err != nil {
		resp.Diagnostics.AddError("ZenTao client init failed", err.Error())
		return
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *zentaoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProductResource,
		NewProgramResource,
		NewProgramParentAttachmentResource,
		NewProjectResource,
		NewGroupResource,
	}
}

func (p *zentaoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProductDataSource,
		NewProgramDataSource,
		NewProjectDataSource,
		NewGroupDataSource,
	}
}

func coalesce(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
