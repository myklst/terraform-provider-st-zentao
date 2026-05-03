package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ datasource.DataSource              = (*productDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*productDataSource)(nil)
)

type productDataSource struct {
	client *zentaoapi.Client
}

type productDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Code        types.String `tfsdk:"code"`
	Status      types.String `tfsdk:"status"`
	Description types.String `tfsdk:"description"`
	ACL         types.String `tfsdk:"acl"`
	Type        types.String `tfsdk:"type"`
}

func NewProductDataSource() datasource.DataSource { return &productDataSource{} }

func (d *productDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (d *productDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao product by its numeric id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao product ID (stringified).",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Product display name.",
				Computed:    true,
			},
			"code": schema.StringAttribute{
				Description: "Product short code (slug).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Product status (e.g. \"normal\", \"closed\").",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Product description (mapped from ZenTao 'desc').",
				Computed:    true,
			},
			"acl": schema.StringAttribute{
				Description: "Access control (e.g. \"open\", \"private\").",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Product type (e.g. \"normal\", \"branch\", \"platform\").",
				Computed:    true,
			},
		},
	}
}

func (d *productDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*zentaoapi.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("got %T, want *zentaoapi.Client", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *productDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg productDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"Invalid id",
			fmt.Sprintf("id must be a numeric string, got %q", cfg.ID.ValueString()),
		)
		return
	}
	fetched, err := d.client.GetProduct(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Product not found",
			fmt.Sprintf("ZenTao has no product with id=%d", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read product failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, productDataSourceModel{
		ID:          types.StringValue(strconv.Itoa(fetched.ID)),
		Name:        types.StringValue(fetched.Name),
		Code:        types.StringValue(fetched.Code),
		Status:      types.StringValue(fetched.Status),
		Description: types.StringValue(fetched.Description),
		ACL:         types.StringValue(fetched.ACL),
		Type:        types.StringValue(fetched.Type),
	})...)
}
