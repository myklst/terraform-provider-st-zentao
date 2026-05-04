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
	_ datasource.DataSource              = (*programDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*programDataSource)(nil)
)

type programDataSource struct {
	client *zentaoapi.Client
}

type programDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Code types.String `tfsdk:"code"`

	Name        types.String `tfsdk:"name"`
	Begin       types.String `tfsdk:"begin"`
	End         types.String `tfsdk:"end"`
	PM          types.String `tfsdk:"pm"`
	Description types.String `tfsdk:"description"`

	Status     types.String `tfsdk:"status"`
	Parent     types.Int64  `tfsdk:"parent"`
	Type       types.String `tfsdk:"type"`
	Category   types.String `tfsdk:"category"`
	ACL        types.String `tfsdk:"acl"`
	PO         types.String `tfsdk:"po"`
	QD         types.String `tfsdk:"qd"`
	RD         types.String `tfsdk:"rd"`
	Budget     types.String `tfsdk:"budget"`
	BudgetUnit types.String `tfsdk:"budget_unit"`
	OpenedBy   types.String `tfsdk:"opened_by"`
	OpenedDate types.String `tfsdk:"opened_date"`
	RealBegan  types.String `tfsdk:"real_began"`
	RealEnd    types.String `tfsdk:"real_end"`
	Progress   types.String `tfsdk:"progress"`
	TeamCount  types.String `tfsdk:"team_count"`
}

func NewProgramDataSource() datasource.DataSource { return &programDataSource{} }

func (d *programDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (d *programDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao program (project portfolio) by its numeric id. " +
			"All fields the v2 GET endpoint surfaces are exposed as Computed attributes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao program ID (stringified).",
				Required:    true,
			},
			"code":        schema.StringAttribute{Description: "Program short code (server-managed).", Computed: true},
			"name":        schema.StringAttribute{Description: "Program display name.", Computed: true},
			"begin":       schema.StringAttribute{Description: "Planned start date (YYYY-MM-DD).", Computed: true},
			"end":         schema.StringAttribute{Description: "Planned end date (YYYY-MM-DD).", Computed: true},
			"pm":          schema.StringAttribute{Description: "Program Manager username.", Computed: true},
			"description": schema.StringAttribute{Description: "Program description (mapped from ZenTao 'desc').", Computed: true},
			"status":      schema.StringAttribute{Description: "Server-managed program status.", Computed: true},
			"parent":      schema.Int64Attribute{Description: "Parent program ID (0 if top-level).", Computed: true},
			"type":        schema.StringAttribute{Description: "Program type.", Computed: true},
			"category":    schema.StringAttribute{Description: "Program category.", Computed: true},
			"acl":         schema.StringAttribute{Description: "Access control level.", Computed: true},
			"po":          schema.StringAttribute{Description: "Product Owner username.", Computed: true},
			"qd":          schema.StringAttribute{Description: "QA Lead username.", Computed: true},
			"rd":          schema.StringAttribute{Description: "Release Lead username.", Computed: true},
			"budget":      schema.StringAttribute{Description: "Program budget amount.", Computed: true},
			"budget_unit": schema.StringAttribute{Description: "Currency unit of the budget.", Computed: true},
			"opened_by":   schema.StringAttribute{Description: "Creator username.", Computed: true},
			"opened_date": schema.StringAttribute{Description: "Creation timestamp.", Computed: true},
			"real_began":  schema.StringAttribute{Description: "Actual start date.", Computed: true},
			"real_end":    schema.StringAttribute{Description: "Actual end date.", Computed: true},
			"progress":    schema.StringAttribute{Description: "Completion progress percentage.", Computed: true},
			"team_count":  schema.StringAttribute{Description: "Total team members.", Computed: true},
		},
	}
}

func (d *programDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *programDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg programDataSourceModel
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
	fetched, err := d.client.GetProgram(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Program not found",
			fmt.Sprintf("ZenTao has no program with id=%d", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read program failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, programDataSourceModel{
		ID:          types.StringValue(strconv.Itoa(fetched.ID)),
		Code:        types.StringValue(fetched.Code),
		Name:        types.StringValue(fetched.Name),
		Begin:       types.StringValue(fetched.Begin),
		End:         types.StringValue(fetched.End),
		PM:          types.StringValue(fetched.PM),
		Description: types.StringValue(fetched.Description),
		Status:      types.StringValue(fetched.Status),
		Parent:      types.Int64Value(int64(fetched.Parent)),
		Type:        types.StringValue(fetched.Type),
		Category:    types.StringValue(fetched.Category),
		ACL:         types.StringValue(fetched.ACL),
		PO:          types.StringValue(fetched.PO),
		QD:          types.StringValue(fetched.QD),
		RD:          types.StringValue(fetched.RD),
		Budget:      types.StringValue(fetched.Budget),
		BudgetUnit:  types.StringValue(fetched.BudgetUnit),
		OpenedBy:    types.StringValue(fetched.OpenedBy),
		OpenedDate:  types.StringValue(fetched.OpenedDate),
		RealBegan:   types.StringValue(fetched.RealBegan),
		RealEnd:     types.StringValue(fetched.RealEnd),
		Progress:    types.StringValue(fetched.Progress),
		TeamCount:   types.StringValue(fetched.TeamCount),
	})...)
}
