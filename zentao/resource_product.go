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
	Code types.String `tfsdk:"code"`

	Name     types.String `tfsdk:"name"`
	Program  types.Int64  `tfsdk:"program"`
	Line     types.Int64  `tfsdk:"line"`
	Type     types.String `tfsdk:"type"`
	Desc     types.String `tfsdk:"desc"`
	ACL      types.String `tfsdk:"acl"`
	PO       types.String `tfsdk:"po"`
	QD       types.String `tfsdk:"qd"`
	RD       types.String `tfsdk:"rd"`
	Reviewer types.List   `tfsdk:"reviewer"`

	Status      types.String `tfsdk:"status"`
	CreatedBy   types.String `tfsdk:"created_by"`
	CreatedDate types.String `tfsdk:"created_date"`
	ProgramName types.String `tfsdk:"program_name"`
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
			"code": schema.StringAttribute{
				Description:   "Product short code.",
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
			"type": schema.StringAttribute{
				Description:   "Product type. One of: " + commaJoin(productTypeEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(productTypeEnum...)},
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
			// po/qd/rd are role-username Optional+Computed fields. ZenTao
			// backfills them with the requesting account when the request body
			// omits the field, so an empty value from prior state must NOT be
			// pinned into the plan — that would mismatch the server-assigned
			// default and raise "Provider produced inconsistent result after apply".
			"po": schema.StringAttribute{
				Description:   "Product Owner username. Defaults to the requesting account if omitted.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{useStateUnlessEmpty()},
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
			"status":       schema.StringAttribute{Description: "Product status.", Computed: true, PlanModifiers: useStateForString},
			"created_by":   schema.StringAttribute{Description: "Creator username.", Computed: true, PlanModifiers: useStateForString},
			"created_date": schema.StringAttribute{Description: "Creation timestamp.", Computed: true, PlanModifiers: useStateForString},
			// program_name is derived from `program` on the server side — it must NOT
			// use UseStateForUnknown, otherwise changing `program` keeps the stale
			// program_name in the plan, then Update returns the new name and Terraform
			// raises "Provider produced inconsistent result after apply".
			"program_name": schema.StringAttribute{Description: "Associated program name.", Computed: true},
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
	id, err := strconv.Atoi(prior.ID.ValueString())
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
	id, err := strconv.Atoi(prior.ID.ValueString())
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
	id, err := strconv.Atoi(prior.ID.ValueString())
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
	var reviewers []string
	diags := m.Reviewer.ElementsAs(ctx, &reviewers, true)
	return &zentaoapi.Product{
		Name:     m.Name.ValueString(),
		Program:  int(m.Program.ValueInt64()),
		Line:     int(m.Line.ValueInt64()),
		Type:     m.Type.ValueString(),
		Desc:     m.Desc.ValueString(),
		ACL:      m.ACL.ValueString(),
		PO:       m.PO.ValueString(),
		QD:       m.QD.ValueString(),
		RD:       m.RD.ValueString(),
		Reviewer: reviewers,
	}, diags
}

func fromAPI(ctx context.Context, p *zentaoapi.Product) (productResourceModel, diag.Diagnostics) {
	reviewerSrc := p.Reviewer
	if reviewerSrc == nil {
		reviewerSrc = []string{}
	}
	reviewers, diags := types.ListValueFrom(ctx, types.StringType, reviewerSrc)
	return productResourceModel{
		ID:          types.StringValue(strconv.Itoa(p.ID)),
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
		Status:      types.StringValue(p.Status),
		CreatedBy:   types.StringValue(p.CreatedBy),
		CreatedDate: types.StringValue(p.CreatedDate),
		ProgramName: types.StringValue(p.ProgramName),
	}, diags
}

func commaJoin(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += `"` + s + `"`
	}
	return out
}
