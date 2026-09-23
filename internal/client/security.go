package client

import (
	"context"
	"net/http"

	"github.com/quicknode/terraform-provider-quicknode/api/admin"
)

const (
	optionEnabled  = "enabled"
	optionDisabled = "disabled"
)

// SecurityOptions carries the toggles the Admin API reports for an endpoint.
// Tokens through Cors are settable. RequestFilters and ResponseLogging are
// reported but cannot be written, so the provider surfaces them read-only.
type SecurityOptions struct {
	Tokens      bool
	Referrers   bool
	JWTs        bool
	IPs         bool
	DomainMasks bool
	HSTS        bool
	Cors        bool

	RequestFilters  bool
	ResponseLogging bool

	IPCustomHeader string
}

// SecurityOptionsPatch names the settable toggles. A nil field is left alone,
// which keeps a write from clobbering a toggle the configuration does not
// manage.
type SecurityOptionsPatch struct {
	Tokens      *bool
	Referrers   *bool
	JWTs        *bool
	IPs         *bool
	DomainMasks *bool
	HSTS        *bool
	Cors        *bool
}

type SecurityEntry struct {
	ID    string
	Value string
}

type JWT struct {
	ID        string
	Name      string
	KID       string
	PublicKey string
}

type RequestFilter struct {
	ID      string
	Methods []string
}

type EndpointSecurity struct {
	Options        SecurityOptions
	IPs            []SecurityEntry
	DomainMasks    []SecurityEntry
	Referrers      []SecurityEntry
	JWTs           []JWT
	RequestFilters []RequestFilter
	Tokens         []EndpointToken
}

func (c *Client) GetEndpointSecurity(ctx context.Context, endpointID string) (*EndpointSecurity, error) {
	const operation = "read endpoint security"

	resp, err := c.api.GetV0EndpointsByIdSecurityWithResponse(ctx, endpointID)
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
	if resp.JSON200.Data == nil {
		return nil, &Error{Operation: operation, Status: http.StatusNotFound, Message: "endpoint not found"}
	}

	data := resp.JSON200.Data
	security := &EndpointSecurity{}

	if data.Options != nil {
		security.Options = SecurityOptions{
			Tokens:          deref(data.Options.Tokens),
			Referrers:       deref(data.Options.Referrers),
			JWTs:            deref(data.Options.Jwts),
			IPs:             deref(data.Options.Ips),
			DomainMasks:     deref(data.Options.DomainMasks),
			HSTS:            deref(data.Options.Hsts),
			Cors:            deref(data.Options.Cors),
			RequestFilters:  deref(data.Options.RequestFilters),
			ResponseLogging: deref(data.Options.ResponseLogging),
		}
		if data.Options.IpCustomHeader != nil {
			security.Options.IPCustomHeader = deref(data.Options.IpCustomHeader.Value)
		}
	}

	if data.Ips != nil {
		for _, raw := range *data.Ips {
			security.IPs = append(security.IPs, SecurityEntry{ID: deref(raw.Id), Value: deref(raw.Ip)})
		}
	}
	if data.DomainMasks != nil {
		for _, raw := range *data.DomainMasks {
			security.DomainMasks = append(security.DomainMasks, SecurityEntry{ID: deref(raw.Id), Value: deref(raw.Domain)})
		}
	}
	if data.Referrers != nil {
		for _, raw := range *data.Referrers {
			security.Referrers = append(security.Referrers, SecurityEntry{ID: deref(raw.Id), Value: deref(raw.Referrer)})
		}
	}
	if data.Jwts != nil {
		for _, raw := range *data.Jwts {
			security.JWTs = append(security.JWTs, JWT{
				ID:        deref(raw.Id),
				Name:      deref(raw.Name),
				KID:       deref(raw.Kid),
				PublicKey: deref(raw.PublicKey),
			})
		}
	}
	if data.RequestFilters != nil {
		for _, raw := range *data.RequestFilters {
			filter := RequestFilter{ID: deref(raw.Id)}
			if raw.Method != nil {
				filter.Methods = append(filter.Methods, *raw.Method...)
			}
			security.RequestFilters = append(security.RequestFilters, filter)
		}
	}
	if data.Tokens != nil {
		for _, raw := range *data.Tokens {
			security.Tokens = append(security.Tokens, EndpointToken{ID: deref(raw.Id), Value: deref(raw.Token)})
		}
	}

	return security, nil
}

// SetSecurityOptions writes the settable toggles. The read path reports them as
// booleans and the write path takes the strings "enabled" and "disabled", so
// the conversion happens here rather than in every caller. A patch that manages
// nothing is a no-op rather than an empty write.
func (c *Client) SetSecurityOptions(ctx context.Context, endpointID string, patch SecurityOptionsPatch) error {
	const operation = "update endpoint security options"

	if patch.Tokens == nil && patch.Referrers == nil && patch.JWTs == nil && patch.IPs == nil &&
		patch.DomainMasks == nil && patch.HSTS == nil && patch.Cors == nil {
		return nil
	}

	body := admin.PatchV0EndpointsByIdSecurityOptionsJSONRequestBody{}
	body.Options = &struct {
		Cors        *string `json:"cors,omitempty"`
		DomainMasks *string `json:"domainMasks,omitempty"`
		Hsts        *string `json:"hsts,omitempty"`
		Ips         *string `json:"ips,omitempty"`
		Jwts        *string `json:"jwts,omitempty"`
		Referrers   *string `json:"referrers,omitempty"`
		Tokens      *string `json:"tokens,omitempty"`
	}{
		Tokens:      toggle(patch.Tokens),
		Referrers:   toggle(patch.Referrers),
		Jwts:        toggle(patch.JWTs),
		Ips:         toggle(patch.IPs),
		DomainMasks: toggle(patch.DomainMasks),
		Hsts:        toggle(patch.HSTS),
		Cors:        toggle(patch.Cors),
	}

	resp, err := c.api.PatchV0EndpointsByIdSecurityOptionsWithResponse(ctx, endpointID, body)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func toggle(value *bool) *string {
	if value == nil {
		return nil
	}
	setting := optionDisabled
	if *value {
		setting = optionEnabled
	}
	return &setting
}

func (c *Client) SetIPCustomHeader(ctx context.Context, endpointID, headerName string) error {
	const operation = "set endpoint ip custom header"

	resp, err := c.api.PatchV0EndpointsByIdIpCustomHeaderWithResponse(ctx, endpointID,
		admin.PatchV0EndpointsByIdIpCustomHeaderJSONRequestBody{HeaderName: headerName})
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) DeleteIPCustomHeader(ctx context.Context, endpointID string) error {
	const operation = "clear endpoint ip custom header"

	resp, err := c.api.DeleteV0EndpointsByIdIpCustomHeaderWithResponse(ctx, endpointID)
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

func (c *Client) AddEndpointIP(ctx context.Context, endpointID, ip string) (*SecurityEntry, error) {
	const operation = "add endpoint ip"

	resp, err := c.api.PostV0EndpointsByIdSecurityIpsWithResponse(ctx, endpointID,
		admin.PostV0EndpointsByIdSecurityIpsJSONRequestBody{Ip: ip})
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}
	return &SecurityEntry{ID: *resp.JSON200.Data.Id, Value: deref(resp.JSON200.Data.Ip)}, nil
}

func (c *Client) RemoveEndpointIP(ctx context.Context, endpointID, ipID string) error {
	const operation = "remove endpoint ip"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityIpsByIpIdWithResponse(ctx, endpointID, ipID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return deleteResult(operation, resp.StatusCode(), resp.JSON200.Data, resp.JSON200.Error)
}

func (c *Client) AddDomainMask(ctx context.Context, endpointID, domainMask string) (*SecurityEntry, error) {
	const operation = "add endpoint domain mask"

	resp, err := c.api.PostV0EndpointsByIdSecurityDomainMasksWithResponse(ctx, endpointID,
		admin.PostV0EndpointsByIdSecurityDomainMasksJSONRequestBody{DomainMask: domainMask})
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}
	return &SecurityEntry{ID: *resp.JSON200.Data.Id, Value: deref(resp.JSON200.Data.DomainMask)}, nil
}

func (c *Client) RemoveDomainMask(ctx context.Context, endpointID, domainMaskID string) error {
	const operation = "remove endpoint domain mask"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityDomainMasksByDomainMaskIdWithResponse(ctx, endpointID, domainMaskID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return deleteResult(operation, resp.StatusCode(), resp.JSON200.Data, resp.JSON200.Error)
}

func (c *Client) AddReferrer(ctx context.Context, endpointID, referrer string) (*SecurityEntry, error) {
	const operation = "add endpoint referrer"

	resp, err := c.api.PostV0EndpointsByIdSecurityReferrersWithResponse(ctx, endpointID,
		admin.PostV0EndpointsByIdSecurityReferrersJSONRequestBody{Referrer: referrer})
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}
	return &SecurityEntry{ID: *resp.JSON200.Data.Id, Value: deref(resp.JSON200.Data.Referrer)}, nil
}

func (c *Client) RemoveReferrer(ctx context.Context, endpointID, referrerID string) error {
	const operation = "remove endpoint referrer"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityReferrersByReferrerIdWithResponse(ctx, endpointID, referrerID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return deleteResult(operation, resp.StatusCode(), resp.JSON200.Data, resp.JSON200.Error)
}

func (c *Client) AddJWT(ctx context.Context, endpointID string, jwt JWT) (*JWT, error) {
	const operation = "add endpoint jwt"

	body := admin.PostV0EndpointsByIdSecurityJwtsJSONRequestBody{}
	if jwt.Name != "" {
		body.Name = &jwt.Name
	}
	if jwt.KID != "" {
		body.Kid = &jwt.KID
	}
	if jwt.PublicKey != "" {
		body.PublicKey = &jwt.PublicKey
	}

	resp, err := c.api.PostV0EndpointsByIdSecurityJwtsWithResponse(ctx, endpointID, body)
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}

	data := resp.JSON200.Data
	return &JWT{
		ID:        *data.Id,
		Name:      deref(data.Name),
		KID:       deref(data.Kid),
		PublicKey: deref(data.PublicKey),
	}, nil
}

func (c *Client) RemoveJWT(ctx context.Context, endpointID, jwtID string) error {
	const operation = "remove endpoint jwt"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityJwtsByJwtIdWithResponse(ctx, endpointID, jwtID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return deleteResult(operation, resp.StatusCode(), resp.JSON200.Data, resp.JSON200.Error)
}

func (c *Client) AddRequestFilter(ctx context.Context, endpointID string, methods []string) (*RequestFilter, error) {
	const operation = "add endpoint request filter"

	body := admin.PostV0EndpointsByIdSecurityRequestFiltersJSONRequestBody{Method: &methods}

	resp, err := c.api.PostV0EndpointsByIdSecurityRequestFiltersWithResponse(ctx, endpointID, body)
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}
	return &RequestFilter{ID: *resp.JSON200.Data.Id, Methods: methods}, nil
}

func (c *Client) UpdateRequestFilter(ctx context.Context, endpointID, filterID string, methods []string) error {
	const operation = "update endpoint request filter"

	body := admin.PutV0EndpointsByIdSecurityRequestFiltersByRequestFilterIdJSONRequestBody{Method: &methods}

	resp, err := c.api.PutV0EndpointsByIdSecurityRequestFiltersByRequestFilterIdWithResponse(ctx, endpointID, filterID, body)
	if err != nil {
		return err
	}
	return noContentResult(operation, resp.StatusCode(), resp.Body)
}

func (c *Client) RemoveRequestFilter(ctx context.Context, endpointID, filterID string) error {
	const operation = "remove endpoint request filter"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityRequestFiltersByRequestFilterIdWithResponse(ctx, endpointID, filterID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	return noContentResult(operation, resp.StatusCode(), resp.Body)
}

func (c *Client) AddEndpointToken(ctx context.Context, endpointID string) (*EndpointToken, error) {
	const operation = "add endpoint token"

	resp, err := c.api.PostV0EndpointsByIdSecurityTokensWithResponse(ctx, endpointID)
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no id"}
	}
	return &EndpointToken{ID: *resp.JSON200.Data.Id, Value: deref(resp.JSON200.Data.Token)}, nil
}

func (c *Client) RemoveEndpointToken(ctx context.Context, endpointID, tokenID string) error {
	const operation = "remove endpoint token"

	resp, err := c.api.DeleteV0EndpointsByIdSecurityTokensByTokenIdWithResponse(ctx, endpointID, tokenID)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return deleteResult(operation, resp.StatusCode(), resp.JSON200.Data, resp.JSON200.Error)
}

// deleteResult reads the {data: bool, error: string} envelope the security
// delete routes return. A 404 is treated as success by the callers, since a
// removed entry is the state the caller asked for.
func deleteResult(operation string, status int, data *bool, apiError *string) error {
	if err := envelopeError(operation, status, apiError); err != nil {
		return err
	}
	if data != nil && !*data {
		return &Error{Operation: operation, Status: status, Message: "the API reported the delete did not succeed"}
	}
	return nil
}

// noContentResult handles the routes that answer 204 with an empty body rather
// than the JSON envelope the rest of the Admin API uses.
func noContentResult(operation string, status int, body []byte) error {
	if status < 200 || status > 299 {
		return statusError(operation, status, body)
	}
	return nil
}
