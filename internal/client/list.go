package client

import (
	"context"
	"net/http"

	"github.com/quicknode/terraform-provider-quicknode/api/admin"
)

// listPageSize is the largest page the list route accepts. Paging at the
// maximum keeps a large account down to few round trips.
const listPageSize = 250

// EndpointFilter narrows a listing. Empty fields are left off the query, and
// the API treats several values in one field as "any of".
type EndpointFilter struct {
	Search    string
	Networks  []string
	Statuses  []string
	Labels    []string
	TagLabels []string
}

// EndpointSummary is one row of the list route. It carries less than a full
// endpoint read: no tokens, no security and no rate limits. The list route
// returns credentialed URLs, so the URLs here are redacted on the way in and the
// working form is only available from a full endpoint read.
type EndpointSummary struct {
	ID          string
	Name        string
	Label       string
	Chain       string
	Network     string
	Status      string
	SafeHTTPURL string
	SafeWSSURL  string
	Dedicated   bool
	FlatRate    bool
	Multichain  bool
	Tags        []Tag
}

// ListEndpoints walks every page, so a caller gets the whole account rather
// than the first twenty rows.
func (c *Client) ListEndpoints(ctx context.Context, filter EndpointFilter) ([]EndpointSummary, error) {
	const operation = "list endpoints"

	limit := listPageSize
	offset := 0
	var endpoints []EndpointSummary

	for {
		params := &admin.GetV0EndpointsParams{Limit: &limit, Offset: &offset}
		if filter.Search != "" {
			params.Search = &filter.Search
		}
		if len(filter.Networks) > 0 {
			params.Networks = &filter.Networks
		}
		if len(filter.Statuses) > 0 {
			params.Statuses = &filter.Statuses
		}
		if len(filter.Labels) > 0 {
			params.Labels = &filter.Labels
		}
		if len(filter.TagLabels) > 0 {
			params.TagLabels = &filter.TagLabels
		}

		resp, err := c.api.GetV0EndpointsWithResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, statusError(operation, resp.StatusCode(), resp.Body)
		}
		if err := envelopeError(operation, resp.StatusCode(), resp.JSON200.Error); err != nil {
			return nil, err
		}
		if resp.JSON200.Data == nil {
			break
		}

		page := *resp.JSON200.Data
		for _, raw := range page {
			endpoint := EndpointSummary{
				ID:          deref(raw.Id),
				Name:        deref(raw.Name),
				Label:       deref(raw.Label),
				Chain:       deref(raw.Chain),
				Network:     deref(raw.Network),
				Status:      deref(raw.Status),
				SafeHTTPURL: RedactEndpointURL(deref(raw.HttpUrl)),
				SafeWSSURL:  RedactEndpointURL(deref(raw.WssUrl)),
				Dedicated:   deref(raw.IsDedicated),
				FlatRate:    deref(raw.IsFlatRate),
				Multichain:  deref(raw.IsMultichain),
			}
			if raw.Tags != nil {
				for _, rawTag := range *raw.Tags {
					tag := Tag{Label: deref(rawTag.Label)}
					if rawTag.TagId != nil {
						tag.ID = int64(*rawTag.TagId)
					}
					endpoint.Tags = append(endpoint.Tags, tag)
				}
			}
			endpoints = append(endpoints, endpoint)
		}

		if len(page) < limit {
			break
		}
		offset += len(page)
	}
	return endpoints, nil
}

// FindEndpointByLabel resolves a label to a single endpoint. Labels are not
// unique, so more than one match is an error rather than an arbitrary pick.
func (c *Client) FindEndpointByLabel(ctx context.Context, label string) (*EndpointSummary, error) {
	const operation = "find endpoint by label"

	endpoints, err := c.ListEndpoints(ctx, EndpointFilter{Labels: []string{label}})
	if err != nil {
		return nil, err
	}

	matches := make([]EndpointSummary, 0, 1)
	for _, endpoint := range endpoints {
		if endpoint.Label == label {
			matches = append(matches, endpoint)
		}
	}
	switch len(matches) {
	case 0:
		return nil, &Error{Operation: operation, Status: http.StatusNotFound, Message: "no endpoint carries the label " + label}
	case 1:
		return &matches[0], nil
	}
	return nil, &Error{
		Operation: operation,
		Status:    http.StatusConflict,
		Message:   "more than one endpoint carries the label " + label + "; labels are not unique, so look the endpoint up by id instead",
	}
}
