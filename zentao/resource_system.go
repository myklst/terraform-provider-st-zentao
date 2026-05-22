package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*systemResource)(nil)
	_ resource.ResourceWithConfigure   = (*systemResource)(nil)
	_ resource.ResourceWithImportState = (*systemResource)(nil)
)

var systemStatusEnum = []string{"active", "inactive"}

type systemResource struct {
	client *zentaoapi.Client
}

type systemResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Product        types.Int64  `tfsdk:"product"`
	Name           types.String `tfsdk:"name"`
	Desc           types.String `tfsdk:"desc"`
	Integrated     types.Int64  `tfsdk:"integrated"`
	Status         types.String `tfsdk:"status"`
	CreatedBy      types.String `tfsdk:"created_by"`
	CreatedDate    types.String `tfsdk:"created_date"`
	LastEditedBy   types.String `tfsdk:"last_edited_by"`
	LastEditedDate types.String `tfsdk:"last_edited_date"`
}

func NewSystemResource() resource.Resource { return &systemResource{} }

func (r *systemResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system"
}

func (r *systemResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao application ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"product": schema.Int64Attribute{
				Description: "Owning product id. Changing it forces resource replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Application display name.",
				Required:    true,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"integrated": schema.Int64Attribute{
				Description:   "Whether the application aggregates child applications (1) or not (0).",
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"status": schema.StringAttribute{
				Description:   "Enabled state. One of: " + commaJoin(systemStatusEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(systemStatusEnum...)},
			},
			"created_by": schema.StringAttribute{
				Description:   "Account that created the application.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"created_date": schema.StringAttribute{
				Description:   "Creation timestamp.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			// No UseStateForUnknown: the server recomputes these on every
			// edit, so pinning prior state trips inconsistent-after-apply.
			"last_edited_by": schema.StringAttribute{
				Description: "Account of the last edit.",
				Computed:    true,
			},
			"last_edited_date": schema.StringAttribute{
				Description: "Last edit timestamp.",
				Computed:    true,
			},
		},
	}
}

func (r *systemResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *systemResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateSystem(ctx, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Create application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, systemFromAPI(created))...)
}

func (r *systemResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior systemResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetSystem(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, systemFromAPI(fetched))...)
}

func (r *systemResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior systemResourceModel
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
	updated, err := r.client.UpdateSystem(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, systemFromAPI(updated))...)
}

func (r *systemResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior systemResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteSystem(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete application failed", err.Error())
		return
	}
}

func (r *systemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI maps plan values onto the API struct. children is intentionally
// omitted — the attachment resource owns it, and UpdateSystem's M-Z merge
// preserves the column when input.Children is nil. integrated is omitted
// too: it is server-owned and not an edit-form key.
func (m *systemResourceModel) toAPI() *zentaoapi.System {
	return &zentaoapi.System{
		Product: optInt64(m.Product),
		Name:    optString(m.Name),
		Desc:    optString(m.Desc),
		Status:  optString(m.Status),
	}
}

func systemFromAPI(s *zentaoapi.System) systemResourceModel {
	return systemResourceModel{
		ID:             types.StringValue(strconv.FormatInt(deref(s.ID), 10)),
		Product:        types.Int64Value(deref(s.Product)),
		Name:           types.StringValue(deref(s.Name)),
		Desc:           types.StringValue(deref(s.Desc)),
		Integrated:     types.Int64Value(deref(s.Integrated)),
		Status:         types.StringValue(deref(s.Status)),
		CreatedBy:      types.StringValue(deref(s.CreatedBy)),
		CreatedDate:    types.StringValue(deref(s.CreatedDate)),
		LastEditedBy:   types.StringValue(deref(s.EditedBy)),
		LastEditedDate: types.StringValue(deref(s.EditedDate)),
	}
}
