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
	ID   types.String `tfsdk:"id"`
	Code types.String `tfsdk:"code"`

	Name     types.String `tfsdk:"name"`
	Program  types.Int64  `tfsdk:"program"`
	Line     types.Int64  `tfsdk:"line"`
	Type     types.String `tfsdk:"type"`
	Desc     types.String `tfsdk:"desc"`
	ACL      types.String `tfsdk:"acl"`
	PO       types.String `tfsdk:"po"`
	QD       types.String `tfsdk:"qd"`
	RD       types.String `tfsdk:"rd"`
	Reviewer types.List   `tfsdk:"reviewer"`

	Status      types.String `tfsdk:"status"`
	CreatedBy   types.String `tfsdk:"created_by"`
	CreatedDate types.String `tfsdk:"created_date"`
	ProgramName types.String `tfsdk:"program_name"`
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
				Description: "Numeric ZenTao product ID.",
				Required:    true,
			},
			"code":         schema.StringAttribute{Description: "Product short code.", Computed: true},
			"name":         schema.StringAttribute{Description: "Product display name.", Computed: true},
			"program":      schema.Int64Attribute{Description: "Associated program id.", Computed: true},
			"line":         schema.Int64Attribute{Description: "Associated product line id.", Computed: true},
			"type":         schema.StringAttribute{Description: "Product type (`normal` / `branch` / `platform`).", Computed: true},
			"desc":         schema.StringAttribute{Description: "Description.", Computed: true},
			"acl":          schema.StringAttribute{Description: "Access control (`open` / `private`).", Computed: true},
			"po":           schema.StringAttribute{Description: "Product Owner username.", Computed: true},
			"qd":           schema.StringAttribute{Description: "QA Lead username.", Computed: true},
			"rd":           schema.StringAttribute{Description: "Release Lead username.", Computed: true},
			"reviewer":     schema.ListAttribute{Description: "Reviewer usernames.", Computed: true, ElementType: types.StringType},
			"status":       schema.StringAttribute{Description: "Product status.", Computed: true},
			"created_by":   schema.StringAttribute{Description: "Creator username.", Computed: true},
			"created_date": schema.StringAttribute{Description: "Creation timestamp.", Computed: true},
			"program_name": schema.StringAttribute{Description: "Associated program name.", Computed: true},
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
	reviewerSrc := fetched.Reviewer
	if reviewerSrc == nil {
		reviewerSrc = []string{}
	}
	reviewers, diags := types.ListValueFrom(ctx, types.StringType, reviewerSrc)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, productDataSourceModel{
		ID:          types.StringValue(strconv.Itoa(fetched.ID)),
		Code:        types.StringValue(fetched.Code),
		Name:        types.StringValue(fetched.Name),
		Program:     types.Int64Value(int64(fetched.Program)),
		Line:        types.Int64Value(int64(fetched.Line)),
		Type:        types.StringValue(fetched.Type),
		Desc:        types.StringValue(fetched.Desc),
		ACL:         types.StringValue(fetched.ACL),
		PO:          types.StringValue(fetched.PO),
		QD:          types.StringValue(fetched.QD),
		RD:          types.StringValue(fetched.RD),
		Reviewer:    reviewers,
		Status:      types.StringValue(fetched.Status),
		CreatedBy:   types.StringValue(fetched.CreatedBy),
		CreatedDate: types.StringValue(fetched.CreatedDate),
		ProgramName: types.StringValue(fetched.ProgramName),
	})...)
}
