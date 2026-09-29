package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

func kvKeyValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 255),
		stringvalidator.RegexMatches(client.KVKeyPattern, "may only contain letters, digits, spaces and . : _ - $"),
	}
}

func kvKeyAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description + " Up to 255 letters, digits, spaces and `.` `:` `_` `-` `$`. Changing it replaces the resource.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		Validators:          kvKeyValidators(),
	}
}

func kvIDAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func stringSet(items []string) (types.Set, diag.Diagnostics) {
	values := make([]attr.Value, 0, len(items))
	for _, item := range items {
		values = append(values, types.StringValue(item))
	}
	return types.SetValue(types.StringType, values)
}

func setStrings(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	var items []string
	diags := set.ElementsAs(ctx, &items, false)
	slices.Sort(items)
	return items, diags
}

// setDifference returns the items in from that are not in without.
func setDifference(from, without []string) []string {
	excluded := make(map[string]bool, len(without))
	for _, item := range without {
		excluded[item] = true
	}
	var difference []string
	for _, item := range from {
		if !excluded[item] {
			difference = append(difference, item)
		}
	}
	return difference
}

func kvDataSourceKey(description string) datasourceschema.StringAttribute {
	return datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          kvKeyValidators(),
	}
}

func kvDataSourceClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse, name string) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The %s data source expected providerData, got %T.", name, req.ProviderData))
		return nil
	}
	return data.Client
}

// setIntersection returns the items in from that are also in within.
func setIntersection(from, within []string) []string {
	included := make(map[string]bool, len(within))
	for _, item := range within {
		included[item] = true
	}
	var intersection []string
	for _, item := range from {
		if included[item] {
			intersection = append(intersection, item)
		}
	}
	return intersection
}
