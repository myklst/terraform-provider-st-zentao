package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*groupResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupResource)(nil)
	_ resource.ResourceWithImportState = (*groupResource)(nil)
)

type groupResource struct {
	client *zentaoapi.Client
}

type groupResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Project types.Int64  `tfsdk:"project"`
	Name    types.String `tfsdk:"name"`
	Role    types.String `tfsdk:"role"`
	Desc    types.String `tfsdk:"desc"`
}

func NewGroupResource() resource.Resource { return &groupResource{} }

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao permission group. `project = 0` (default) is a system " +
			"(org-wide) group; `project > 0` is a project-scoped group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao group ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"project": schema.Int64Attribute{
				Description: "Parent project id. `0` = system (org-wide) group; positive integer = project-scoped group. Changing this forces resource replacement.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Group display name. Must be unique within its scope.",
				Required:    true,
			},
			"role": schema.StringAttribute{
				Description:   "Free-text role label.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: useStateForString,
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.client = client
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateGroup(ctx, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Create group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(created))...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetGroup(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(fetched))...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	apiInput := plan.toAPI()
	apiInput.ID = &id
	updated, err := r.client.UpdateGroup(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(updated))...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteGroup(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete group failed", err.Error())
		return
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI projects a Terraform plan into the zentaoapi.Group wire shape.
// Optional+Computed fields produce nil pointers when null/unknown so the
// API layer's M-Z merge preserves baseline values.
func (m *groupResourceModel) toAPI() *zentaoapi.Group {
	return &zentaoapi.Group{
		Project: optInt64(m.Project),
		Name:    optString(m.Name),
		Role:    optString(m.Role),
		Desc:    optString(m.Desc),
	}
}

func groupFromAPI(g *zentaoapi.Group) groupResourceModel {
	return groupResourceModel{
		ID:      types.StringValue(strconv.FormatInt(deref(g.ID), 10)),
		Project: types.Int64Value(deref(g.Project)),
		Name:    types.StringValue(deref(g.Name)),
		Role:    types.StringValue(deref(g.Role)),
		Desc:    types.StringValue(deref(g.Desc)),
	}
}
