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
	_ datasource.DataSource              = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectDataSource)(nil)
)

type projectDataSource struct {
	client *zentaoapi.Client
}

type projectDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Model         types.String `tfsdk:"model"`
	Begin         types.String `tfsdk:"begin"`
	End           types.String `tfsdk:"end"`
	Program       types.Int64  `tfsdk:"program"`
	Products      types.List   `tfsdk:"products"`
	WorkflowGroup types.Int64  `tfsdk:"workflow_group"`
	Multiple      types.Bool   `tfsdk:"multiple"`
	ACL           types.String `tfsdk:"acl"`
	PM            types.String `tfsdk:"pm"`
	Desc          types.String `tfsdk:"desc"`
	Status        types.String `tfsdk:"status"`
}

func NewProjectDataSource() datasource.DataSource { return &projectDataSource{} }

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao project by its numeric id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao project ID.",
				Required:    true,
			},
			"name":           schema.StringAttribute{Description: "Project display name.", Computed: true},
			"model":          schema.StringAttribute{Description: "Project methodology.", Computed: true},
			"begin":          schema.StringAttribute{Description: "Planned start date (YYYY-MM-DD).", Computed: true},
			"end":            schema.StringAttribute{Description: "Planned end date (YYYY-MM-DD).", Computed: true},
			"program":        schema.Int64Attribute{Description: "Parent program id (0 = no parent).", Computed: true},
			"products":       schema.ListAttribute{Description: "Associated product IDs.", Computed: true, ElementType: types.Int64Type},
			"workflow_group": schema.Int64Attribute{Description: "Workflow scheme id.", Computed: true},
			"multiple":       schema.BoolAttribute{Description: "Whether iterations (sprints) are enabled.", Computed: true},
			"acl":            schema.StringAttribute{Description: "Access control (`open` / `private` / `custom`).", Computed: true},
			"pm":             schema.StringAttribute{Description: "Project Manager username.", Computed: true},
			"desc":           schema.StringAttribute{Description: "Description.", Computed: true},
			"status":         schema.StringAttribute{Description: "Lifecycle status (wait / doing / suspended / closed).", Computed: true},
		},
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg projectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"Invalid id",
			fmt.Sprintf("id must be a numeric string, got %q", cfg.ID.ValueString()),
		)
		return
	}
	fetched, err := d.client.GetProject(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Project not found",
			fmt.Sprintf("ZenTao has no project with id=%d", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read project failed", err.Error())
		return
	}
	productsSrc := deref(fetched.Products)
	if productsSrc == nil {
		productsSrc = []int64{}
	}
	products, diags := types.ListValueFrom(ctx, types.Int64Type, productsSrc)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, projectDataSourceModel{
		ID:            types.StringValue(strconv.FormatInt(deref(fetched.ID), 10)),
		Name:          types.StringValue(deref(fetched.Name)),
		Model:         types.StringValue(deref(fetched.Model)),
		Begin:         types.StringValue(deref(fetched.Begin)),
		End:           types.StringValue(deref(fetched.End)),
		Program:       types.Int64Value(deref(fetched.Parent)),
		Products:      products,
		WorkflowGroup: types.Int64Value(deref(fetched.WorkflowGroup)),
		Multiple:      types.BoolValue(deref(fetched.Multiple)),
		ACL:           types.StringValue(deref(fetched.ACL)),
		PM:            types.StringValue(deref(fetched.PM)),
		Desc:          types.StringValue(deref(fetched.Desc)),
		Status:        types.StringValue(deref(fetched.Status)),
	})...)
}
