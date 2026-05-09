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
	PO            types.String `tfsdk:"po"`
	QD            types.String `tfsdk:"qd"`
	RD            types.String `tfsdk:"rd"`
	Desc          types.String `tfsdk:"desc"`
	Code          types.String `tfsdk:"code"`
	Status        types.String `tfsdk:"status"`
	Lifetime      types.String `tfsdk:"lifetime"`
	OpenedBy      types.String `tfsdk:"opened_by"`
	OpenedDate    types.String `tfsdk:"opened_date"`
	LastEditedBy  types.String `tfsdk:"last_edited_by"`
	RealBegan     types.String `tfsdk:"real_began"`
	RealEnd       types.String `tfsdk:"real_end"`
	Progress      types.String `tfsdk:"progress"`
	TeamCount     types.String `tfsdk:"team_count"`
	Budget        types.String `tfsdk:"budget"`
	BudgetUnit    types.String `tfsdk:"budget_unit"`
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
			"po":             schema.StringAttribute{Description: "Product Owner username.", Computed: true},
			"qd":             schema.StringAttribute{Description: "QA Lead username.", Computed: true},
			"rd":             schema.StringAttribute{Description: "Release Lead username.", Computed: true},
			"desc":           schema.StringAttribute{Description: "Description.", Computed: true},
			"code":           schema.StringAttribute{Description: "Project short code.", Computed: true},
			"status":         schema.StringAttribute{Description: "Project status.", Computed: true},
			"lifetime":       schema.StringAttribute{Description: "Project lifetime classification.", Computed: true},
			"opened_by":      schema.StringAttribute{Description: "Creator username.", Computed: true},
			"opened_date":    schema.StringAttribute{Description: "Creation timestamp.", Computed: true},
			"last_edited_by": schema.StringAttribute{Description: "Last-editor username.", Computed: true},
			"real_began":     schema.StringAttribute{Description: "Actual start date.", Computed: true},
			"real_end":       schema.StringAttribute{Description: "Actual end date.", Computed: true},
			"progress":       schema.StringAttribute{Description: "Completion progress percentage.", Computed: true},
			"team_count":     schema.StringAttribute{Description: "Total team members.", Computed: true},
			"budget":         schema.StringAttribute{Description: "Project budget amount.", Computed: true},
			"budget_unit":    schema.StringAttribute{Description: "Currency unit of the budget.", Computed: true},
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
	id, err := strconv.Atoi(cfg.ID.ValueString())
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
	productsSrc := fetched.Products
	if productsSrc == nil {
		productsSrc = []int{}
	}
	products, diags := types.ListValueFrom(ctx, types.Int64Type, productsSrc)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, projectDataSourceModel{
		ID:            types.StringValue(strconv.Itoa(fetched.ID)),
		Name:          types.StringValue(fetched.Name),
		Model:         types.StringValue(fetched.Model),
		Begin:         types.StringValue(fetched.Begin),
		End:           types.StringValue(fetched.End),
		Program:       types.Int64Value(int64(fetched.Parent)),
		Products:      products,
		WorkflowGroup: types.Int64Value(int64(fetched.WorkflowGroup)),
		Multiple:      multipleWireToBool(fetched.Multiple),
		ACL:           types.StringValue(fetched.ACL),
		PM:            types.StringValue(fetched.PM),
		PO:            types.StringValue(fetched.PO),
		QD:            types.StringValue(fetched.QD),
		RD:            types.StringValue(fetched.RD),
		Desc:          types.StringValue(fetched.Desc),
		Code:          types.StringValue(fetched.Code),
		Status:        types.StringValue(fetched.Status),
		Lifetime:      types.StringValue(fetched.Lifetime),
		OpenedBy:      types.StringValue(fetched.OpenedBy),
		OpenedDate:    types.StringValue(fetched.OpenedDate),
		LastEditedBy:  types.StringValue(fetched.LastEditedBy),
		RealBegan:     types.StringValue(fetched.RealBegan),
		RealEnd:       types.StringValue(fetched.RealEnd),
		Progress:      types.StringValue(fetched.Progress),
		TeamCount:     types.StringValue(fetched.TeamCount),
		Budget:        types.StringValue(fetched.Budget),
		BudgetUnit:    types.StringValue(fetched.BudgetUnit),
	})...)
}
