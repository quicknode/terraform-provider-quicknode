package client

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/quicknode/terraform-provider-quicknode/api/admin"
)

const DefaultBaseURL = "https://api.quicknode.com"

// URLTokenPlaceholder stands in for the auth token in SafeHTTPURL and
// SafeWSSURL. It keeps the shape of the real URL, including any path suffix the
// chain appends, so the token's position stays visible and a caller can
// substitute one without guessing where it goes.
const URLTokenPlaceholder = "REPLACE_WITH_TOKEN"

type Client struct {
	api *admin.ClientWithResponses
}

type Option func(*options)

type options struct {
	baseURL           string
	httpClient        *http.Client
	maxRetries        int
	requestsPerSecond int
}

func WithBaseURL(baseURL string) Option {
	return func(o *options) {
		if baseURL != "" {
			o.baseURL = baseURL
		}
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(o *options) { o.httpClient = httpClient }
}

func WithMaxRetries(maxRetries int) Option {
	return func(o *options) { o.maxRetries = maxRetries }
}

// WithRequestsPerSecond throttles outbound calls. A terraform apply over a large
// workspace bursts many Admin API calls at once, so the provider paces itself
// and does not lean on the API to reject the excess.
func WithRequestsPerSecond(requestsPerSecond int) Option {
	return func(o *options) {
		if requestsPerSecond > 0 {
			o.requestsPerSecond = requestsPerSecond
		}
	}
}

func New(apiKey string, opts ...Option) (*Client, error) {
	settings := options{
		baseURL:           DefaultBaseURL,
		maxRetries:        defaultMaxRetries,
		requestsPerSecond: defaultRequestsPerSecond,
	}
	for _, opt := range opts {
		opt(&settings)
	}

	httpClient := settings.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	httpClient.Transport = &retryTransport{
		base:       httpClient.Transport,
		apiKey:     apiKey,
		maxRetries: settings.maxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
		limiter: rate.NewLimiter(
			rate.Limit(settings.requestsPerSecond),
			settings.requestsPerSecond,
		),
	}

	api, err := admin.NewClientWithResponses(settings.baseURL, admin.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	return &Client{api: api}, nil
}

type Network struct {
	Slug    string
	Name    string
	ChainID *int64
}

type Chain struct {
	Slug          string
	IsSelectChain bool
	Networks      []Network
}

type Endpoint struct {
	ID      string
	Chain   string
	Network string
	Label   string
	Status  string

	// SafeHTTPURL and SafeWSSURL carry URLTokenPlaceholder where the token
	// belongs, so they can be logged or displayed. HTTPURLWithToken and
	// WSSURLWithToken are what the API returned, credential included.
	SafeHTTPURL      string
	SafeWSSURL       string
	HTTPURLWithToken string
	WSSURLWithToken  string

	Multichain bool
	Tokens     []EndpointToken
	Tags       []Tag
	Security   SecurityOptions
}

// EndpointToken is one of an endpoint's auth tokens. An endpoint can carry
// several, so this is a list.
type EndpointToken struct {
	ID    string
	Value string
}

type Tag struct {
	ID    int64
	Label string
}

func (c *Client) ListChains(ctx context.Context) ([]Chain, error) {
	const operation = "list chains"

	resp, err := c.api.GetV0ChainsWithResponse(ctx)
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
		return nil, nil
	}

	chains := make([]Chain, 0, len(*resp.JSON200.Data))
	for _, raw := range *resp.JSON200.Data {
		chain := Chain{Slug: deref(raw.Slug), IsSelectChain: deref(raw.IsSelectChain)}
		if raw.Networks != nil {
			for _, rawNetwork := range *raw.Networks {
				network := Network{Slug: deref(rawNetwork.Slug), Name: deref(rawNetwork.Name)}
				if rawNetwork.ChainId != nil {
					chainID := int64(*rawNetwork.ChainId)
					network.ChainID = &chainID
				}
				chain.Networks = append(chain.Networks, network)
			}
		}
		chains = append(chains, chain)
	}
	return chains, nil
}

func (c *Client) CreateEndpoint(ctx context.Context, chain, network string) (*Endpoint, error) {
	const operation = "create endpoint"

	resp, err := c.api.PostV0EndpointsWithResponse(ctx, admin.PostV0EndpointsJSONRequestBody{
		Chain:   chain,
		Network: network,
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
		return nil, &Error{Operation: operation, Status: resp.StatusCode(), Message: "response carried no endpoint id"}
	}

	data := resp.JSON200.Data
	endpoint := &Endpoint{
		ID:      *data.Id,
		Chain:   deref(data.Chain),
		Network: deref(data.Network),
		Label:   deref(data.Label),
	}
	endpoint.setURLs(deref(data.HttpUrl), deref(data.WssUrl))
	if data.Security != nil && data.Security.Tokens != nil {
		for _, rawToken := range *data.Security.Tokens {
			endpoint.Tokens = append(endpoint.Tokens, EndpointToken{
				ID:    deref(rawToken.Id),
				Value: deref(rawToken.Token),
			})
		}
	}
	return endpoint, nil
}

func (c *Client) GetEndpoint(ctx context.Context, id string) (*Endpoint, error) {
	const operation = "read endpoint"

	resp, err := c.api.GetV0EndpointsByIdWithResponse(ctx, id)
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
	endpoint := &Endpoint{
		ID:         deref(data.Id),
		Chain:      deref(data.Chain),
		Network:    deref(data.Network),
		Label:      deref(data.Label),
		Status:     deref(data.Status),
		Multichain: deref(data.IsMultichain),
	}
	endpoint.setURLs(deref(data.HttpUrl), deref(data.WssUrl))
	if data.Security != nil && data.Security.Options != nil {
		options := data.Security.Options
		endpoint.Security = SecurityOptions{
			Tokens:          deref(options.Tokens),
			Referrers:       deref(options.Referrers),
			JWTs:            deref(options.Jwts),
			IPs:             deref(options.Ips),
			DomainMasks:     deref(options.DomainMasks),
			HSTS:            deref(options.Hsts),
			Cors:            deref(options.Cors),
			RequestFilters:  deref(options.RequestFilters),
			ResponseLogging: deref(options.ResponseLogging),
		}
		if options.IpCustomHeader != nil {
			endpoint.Security.IPCustomHeader = deref(options.IpCustomHeader.Value)
		}
	}
	if data.Security != nil && data.Security.Tokens != nil {
		for _, rawToken := range *data.Security.Tokens {
			endpoint.Tokens = append(endpoint.Tokens, EndpointToken{
				ID:    deref(rawToken.Id),
				Value: deref(rawToken.Token),
			})
		}
	}

	if data.Tags != nil {
		for _, rawTag := range *data.Tags {
			tag := Tag{Label: deref(rawTag.Label)}
			if rawTag.TagId != nil {
				tag.ID = int64(*rawTag.TagId)
			}
			endpoint.Tags = append(endpoint.Tags, tag)
		}
	}
	return endpoint, nil
}

// NetworkURLs is one network's URLs, in the same safe and credentialed forms as
// Endpoint's.
type NetworkURLs struct {
	SafeHTTPURL      string
	SafeWSSURL       string
	HTTPURLWithToken string
	WSSURLWithToken  string
}

// EndpointURLs is what the URLs route returns: the endpoint's own network, and
// for a multichain endpoint every network it serves, keyed by network slug.
type EndpointURLs struct {
	NetworkURLs
	Multichain map[string]NetworkURLs
}

func newNetworkURLs(httpURL, wssURL string) NetworkURLs {
	return NetworkURLs{
		SafeHTTPURL:      RedactEndpointURL(httpURL),
		SafeWSSURL:       RedactEndpointURL(wssURL),
		HTTPURLWithToken: httpURL,
		WSSURLWithToken:  wssURL,
	}
}

func (c *Client) GetEndpointURLs(ctx context.Context, id string) (*EndpointURLs, error) {
	const operation = "read endpoint urls"

	resp, err := c.api.GetV0EndpointsByIdUrlsWithResponse(ctx, id)
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
	urls := &EndpointURLs{
		NetworkURLs: newNetworkURLs(deref(data.HttpUrl), deref(data.WssUrl)),
		Multichain:  map[string]NetworkURLs{},
	}
	if data.MultichainUrls != nil {
		for network, raw := range *data.MultichainUrls {
			urls.Multichain[network] = newNetworkURLs(deref(raw.HttpUrl), deref(raw.WssUrl))
		}
	}
	return urls, nil
}

func (c *Client) SetEndpointLabel(ctx context.Context, id, label string) error {
	const operation = "set endpoint label"

	resp, err := c.api.PatchV0EndpointsByIdWithResponse(ctx, id, admin.PatchV0EndpointsByIdJSONRequestBody{
		Label: label,
	})
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) SetEndpointStatus(ctx context.Context, id, status string) error {
	const operation = "set endpoint status"

	resp, err := c.api.PatchV0EndpointsByIdStatusWithResponse(ctx, id, admin.PatchV0EndpointsByIdStatusJSONRequestBody{
		Status: status,
	})
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) SetEndpointMultichain(ctx context.Context, id string, enabled bool) error {
	operation := "disable endpoint multichain"
	if enabled {
		operation = "enable endpoint multichain"
	}

	if enabled {
		resp, err := c.api.PostV0EndpointsByIdEnableMultichainWithResponse(ctx, id)
		if err != nil {
			return err
		}
		if resp.JSON200 == nil {
			return statusError(operation, resp.StatusCode(), resp.Body)
		}
		return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
	}

	resp, err := c.api.PostV0EndpointsByIdDisableMultichainWithResponse(ctx, id)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) DeleteEndpoint(ctx context.Context, id string) error {
	const operation = "delete endpoint"

	resp, err := c.api.DeleteV0EndpointsByIdWithResponse(ctx, id)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	if resp.JSON200.Result != nil && !*resp.JSON200.Result {
		return &Error{Operation: operation, Status: resp.StatusCode(), Message: "the API reported the endpoint was not deleted"}
	}
	return nil
}

func (c *Client) AddEndpointTag(ctx context.Context, id, label string) error {
	const operation = "add endpoint tag"

	resp, err := c.api.PostV0EndpointsByIdTagsWithResponse(ctx, id, admin.PostV0EndpointsByIdTagsJSONRequestBody{
		Label: label,
	})
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return statusError(operation, resp.StatusCode(), resp.Body)
	}
	return envelopeError(operation, resp.StatusCode(), resp.JSON200.Error)
}

func (c *Client) RemoveEndpointTag(ctx context.Context, id string, tagID int64) error {
	const operation = "remove endpoint tag"

	resp, err := c.api.DeleteV0EndpointsByIdTagsByTagIdWithResponse(ctx, id, strconv.FormatInt(tagID, 10))
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

func (e *Endpoint) setURLs(httpURL, wssURL string) {
	e.HTTPURLWithToken = httpURL
	e.WSSURLWithToken = wssURL
	e.SafeHTTPURL = RedactEndpointURL(httpURL)
	e.SafeWSSURL = RedactEndpointURL(wssURL)
}

// RedactEndpointURL replaces the credential in an endpoint URL with
// URLTokenPlaceholder. The Admin API returns URLs shaped
// https://<subdomain>.quiknode.pro/<token>[/<suffix>], and the suffix differs by
// chain, so the placeholder goes in the token's position and nothing is cut
// out. The result keeps the real URL's shape and is safe to log.
func RedactEndpointURL(raw string) string {
	return EndpointURLWithToken(raw, URLTokenPlaceholder)
}

// EndpointURLWithToken puts token in the credential's position of an endpoint
// URL, whether that position holds a real token or URLTokenPlaceholder.
func EndpointURLWithToken(raw, token string) string {
	if raw == "" {
		return ""
	}
	scheme, rest, found := strings.Cut(raw, "://")
	if !found {
		return raw
	}
	host, path, found := strings.Cut(rest, "/")
	if !found || path == "" {
		return raw
	}

	_, suffix, hadSuffix := strings.Cut(path, "/")
	rebuilt := scheme + "://" + host + "/" + token
	if hadSuffix {
		rebuilt += "/" + suffix
	}
	return rebuilt
}

func deref[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}
