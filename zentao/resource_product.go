package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*productResource)(nil)
	_ resource.ResourceWithConfigure   = (*productResource)(nil)
	_ resource.ResourceWithImportState = (*productResource)(nil)
)

// Allowed enums per ZenTao v2 API documentation.
var (
	productTypeEnum = []string{"normal", "branch", "platform"}
	productACLEnum  = []string{"open", "private"}
)

type productResource struct {
	client *zentaoapi.Client
}

type productResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`

	Program   types.Int64  `tfsdk:"program"`
	Line      types.Int64  `tfsdk:"line"`
	Code      types.String `tfsdk:"code"`
	PO        types.String `tfsdk:"po"`
	QD        types.String `tfsdk:"qd"`
	RD        types.String `tfsdk:"rd"`
	Reviewer  types.List   `tfsdk:"reviewer"`
	Type      types.String `tfsdk:"type"`
	Status    types.String `tfsdk:"status"`
	Desc      types.String `tfsdk:"desc"`
	ACL       types.String `tfsdk:"acl"`
	Groups    types.List   `tfsdk:"groups"`
	Whitelist types.List   `tfsdk:"whitelist"`

	CreatedBy   types.String `tfsdk:"created_by"`
	CreatedDate types.String `tfsdk:"created_date"`
}

func NewProductResource() resource.Resource { return &productResource{} }

func (r *productResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *productResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	useStateForList := []planmodifier.List{listplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao product.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao product ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"name": schema.StringAttribute{
				Description: "Product display name.",
				Required:    true,
			},
			"program": schema.Int64Attribute{
				Description:   "Associated program id.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"line": schema.Int64Attribute{
				Description:   "Associated product line id.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"code": schema.StringAttribute{
				Description:   "Product short code.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"po": schema.StringAttribute{
				Description:   "Product Owner username. Defaults to the requesting account if omitted.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"qd": schema.StringAttribute{
				Description:   "QA Lead username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"rd": schema.StringAttribute{
				Description:   "Release Lead username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"reviewer": schema.ListAttribute{
				Description:   "Reviewer usernames.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: useStateForList,
			},
			"groups": schema.ListAttribute{
				Description:   "Permission group ids granted access to this product.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: useStateForList,
			},
			"whitelist": schema.ListAttribute{
				Description:   "Whitelisted usernames granted access to this product.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: useStateForList,
			},
			"type": schema.StringAttribute{
				Description:   "Product type. One of: " + commaJoin(productTypeEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(productTypeEnum...)},
			},
			"status": schema.StringAttribute{
				Description: "Product status.",
				Computed:    true,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: useStateForString,
			},
			"acl": schema.StringAttribute{
				Description:   "Access control. One of: " + commaJoin(productACLEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(productACLEnum...)},
			},
			"created_by":   schema.StringAttribute{Description: "Creator username.", Computed: true, PlanModifiers: useStateForString},
			"created_date": schema.StringAttribute{Description: "Creation timestamp.", Computed: true, PlanModifiers: useStateForString},
		},
	}
}

func (r *productResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *productResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan productResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateProduct(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Create product failed", err.Error())
		return
	}
	fetched, err := r.client.GetProduct(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Re-fetch after create failed", err.Error())
		return
	}
	state, diags := fromAPI(ctx, fetched)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior productResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetProduct(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read product failed", err.Error())
		return
	}
	state, diags := fromAPI(ctx, fetched)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior productResourceModel
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
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput.ID = id
	updated, err := r.client.UpdateProduct(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update product failed", err.Error())
		return
	}
	state, diags := fromAPI(ctx, updated)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior productResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteProduct(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete product failed", err.Error())
		return
	}
}

func (r *productResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *productResourceModel) toAPI(ctx context.Context) (*zentaoapi.Product, diag.Diagnostics) {
	var reviewers, groups, whitelist []string
	diags := m.Reviewer.ElementsAs(ctx, &reviewers, true)
	diags.Append(m.Groups.ElementsAs(ctx, &groups, true)...)
	diags.Append(m.Whitelist.ElementsAs(ctx, &whitelist, true)...)
	return &zentaoapi.Product{
		Name:      m.Name.ValueString(),
		Program:   m.Program.ValueInt64(),
		Line:      m.Line.ValueInt64(),
		Type:      m.Type.ValueString(),
		Desc:      m.Desc.ValueString(),
		ACL:       m.ACL.ValueString(),
		PO:        m.PO.ValueString(),
		QD:        m.QD.ValueString(),
		RD:        m.RD.ValueString(),
		Reviewer:  reviewers,
		Groups:    groups,
		Whitelist: whitelist,
	}, diags
}

func fromAPI(ctx context.Context, p *zentaoapi.Product) (productResourceModel, diag.Diagnostics) {
	reviewers, diags := stringListFromSlice(ctx, p.Reviewer)
	groups, d := stringListFromSlice(ctx, p.Groups)
	diags.Append(d...)
	whitelist, d := stringListFromSlice(ctx, p.Whitelist)
	diags.Append(d...)
	return productResourceModel{
		ID:          types.StringValue(strconv.FormatInt(p.ID, 10)),
		Code:        types.StringValue(p.Code),
		Name:        types.StringValue(p.Name),
		Program:     types.Int64Value(int64(p.Program)),
		Line:        types.Int64Value(int64(p.Line)),
		Type:        types.StringValue(p.Type),
		Desc:        types.StringValue(p.Desc),
		ACL:         types.StringValue(p.ACL),
		PO:          types.StringValue(p.PO),
		QD:          types.StringValue(p.QD),
		RD:          types.StringValue(p.RD),
		Reviewer:    reviewers,
		Groups:      groups,
		Whitelist:   whitelist,
		Status:      types.StringValue(p.Status),
		CreatedBy:   types.StringValue(p.CreatedBy),
		CreatedDate: types.StringValue(p.CreatedDate),
	}, diags
}

// stringListFromSlice normalises a nil slice into an empty list value
// (Terraform distinguishes null-list from empty-list and the latter is
// what `flexibleStringList` decodes to when the wire returns an empty
// string).
func stringListFromSlice(ctx context.Context, src []string) (types.List, diag.Diagnostics) {
	if src == nil {
		src = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, src)
}
