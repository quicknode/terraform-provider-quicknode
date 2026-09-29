package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

const (
	resumeFromLast   = "last"
	resumeFromLatest = "latest"
)

var _ resource.Resource = (*streamResource)(nil)
var _ resource.ResourceWithConfigure = (*streamResource)(nil)
var _ resource.ResourceWithImportState = (*streamResource)(nil)
var _ resource.ResourceWithModifyPlan = (*streamResource)(nil)
var _ resource.ResourceWithValidateConfig = (*streamResource)(nil)

type streamResource struct {
	client *client.Client
}

type streamResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Network              types.String `tfsdk:"network"`
	Dataset              types.String `tfsdk:"dataset"`
	Region               types.String `tfsdk:"region"`
	Status               types.String `tfsdk:"status"`
	ResumeFrom           types.String `tfsdk:"resume_from"`
	State                types.String `tfsdk:"state"`
	FilterFunction       types.String `tfsdk:"filter_function"`
	FilterLanguage       types.String `tfsdk:"filter_language"`
	StartRange           types.Int64  `tfsdk:"start_range"`
	EndRange             types.Int64  `tfsdk:"end_range"`
	DatasetBatchSize     types.Int64  `tfsdk:"dataset_batch_size"`
	ElasticBatchEnabled  types.Bool   `tfsdk:"elastic_batch_enabled"`
	FixBlockReorgs       types.Bool   `tfsdk:"fix_block_reorgs"`
	KeepDistanceFromTip  types.Int64  `tfsdk:"keep_distance_from_tip"`
	RestreamBatchOnReorg types.Bool   `tfsdk:"restream_batch_on_reorg"`
	NotificationEmail    types.String `tfsdk:"notification_email"`
	Destination          types.Object `tfsdk:"destination"`
	ExtraDestinations    types.List   `tfsdk:"extra_destinations"`
}

func NewStreamResource() resource.Resource {
	return &streamResource{}
}

func (r *streamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stream"
}

func (r *streamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Quicknode Stream. It delivers blockchain data from a network to one or more destinations.\n\n" +
			"Quicknode can stop a stream by itself: `terminated` when a destination keeps failing or the account's plan no longer allows it, " +
			"and `completed` after it delivers `end_range`. `state` reports which. For a terminated stream the plan changes `status` back to the configured value, " +
			"and applying it resumes or pauses the stream. A completed stream can't be changed or resumed, so any change replaces it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stream id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": requiredString("Name shown in the dashboard. Names are not unique."),
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network slug, for example `ethereum-mainnet` or `solana-devnet`. Changing it replaces the stream.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"dataset": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What each batch carries, for example `block`, `block_with_receipts`, `receipts`, `logs`, `transactions` or `trace_blocks`. The datasets available depend on the network. Changing it replaces the stream.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region the stream runs in: `usa_east`, `europe_central` or `asia_east`. Changing it replaces the stream.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.OneOf("usa_east", "europe_central", "asia_east")},
			},
			"status": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`active` or `paused`. Required, so importing a stream never starts or stops it.",
				Validators:          []validator.String{stringvalidator.OneOf(client.StreamStatusActive, client.StreamStatusPaused)},
			},
			"resume_from": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(resumeFromLast),
				MarkdownDescription: "Where a paused or terminated stream continues when `status` changes to `active`: `last` continues after the last block delivered, " +
					"`latest` skips ahead to the newest block. `latest` moves `start_range`, so it cannot be combined with a configured `start_range`. Defaults to `last`.",
				Validators: []validator.String{stringvalidator.OneOf(resumeFromLast, resumeFromLatest)},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The stream's actual status: `active`, `paused`, `terminated` or `completed`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"filter_function": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Source code of the filter that shapes each batch, as plain text. Read it from a file with `file(\"filter.js\")`. " +
					"Quicknode runs the filter against `start_range` when the stream is created and rejects the stream if it fails.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"filter_language": defaultedString("Language of `filter_function`: `javascript` or `go`.", "javascript", "javascript", "go"),
			"start_range": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "First block to deliver. When omitted, the stream starts at the newest block and this attribute reports which one. " +
					"Changing it moves the stream to that block, which can deliver blocks again or skip them.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:    []validator.Int64{int64validator.AtLeast(0)},
			},
			"end_range": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Last block to deliver. When set, the stream stops as `completed` after delivering it. When omitted, the stream runs until it is paused or deleted.",
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"dataset_batch_size": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: "Blocks per batch. Some fast networks require a larger minimum. Defaults to `1`.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"elastic_batch_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Grow batches while the stream catches up to the newest block. Only allowed without `end_range`. Defaults to `false`.",
			},
			"fix_block_reorgs": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Deliver corrected blocks when the network reorganizes. Depends on the account's plan. Defaults to `false`.",
			},
			"keep_distance_from_tip": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Stay this many blocks behind the newest block, so reorganized blocks are never delivered. Depends on the account's plan.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"restream_batch_on_reorg": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Deliver the whole batch again when a block in it is reorganized.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"notification_email": optionalString("Address notified when the stream is terminated."),
			"destination": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Where the stream delivers. Set exactly one type. Changing the type replaces the stream.",
				Attributes:          streamDestinationAttributes(),
			},
			"extra_destinations": schema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "More destinations that receive every batch, each set like `destination`. Types can repeat. " +
					"The stream moves on only after every destination accepts a batch, and stops if one keeps failing. " +
					"The limit depends on the account's plan.",
				NestedObject: schema.NestedAttributeObject{Attributes: streamDestinationAttributes()},
			},
		},
	}
}

func (r *streamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The stream resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *streamResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config streamResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ElasticBatchEnabled.ValueBool() && !config.EndRange.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("elastic_batch_enabled"),
			"Elastic batching needs an open-ended stream",
			"Quicknode turns elastic batching off for a stream with an end_range. Remove end_range or set elastic_batch_enabled to false.")
	}

	checkOneType := func(object types.Object, at path.Path) {
		if object.IsNull() || object.IsUnknown() {
			return
		}
		count, known, diags := destinationTypeCount(ctx, object)
		resp.Diagnostics.Append(diags...)
		if known && count != 1 {
			resp.Diagnostics.AddAttributeError(at, "Set exactly one destination type",
				fmt.Sprintf("A destination takes one of webhook, s3, azure, postgres or kafka; %d are set.", count))
		}
	}
	checkOneType(config.Destination, path.Root("destination"))

	if config.ExtraDestinations.IsNull() || config.ExtraDestinations.IsUnknown() {
		return
	}
	var extras []types.Object
	resp.Diagnostics.Append(config.ExtraDestinations.ElementsAs(ctx, &extras, false)...)
	for index, extra := range extras {
		checkOneType(extra, path.Root("extra_destinations").AtListIndex(index))
	}
}

// ModifyPlan covers what the API decides at run time: resuming from the newest
// block moves start_range, a status change can land on a different state, and
// a completed stream accepts no changes at all.
func (r *streamResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan, state, config streamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.State.ValueString() == client.StreamStatusCompleted {
		resp.RequiresReplace = append(resp.RequiresReplace, changedStreamAttributes(plan, state)...)
		return
	}

	if destinationTypeName(ctx, plan.Destination) != destinationTypeName(ctx, state.Destination) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("destination"))
	}

	if !plan.Status.Equal(state.Status) {
		plan.State = types.StringUnknown()
	}

	if resuming(plan, state) && plan.ResumeFrom.ValueString() == resumeFromLatest {
		if !config.StartRange.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("resume_from"),
				"resume_from = \"latest\" conflicts with start_range",
				"Resuming from the newest block moves start_range, so the configured value would be applied again on the next plan. Remove start_range or use resume_from = \"last\".")
			return
		}
		plan.StartRange = types.Int64Unknown()
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *streamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan streamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	destination, diags := destinationFromObject(ctx, plan.Destination)
	resp.Diagnostics.Append(diags...)
	extras, diags := destinationsFromList(ctx, plan.ExtraDestinations)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.StreamInput{
		Name:                 plan.Name.ValueString(),
		Network:              plan.Network.ValueString(),
		Dataset:              plan.Dataset.ValueString(),
		Region:               plan.Region.ValueString(),
		Status:               plan.Status.ValueString(),
		FilterFunction:       plan.FilterFunction.ValueString(),
		FilterLanguage:       plan.FilterLanguage.ValueString(),
		StartRange:           knownInt64(plan.StartRange),
		EndRange:             knownInt64(plan.EndRange),
		DatasetBatchSize:     plan.DatasetBatchSize.ValueInt64(),
		ElasticBatchEnabled:  plan.ElasticBatchEnabled.ValueBool(),
		RestreamBatchOnReorg: knownBool(plan.RestreamBatchOnReorg),
		FixBlockReorgs:       reorgFlag(plan.FixBlockReorgs),
		KeepDistanceFromTip:  knownInt64(plan.KeepDistanceFromTip),
		NotificationEmail:    knownStringPointer(plan.NotificationEmail),
		Destination:          destination,
		ExtraDestinations:    extras,
	}

	created, err := r.client.CreateStream(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Could not create the Quicknode stream", err.Error())
		return
	}

	planned := plan.Status
	resp.Diagnostics.Append(applyStream(created, &plan)...)
	plan.Status = planned
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *streamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state streamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stream, err := r.client.GetStream(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Quicknode stream", err.Error())
		return
	}

	previous := state.Status
	resp.Diagnostics.Append(applyStream(stream, &state)...)
	if stream.Status == client.StreamStatusCompleted {
		state.Status = previous
		if state.Status.IsNull() {
			state.Status = types.StringValue(client.StreamStatusActive)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *streamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state streamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	update, changed, diags := streamUpdate(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if changed {
		if _, err := r.client.UpdateStream(ctx, id, update); err != nil {
			resp.Diagnostics.AddError("Could not update the Quicknode stream", err.Error())
			return
		}
	}

	switch {
	case resuming(plan, state):
		if err := r.client.ActivateStream(ctx, id); err != nil {
			resp.Diagnostics.AddError("Could not activate the Quicknode stream", err.Error())
			return
		}
	case plan.Status.ValueString() == client.StreamStatusPaused && state.Status.ValueString() != client.StreamStatusPaused:
		if err := r.client.PauseStream(ctx, id); err != nil {
			resp.Diagnostics.AddError("Could not pause the Quicknode stream", err.Error())
			return
		}
	}

	stream, err := r.client.GetStream(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Quicknode stream back", err.Error())
		return
	}
	planned := plan.Status
	resp.Diagnostics.Append(applyStream(stream, &plan)...)
	plan.Status = planned
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *streamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state streamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteStream(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete the Quicknode stream", err.Error())
	}
}

func (r *streamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resume_from"), resumeFromLast)...)
}

func resuming(plan, state streamResourceModel) bool {
	return plan.Status.ValueString() == client.StreamStatusActive && state.Status.ValueString() != client.StreamStatusActive
}

// streamUpdate builds a PATCH carrying only what the plan changes. Status is
// left out: Update moves it through the pause and activate calls instead.
func streamUpdate(ctx context.Context, plan, state streamResourceModel) (client.StreamUpdate, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	var update client.StreamUpdate
	changed := false

	if !plan.Name.Equal(state.Name) {
		update.Name = plan.Name.ValueStringPointer()
		changed = true
	}
	if !plan.FilterFunction.Equal(state.FilterFunction) {
		source := plan.FilterFunction.ValueString()
		update.FilterFunction = &source
		changed = true
	}
	if !plan.FilterLanguage.Equal(state.FilterLanguage) {
		update.FilterLanguage = plan.FilterLanguage.ValueStringPointer()
		changed = true
	}

	switch {
	case plan.StartRange.IsUnknown() && resuming(plan, state):
		latest := client.StreamRangeUnset
		update.StartRange = &latest
		changed = true
	case !plan.StartRange.IsUnknown() && !plan.StartRange.Equal(state.StartRange):
		update.StartRange = plan.StartRange.ValueInt64Pointer()
		changed = true
	}

	if !plan.EndRange.Equal(state.EndRange) {
		end := client.StreamRangeUnset
		if !plan.EndRange.IsNull() {
			end = plan.EndRange.ValueInt64()
		}
		update.EndRange = &end
		changed = true
	}
	if !plan.DatasetBatchSize.Equal(state.DatasetBatchSize) {
		update.DatasetBatchSize = plan.DatasetBatchSize.ValueInt64Pointer()
		changed = true
	}
	if !plan.ElasticBatchEnabled.Equal(state.ElasticBatchEnabled) {
		update.ElasticBatchEnabled = plan.ElasticBatchEnabled.ValueBoolPointer()
		changed = true
	}
	if !plan.FixBlockReorgs.Equal(state.FixBlockReorgs) {
		update.FixBlockReorgs = reorgFlag(plan.FixBlockReorgs)
		changed = true
	}
	if !plan.KeepDistanceFromTip.IsUnknown() && !plan.KeepDistanceFromTip.Equal(state.KeepDistanceFromTip) {
		update.KeepDistanceFromTip = plan.KeepDistanceFromTip.ValueInt64Pointer()
		changed = true
	}
	if !plan.RestreamBatchOnReorg.IsUnknown() && !plan.RestreamBatchOnReorg.Equal(state.RestreamBatchOnReorg) {
		update.RestreamBatchOnReorg = plan.RestreamBatchOnReorg.ValueBoolPointer()
		changed = true
	}
	if !plan.NotificationEmail.Equal(state.NotificationEmail) {
		email := plan.NotificationEmail.ValueString()
		update.NotificationEmail = &email
		changed = true
	}

	if !plan.Destination.Equal(state.Destination) {
		destination, destinationDiags := destinationFromObject(ctx, plan.Destination)
		diags.Append(destinationDiags...)
		update.Destination = &destination
		changed = true
	}
	if extraDestinationsChanged(plan, state) {
		extras, extraDiags := destinationsFromList(ctx, plan.ExtraDestinations)
		diags.Append(extraDiags...)
		update.ExtraDestinations = &extras
		changed = true
	}

	return update, changed, diags
}

// changedStreamAttributes lists what a plan changes on a completed stream, all
// of which have to be applied by replacing it.
func changedStreamAttributes(plan, state streamResourceModel) []path.Path {
	candidates := []struct {
		name    string
		changed bool
	}{
		{"name", !plan.Name.Equal(state.Name)},
		{"status", !plan.Status.Equal(state.Status)},
		{"filter_function", !plan.FilterFunction.Equal(state.FilterFunction)},
		{"filter_language", !plan.FilterLanguage.Equal(state.FilterLanguage)},
		{"start_range", !plan.StartRange.Equal(state.StartRange)},
		{"end_range", !plan.EndRange.Equal(state.EndRange)},
		{"dataset_batch_size", !plan.DatasetBatchSize.Equal(state.DatasetBatchSize)},
		{"elastic_batch_enabled", !plan.ElasticBatchEnabled.Equal(state.ElasticBatchEnabled)},
		{"fix_block_reorgs", !plan.FixBlockReorgs.Equal(state.FixBlockReorgs)},
		{"keep_distance_from_tip", !plan.KeepDistanceFromTip.Equal(state.KeepDistanceFromTip)},
		{"restream_batch_on_reorg", !plan.RestreamBatchOnReorg.Equal(state.RestreamBatchOnReorg)},
		{"notification_email", !plan.NotificationEmail.Equal(state.NotificationEmail)},
		{"destination", !plan.Destination.Equal(state.Destination)},
		{"extra_destinations", extraDestinationsChanged(plan, state)},
	}
	var paths []path.Path
	for _, candidate := range candidates {
		if candidate.changed {
			paths = append(paths, path.Root(candidate.name))
		}
	}
	return paths
}

func destinationTypeName(ctx context.Context, object types.Object) string {
	if object.IsNull() || object.IsUnknown() {
		return ""
	}
	var model streamDestinationModel
	if diags := object.As(ctx, &model, objectAsOptions); diags.HasError() {
		return ""
	}
	names := map[string]types.Object{
		client.DestinationWebhook:  model.Webhook,
		client.DestinationS3:       model.S3,
		client.DestinationAzure:    model.Azure,
		client.DestinationPostgres: model.Postgres,
		client.DestinationKafka:    model.Kafka,
	}
	for name, child := range names {
		if !child.IsNull() {
			return name
		}
	}
	return ""
}

// applyStream copies what the API reports into the model. The caller decides
// what status means: Create and Update keep the planned status, Read keeps
// the previous one for a completed stream.
func applyStream(stream *client.Stream, model *streamResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.ID = types.StringValue(stream.ID)
	model.Name = types.StringValue(stream.Name)
	model.Network = types.StringValue(stream.Network)
	model.Dataset = types.StringValue(stream.Dataset)
	model.Region = types.StringValue(stream.Region)
	model.Status = types.StringValue(stream.Status)
	model.State = types.StringValue(stream.Status)
	model.FilterFunction = stringOrNull(stream.FilterFunction)
	model.FilterLanguage = types.StringValue(stream.FilterLanguage)
	model.StartRange = types.Int64Value(stream.StartRange)
	model.EndRange = rangeOrNull(stream.EndRange)
	model.DatasetBatchSize = types.Int64Value(stream.DatasetBatchSize)
	model.ElasticBatchEnabled = types.BoolValue(stream.ElasticBatchEnabled)
	model.FixBlockReorgs = types.BoolValue(stream.FixBlockReorgs != 0)
	model.KeepDistanceFromTip = types.Int64Value(stream.KeepDistanceFromTip)
	model.RestreamBatchOnReorg = types.BoolValue(stream.RestreamBatchOnReorg)
	model.NotificationEmail = stringOrNull(stream.NotificationEmail)

	destination, destinationDiags := destinationObject(stream.Destination)
	diags.Append(destinationDiags...)
	model.Destination = destination

	extras, extraDiags := destinationsList(stream.ExtraDestinations)
	diags.Append(extraDiags...)
	if len(stream.ExtraDestinations) > 0 || model.ExtraDestinations.IsNull() || model.ExtraDestinations.IsUnknown() {
		model.ExtraDestinations = extras
	} else {
		model.ExtraDestinations = types.ListValueMust(streamDestinationObjectType, []attr.Value{})
	}

	if model.ResumeFrom.IsNull() || model.ResumeFrom.IsUnknown() {
		model.ResumeFrom = types.StringValue(resumeFromLast)
	}
	return diags
}

// extraDestinationsChanged treats an empty list and a missing one as the same,
// so switching between them never updates or replaces the stream.
func extraDestinationsChanged(plan, state streamResourceModel) bool {
	if len(plan.ExtraDestinations.Elements()) == 0 && len(state.ExtraDestinations.Elements()) == 0 &&
		!plan.ExtraDestinations.IsUnknown() && !state.ExtraDestinations.IsUnknown() {
		return false
	}
	return !plan.ExtraDestinations.Equal(state.ExtraDestinations)
}

func rangeOrNull(value int64) types.Int64 {
	if value == client.StreamRangeUnset {
		return types.Int64Null()
	}
	return types.Int64Value(value)
}

func knownInt64(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueInt64Pointer()
}

func knownBool(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueBoolPointer()
}

func knownStringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueStringPointer()
}

// reorgFlag converts fix_block_reorgs to the 0 or 1 the API takes.
func reorgFlag(value types.Bool) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	flag := int64(0)
	if value.ValueBool() {
		flag = 1
	}
	return &flag
}
