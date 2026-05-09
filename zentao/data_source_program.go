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
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Begin          types.String `tfsdk:"begin"`
	End            types.String `tfsdk:"end"`
	Parent         types.Int64  `tfsdk:"parent"`
	PM             types.String `tfsdk:"pm"`
	Desc           types.String `tfsdk:"desc"`
	ACL            types.String `tfsdk:"acl"`
	Budget         types.String `tfsdk:"budget"`
	BudgetUnit     types.String `tfsdk:"budget_unit"`
	Whitelist      types.String `tfsdk:"whitelist"`
	Code           types.String `tfsdk:"code"`
	Status         types.String `tfsdk:"status"`
	Type           types.String `tfsdk:"type"`
	Category       types.String `tfsdk:"category"`
	Lifetime       types.String `tfsdk:"lifetime"`
	Vision         types.String `tfsdk:"vision"`
	Attribute      types.String `tfsdk:"attribute"`
	Model          types.String `tfsdk:"model"`
	ProgramPath    types.String `tfsdk:"program_path"`
	Grade          types.Int64  `tfsdk:"grade"`
	Multiple       types.Bool   `tfsdk:"multiple"`
	Parallel       types.Bool   `tfsdk:"parallel"`
	HasProduct     types.Bool   `tfsdk:"has_product"`
	WorkflowGroup  types.Int64  `tfsdk:"workflow_group"`
	StoryType      types.String `tfsdk:"story_type"`
	Days           types.Int64  `tfsdk:"days"`
	FirstEnd       types.String `tfsdk:"first_end"`
	OpenedBy       types.String `tfsdk:"opened_by"`
	OpenedDate     types.String `tfsdk:"opened_date"`
	LastEditedBy   types.String `tfsdk:"last_edited_by"`
	LastEditedDate types.String `tfsdk:"last_edited_date"`
	RealBegan      types.String `tfsdk:"real_began"`
	RealEnd        types.String `tfsdk:"real_end"`
	ClosedBy       types.String `tfsdk:"closed_by"`
	ClosedDate     types.String `tfsdk:"closed_date"`
	ClosedReason   types.String `tfsdk:"closed_reason"`
	CanceledBy     types.String `tfsdk:"canceled_by"`
	CanceledDate   types.String `tfsdk:"canceled_date"`
	SuspendedDate  types.String `tfsdk:"suspended_date"`
	PO             types.String `tfsdk:"po"`
	QD             types.String `tfsdk:"qd"`
	RD             types.String `tfsdk:"rd"`
	Team           types.String `tfsdk:"team"`
	Progress       types.String `tfsdk:"progress"`
	Percent        types.String `tfsdk:"percent"`
	Estimate       types.String `tfsdk:"estimate"`
	Consumed       types.String `tfsdk:"consumed"`
	Left           types.String `tfsdk:"left"`
	TeamCount      types.Int64  `tfsdk:"team_count"`
}

func NewProgramDataSource() datasource.DataSource { return &programDataSource{} }

func (d *programDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (d *programDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao program by its numeric id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao program ID.",
				Required:    true,
			},
			"name":             schema.StringAttribute{Description: "Program display name.", Computed: true},
			"begin":            schema.StringAttribute{Description: "Planned start date (YYYY-MM-DD).", Computed: true},
			"end":              schema.StringAttribute{Description: "Planned end date (YYYY-MM-DD).", Computed: true},
			"parent":           schema.Int64Attribute{Description: "Parent program id (0 = top-level).", Computed: true},
			"pm":               schema.StringAttribute{Description: "Project Manager username.", Computed: true},
			"desc":             schema.StringAttribute{Description: "Description.", Computed: true},
			"acl":              schema.StringAttribute{Description: "Access control (`open` / `private` / `custom`).", Computed: true},
			"budget":           schema.StringAttribute{Description: "Program budget amount.", Computed: true},
			"budget_unit":      schema.StringAttribute{Description: "Currency unit of the budget.", Computed: true},
			"whitelist":        schema.StringAttribute{Description: "Comma-joined account list when `acl = \"custom\"`.", Computed: true},
			"code":             schema.StringAttribute{Description: "Program short code.", Computed: true},
			"status":           schema.StringAttribute{Description: "Program status.", Computed: true},
			"type":             schema.StringAttribute{Description: "Program type.", Computed: true},
			"category":         schema.StringAttribute{Description: "Program category.", Computed: true},
			"lifetime":         schema.StringAttribute{Description: "Program lifetime classification.", Computed: true},
			"vision":           schema.StringAttribute{Description: "Vision (`rnd` / `lite`).", Computed: true},
			"attribute":        schema.StringAttribute{Description: "Attribute.", Computed: true},
			"model":            schema.StringAttribute{Description: "Methodology marker.", Computed: true},
			"program_path":     schema.StringAttribute{Description: "Hierarchy path.", Computed: true},
			"grade":            schema.Int64Attribute{Description: "Depth in program hierarchy.", Computed: true},
			"multiple":         schema.BoolAttribute{Description: "Whether iterations / sub-projects are enabled.", Computed: true},
			"parallel":         schema.BoolAttribute{Description: "Parallel execution flag.", Computed: true},
			"has_product":      schema.BoolAttribute{Description: "Whether the program has any associated products.", Computed: true},
			"workflow_group":   schema.Int64Attribute{Description: "Workflow scheme id.", Computed: true},
			"story_type":       schema.StringAttribute{Description: "Story type marker.", Computed: true},
			"days":             schema.Int64Attribute{Description: "Working-day count.", Computed: true},
			"first_end":        schema.StringAttribute{Description: "First planned end date.", Computed: true},
			"opened_by":        schema.StringAttribute{Description: "Creator username.", Computed: true},
			"opened_date":      schema.StringAttribute{Description: "Creation timestamp.", Computed: true},
			"last_edited_by":   schema.StringAttribute{Description: "Last-editor username.", Computed: true},
			"last_edited_date": schema.StringAttribute{Description: "Timestamp of the last edit.", Computed: true},
			"real_began":       schema.StringAttribute{Description: "Actual start date.", Computed: true},
			"real_end":         schema.StringAttribute{Description: "Actual end date.", Computed: true},
			"closed_by":        schema.StringAttribute{Description: "User who closed the program.", Computed: true},
			"closed_date":      schema.StringAttribute{Description: "When the program was closed.", Computed: true},
			"closed_reason":    schema.StringAttribute{Description: "Reason recorded at close time.", Computed: true},
			"canceled_by":      schema.StringAttribute{Description: "User who canceled the program.", Computed: true},
			"canceled_date":    schema.StringAttribute{Description: "When the program was canceled.", Computed: true},
			"suspended_date":   schema.StringAttribute{Description: "When the program was suspended.", Computed: true},
			"po":               schema.StringAttribute{Description: "Product Owner username.", Computed: true},
			"qd":               schema.StringAttribute{Description: "QA Lead username.", Computed: true},
			"rd":               schema.StringAttribute{Description: "Release Lead username.", Computed: true},
			"team":             schema.StringAttribute{Description: "Team designator.", Computed: true},
			"progress":         schema.StringAttribute{Description: "Completion progress percentage.", Computed: true},
			"percent":          schema.StringAttribute{Description: "Auxiliary completion percent.", Computed: true},
			"estimate":         schema.StringAttribute{Description: "Estimated effort.", Computed: true},
			"consumed":         schema.StringAttribute{Description: "Consumed effort.", Computed: true},
			"left":             schema.StringAttribute{Description: "Remaining effort.", Computed: true},
			"team_count":       schema.Int64Attribute{Description: "Total team members.", Computed: true},
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
		ID:             types.StringValue(strconv.Itoa(fetched.ID)),
		Name:           types.StringValue(fetched.Name),
		Begin:          types.StringValue(fetched.Begin),
		End:            types.StringValue(fetched.End),
		Parent:         types.Int64Value(int64(fetched.Parent)),
		PM:             types.StringValue(fetched.PM),
		Desc:           types.StringValue(fetched.Desc),
		ACL:            types.StringValue(fetched.ACL),
		Budget:         types.StringValue(fetched.Budget),
		BudgetUnit:     types.StringValue(fetched.BudgetUnit),
		Whitelist:      types.StringValue(fetched.Whitelist),
		Code:           types.StringValue(fetched.Code),
		Status:         types.StringValue(fetched.Status),
		Type:           types.StringValue(fetched.Type),
		Category:       types.StringValue(fetched.Category),
		Lifetime:       types.StringValue(fetched.Lifetime),
		Vision:         types.StringValue(fetched.Vision),
		Attribute:      types.StringValue(fetched.Attribute),
		Model:          types.StringValue(fetched.Model),
		ProgramPath:    types.StringValue(fetched.Path),
		Grade:          types.Int64Value(int64(fetched.Grade)),
		Multiple:       programWireBoolToTF(fetched.Multiple),
		Parallel:       programWireBoolToTF(fetched.Parallel),
		HasProduct:     types.BoolValue(fetched.HasProduct == 1),
		WorkflowGroup:  types.Int64Value(int64(fetched.WorkflowGroup)),
		StoryType:      types.StringValue(fetched.StoryType),
		Days:           types.Int64Value(int64(fetched.Days)),
		FirstEnd:       types.StringValue(fetched.FirstEnd),
		OpenedBy:       types.StringValue(fetched.OpenedBy),
		OpenedDate:     types.StringValue(fetched.OpenedDate),
		LastEditedBy:   types.StringValue(fetched.LastEditedBy),
		LastEditedDate: types.StringValue(fetched.LastEditedDate),
		RealBegan:      types.StringValue(fetched.RealBegan),
		RealEnd:        types.StringValue(fetched.RealEnd),
		ClosedBy:       types.StringValue(fetched.ClosedBy),
		ClosedDate:     types.StringValue(fetched.ClosedDate),
		ClosedReason:   types.StringValue(fetched.ClosedReason),
		CanceledBy:     types.StringValue(fetched.CanceledBy),
		CanceledDate:   types.StringValue(fetched.CanceledDate),
		SuspendedDate:  types.StringValue(fetched.SuspendedDate),
		PO:             types.StringValue(fetched.PO),
		QD:             types.StringValue(fetched.QD),
		RD:             types.StringValue(fetched.RD),
		Team:           types.StringValue(fetched.Team),
		Progress:       types.StringValue(fetched.Progress),
		Percent:        types.StringValue(fetched.Percent),
		Estimate:       types.StringValue(fetched.Estimate),
		Consumed:       types.StringValue(fetched.Consumed),
		Left:           types.StringValue(fetched.Left),
		TeamCount:      types.Int64Value(int64(fetched.TeamCount)),
	})...)
}
