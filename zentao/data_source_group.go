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

// NewGroupDataSource registers the st-zentao_group data source. It
// looks up a ZenTao permission group by its numeric id and exposes
// the same five attributes the resource manages (id/project/name/
// role/desc); other zt_group columns are intentionally not surfaced.
func NewGroupDataSource() datasource.DataSource { return &groupDataSource{} }

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao permission group by its numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Description: "Numeric ZenTao group ID.", Required: true},
			"project": schema.Int64Attribute{Description: "Parent project id (`0` for system groups).", Computed: true},
			"name":    schema.StringAttribute{Description: "Group display name.", Computed: true},
			"role":    schema.StringAttribute{Description: "Free-text role label.", Computed: true},
			"desc":    schema.StringAttribute{Description: "Description.", Computed: true},
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
	id, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
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
			"Group not found",
			fmt.Sprintf("ZenTao has no group with id=%d", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupDataSourceModel{
		ID:      types.StringValue(strconv.FormatInt(deref(fetched.ID), 10)),
		Project: types.Int64Value(deref(fetched.Project)),
		Name:    types.StringValue(deref(fetched.Name)),
		Role:    types.StringValue(deref(fetched.Role)),
		Desc:    types.StringValue(deref(fetched.Desc)),
	})...)
}
