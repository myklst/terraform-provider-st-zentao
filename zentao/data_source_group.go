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
	_ datasource.DataSource              = (*groupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*groupDataSource)(nil)
)

type groupDataSource struct {
	client *zentaoapi.Client
}

type groupDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Project types.Int64  `tfsdk:"project"`
	Name    types.String `tfsdk:"name"`
	Role    types.String `tfsdk:"role"`
	Desc    types.String `tfsdk:"desc"`
}

// NewGroupDataSource registers the st-zentao_group data
// source. It looks up a project-scoped permission group (a row in
// zt_group with project>0) by its numeric id via the Controller
// `group-edit-<id>.json` GET endpoint — the same read primitive used
// by the resource. Returned attributes mirror the v1 resource schema
// exactly (id/project/name/role/desc); fields the probe found but the
// v1 resource does not surface (vision/developer/acl/users/actions)
// are intentionally NOT exposed here.
func NewGroupDataSource() datasource.DataSource { return &groupDataSource{} }

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao project-scoped permission group by its numeric id. " +
			"Reads the row via the `group-edit-<id>.json` Controller endpoint and surfaces " +
			"the same minimal attribute set the st-zentao_group resource manages.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao project group ID (stringified).",
				Required:    true,
			},
			"project": schema.Int64Attribute{
				Description: "Parent project ID (foreign key into zt_project; >0 for project-scoped groups).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Group display name.",
				Computed:    true,
			},
			"role": schema.StringAttribute{
				Description: "Free-text role label bound to the system role registry; empty string when unset.",
				Computed:    true,
			},
			"desc": schema.StringAttribute{
				Description: "Group description text.",
				Computed:    true,
			},
		},
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg groupDataSourceModel
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
	fetched, err := d.client.GetGroup(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Project group not found",
			fmt.Sprintf("ZenTao has no project group with id=%d", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupDataSourceModel{
		ID:      types.StringValue(strconv.Itoa(fetched.ID)),
		Project: types.Int64Value(int64(fetched.Project)),
		Name:    types.StringValue(fetched.Name),
		Role:    types.StringValue(fetched.Role),
		Desc:    types.StringValue(fetched.Desc),
	})...)
}
