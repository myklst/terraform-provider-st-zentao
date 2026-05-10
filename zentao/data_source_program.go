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
	ID         types.String `tfsdk:"id"`
	Parent     types.Int64  `tfsdk:"parent"`
	Name       types.String `tfsdk:"name"`
	PM         types.String `tfsdk:"pm"`
	Budget     types.String `tfsdk:"budget"`
	BudgetUnit types.String `tfsdk:"budget_unit"`
	Begin      types.String `tfsdk:"begin"`
	End        types.String `tfsdk:"end"`
	Desc       types.String `tfsdk:"desc"`
	Status     types.String `tfsdk:"status"`
	ACL        types.String `tfsdk:"acl"`
	Whitelist  types.String `tfsdk:"whitelist"`
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
			"parent":      schema.Int64Attribute{Description: "Parent program id (0 = top-level).", Computed: true},
			"name":        schema.StringAttribute{Description: "Program display name.", Computed: true},
			"pm":          schema.StringAttribute{Description: "Project Manager username.", Computed: true},
			"budget":      schema.StringAttribute{Description: "Program budget amount.", Computed: true},
			"budget_unit": schema.StringAttribute{Description: "Currency unit of the budget.", Computed: true},
			"begin":       schema.StringAttribute{Description: "Planned start date (YYYY-MM-DD).", Computed: true},
			"end":         schema.StringAttribute{Description: "Planned end date (YYYY-MM-DD).", Computed: true},
			"desc":        schema.StringAttribute{Description: "Description.", Computed: true},
			"status":      schema.StringAttribute{Description: "Program status.", Computed: true},
			"acl":         schema.StringAttribute{Description: "Access control (`open` / `private` / `custom`).", Computed: true},
			"whitelist":   schema.StringAttribute{Description: "Comma-joined account list when `acl = \"custom\"`.", Computed: true},
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
	id, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
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
		ID:         types.StringValue(strconv.FormatInt(fetched.ID, 10)),
		Parent:     types.Int64Value(int64(fetched.Parent)),
		Name:       types.StringValue(fetched.Name),
		PM:         types.StringValue(fetched.PM),
		Budget:     types.StringValue(fetched.Budget),
		BudgetUnit: types.StringValue(fetched.BudgetUnit),
		Begin:      types.StringValue(fetched.Begin),
		End:        types.StringValue(fetched.End),
		Desc:       types.StringValue(fetched.Desc),
		Status:     types.StringValue(fetched.Status),
		ACL:        types.StringValue(fetched.ACL),
		Whitelist:  types.StringValue(fetched.Whitelist),
	})...)
}
