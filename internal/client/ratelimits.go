package client

import (
	"context"
	"net/http"

	"github.com/quicknode/terraform-provider-quicknode/api/admin"
)

const (
	// RateLimitUnset is what the Admin API reports for a bucket with no
	// endpoint-level override.
	RateLimitUnset = -1

	SourcePlanDefault  = "plan_default"
	SourceUserOverride = "user_override"

	BucketRPS = "rps"
	BucketRPM = "rpm"
	BucketRPD = "rpd"
)

// RateLimit is one bucket as the API reports it. A plan_default carries no id,
// because only an override can be deleted.
type RateLimit struct {
	Bucket string
	Value  int
	Source string
	ID     string
}

// RateLimitOverrides names the buckets a write can set. A nil field is left
// alone, so a configuration that manages only one bucket does not disturb the
// others.
type RateLimitOverrides struct {
	RPS *int
	RPM *int
	RPD *int
}

// MethodRateLimit throttles a named set of RPC methods independently of the
// endpoint-wide buckets.
type MethodRateLimit struct {
	ID       string
	Methods  []string
	Rate     int
	Interval string
	Status   string
	Created  string
}

func (c *Client) GetRateLimits(ctx context.Context, endpointID string) ([]RateLimit, error) {
	const operation = "read endpoint rate limits"

	resp, err := c.api.GetV0EndpointsByIdRateLimitsWithResponse(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, &Error{Operation: operation, Status: http.StatusNotFound, Message: "endpoint not found"}
	}
	if resp.JSON200 == nil {
		return nil, statusError(operation, resp.StatusCode(), resp.Body)
	}
	if err := envelopeError(operation, resp.StatusCode(), resp.JSON200.Error); err != nil {
		return nil, err
	}
	if resp.JSON200.Data == nil || resp.JSON200.Data.RateLimits == nil {
		return nil, nil
	}

	limits := make([]RateLimit, 0, len(*resp.JSON200.Data.RateLimits))
	for _, raw := range *resp.JSON200.Data.RateLimits {
		limit := RateLimit{
			Bucket: deref(raw.Bucket),
			Source: deref(raw.Source),
			ID:     deref(raw.Id),
			Value:  RateLimitUnset,
		}
		if raw.RateLimit != nil {
			limit.Value = *raw.RateLimit
		}
		limits = append(limits, limit)
	}
	return limits, nil
}

// SetRateLimits writes the buckets the caller manages. Managing none sends no
// request at all.
func (c *Client) SetRateLimits(ctx context.Context, endpointID string, overrides RateLimitOverrides) error {
	const operation = "update endpoint rate limits"

	if overrides.RPS == nil && overrides.RPM == nil && overrides.RPD == nil {
		return nil
	}

	body := admin.PatchV0EndpointsByIdRateLimitsJSONRequestBody{}
	body.RateLimits.Rps = overrides.RPS
	body.RateLimits.Rpm = overrides.RPM
	body.RateLimits.Rpd = overrides.RPD

	resp, err := c.api.PatchV0EndpointsByIdRateLimitsWithResponse(ctx, endpointID, body)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

// DeleteRateLimitOverride drops one bucket back to the plan default. A 404 is
// success, since an absent override is the state the caller asked for.
func (c *Client) DeleteRateLimitOverride(ctx context.Context, endpointID, overrideID string) error {
	const operation = "remove endpoint rate limit override"

	resp, err := c.api.DeleteV0EndpointsByIdRateLimitsByOverrideIdWithResponse(ctx, endpointID, overrideID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	if err := envelopeError(operation, resp.StatusCode(), resp.JSON200.Error); err != nil {
		return err
	}
	if resp.JSON200.Data != nil && resp.JSON200.Data.Success != nil && !*resp.JSON200.Data.Success {
		return &Error{Operation: operation, Status: resp.StatusCode(), Message: "the API reported the delete did not succeed"}
	}
	return nil
}

func (c *Client) ListMethodRateLimits(ctx context.Context, endpointID string) ([]MethodRateLimit, error) {
	const operation = "list endpoint method rate limits"

	resp, err := c.api.GetV0EndpointsByIdMethodRateLimitsWithResponse(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, &Error{Operation: operation, Status: http.StatusNotFound, Message: "endpoint not found"}
	}
	if resp.JSON200 == nil {
		return nil, statusError(operation, resp.StatusCode(), resp.Body)
	}
	if err := envelopeError(operation, resp.StatusCode(), resp.JSON200.Error); err != nil {
		return nil, err
	}
	if resp.JSON200.Data == nil || resp.JSON200.Data.RateLimiters == nil {
		return nil, nil
	}

	limiters := make([]MethodRateLimit, 0, len(*resp.JSON200.Data.RateLimiters))
	for _, raw := range *resp.JSON200.Data.RateLimiters {
		limiter := MethodRateLimit{
			ID:       deref(raw.Id),
			Interval: deref(raw.Interval),
			Status:   deref(raw.Status),
			Created:  deref(raw.Created),
		}
		if raw.Rate != nil {
			limiter.Rate = *raw.Rate
		}
		if raw.Methods != nil {
			limiter.Methods = *raw.Methods
		}
		limiters = append(limiters, limiter)
	}
	return limiters, nil
}

func (c *Client) AddMethodRateLimit(ctx context.Context, endpointID string, limiter MethodRateLimit) (*MethodRateLimit, error) {
	const operation = "create endpoint method rate limit"

	resp, err := c.api.PostV0EndpointsByIdMethodRateLimitsWithResponse(ctx, endpointID,
		admin.PostV0EndpointsByIdMethodRateLimitsJSONRequestBody{
			Interval: limiter.Interval,
			Methods:  limiter.Methods,
			Rate:     limiter.Rate,
		})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(operation, resp.StatusCode(), resp.Body)
	}
	if err := envelopeError(operation, resp.StatusCode(), resp.JSON200.Error); err != nil {
		return nil, err
	}
	if resp.JSON200.Data == nil || resp.JSON200.Data.Id == nil {
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no rate limiter id"}
	}

	data := resp.JSON200.Data
	created := &MethodRateLimit{
		ID:       *data.Id,
		Interval: deref(data.Interval),
		Status:   deref(data.Status),
		Created:  deref(data.Created),
	}
	if data.Rate != nil {
		created.Rate = *data.Rate
	}
	if data.Methods != nil {
		created.Methods = *data.Methods
	}
	return created, nil
}

// UpdateMethodRateLimit edits a limiter in place. The route takes the whole
// object, not a delta, and it does not accept interval, so a changed interval
// has to replace the limiter.
func (c *Client) UpdateMethodRateLimit(ctx context.Context, endpointID, limiterID string, limiter MethodRateLimit) error {
	const operation = "update endpoint method rate limit"

	resp, err := c.api.PatchV0EndpointsByIdMethodRateLimitsByMethodRateLimitIdWithResponse(ctx, endpointID, limiterID,
		admin.PatchV0EndpointsByIdMethodRateLimitsByMethodRateLimitIdJSONRequestBody{
			Methods: limiter.Methods,
			Rate:    limiter.Rate,
			Status:  limiter.Status,
		})
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) RemoveMethodRateLimit(ctx context.Context, endpointID, limiterID string) error {
	const operation = "remove endpoint method rate limit"

	resp, err := c.api.DeleteV0EndpointsByIdMethodRateLimitsByMethodRateLimitIdWithResponse(ctx, endpointID, limiterID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}
