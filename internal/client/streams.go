package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const streamsPath = "/streams/rest/v1/streams"

const (
	StreamStatusActive     = "active"
	StreamStatusPaused     = "paused"
	StreamStatusTerminated = "terminated"
	StreamStatusCompleted  = "completed"
)

const (
	DestinationWebhook  = "webhook"
	DestinationS3       = "s3"
	DestinationAzure    = "azure"
	DestinationPostgres = "postgres"
	DestinationKafka    = "kafka"
)

// StreamRangeUnset is what the API reports for an end_range that runs forever,
// and what it accepts as a start_range meaning the latest block.
const StreamRangeUnset int64 = -1

type Stream struct {
	ID      string
	Name    string
	Status  string
	Network string
	Dataset string
	Region  string

	// FilterFunction is the filter's source code. The API carries it base64
	// encoded; the client encodes and decodes it.
	FilterFunction string
	FilterLanguage string

	StartRange           int64
	EndRange             int64
	DatasetBatchSize     int64
	ElasticBatchEnabled  bool
	RestreamBatchOnReorg bool
	FixBlockReorgs       int64
	KeepDistanceFromTip  int64
	NotificationEmail    string

	// Sequence is the last block delivered. It moves on its own while the
	// stream runs.
	Sequence int64

	Destination       StreamDestination
	ExtraDestinations []StreamDestination
}

type StreamInput struct {
	Name           string
	Network        string
	Dataset        string
	Region         string
	Status         string
	FilterFunction string
	FilterLanguage string

	StartRange           *int64
	EndRange             *int64
	DatasetBatchSize     int64
	ElasticBatchEnabled  bool
	RestreamBatchOnReorg *bool
	FixBlockReorgs       *int64
	KeepDistanceFromTip  *int64
	NotificationEmail    *string

	Destination       StreamDestination
	ExtraDestinations []StreamDestination
}

// StreamUpdate sends only the fields that are set. Network, dataset and region
// cannot change after creation, and status changes go through PauseStream and
// ActivateStream, which run the plan checks a PATCH skips.
type StreamUpdate struct {
	Name           *string
	FilterFunction *string
	FilterLanguage *string

	StartRange           *int64
	EndRange             *int64
	DatasetBatchSize     *int64
	ElasticBatchEnabled  *bool
	RestreamBatchOnReorg *bool
	FixBlockReorgs       *int64
	KeepDistanceFromTip  *int64
	NotificationEmail    *string

	Destination       *StreamDestination
	ExtraDestinations *[]StreamDestination
}

// StreamDestination holds exactly one destination type. A type the API reports
// but the client does not model, such as one configured in the dashboard,
// leaves every field nil and is named by Type.
type StreamDestination struct {
	Webhook  *WebhookDestination
	S3       *S3Destination
	Azure    *AzureDestination
	Postgres *PostgresDestination
	Kafka    *KafkaDestination

	unknownType string
}

type WebhookDestination struct {
	URL              string    `json:"url"`
	Compression      string    `json:"compression"`
	Headers          HeaderMap `json:"headers"`
	MaxRetry         int64     `json:"max_retry"`
	RetryIntervalSec int64     `json:"retry_interval_sec"`
	PostTimeoutSec   int64     `json:"post_timeout_sec"`
	SecurityToken    string    `json:"security_token,omitempty"`
	MTLS             *bool     `json:"mtls,omitempty"`
	SSLCAPEM         string    `json:"ssl_ca_pem,omitempty"`
}

type S3Destination struct {
	Endpoint            string `json:"endpoint"`
	Bucket              string `json:"bucket"`
	Region              string `json:"region,omitempty"`
	ObjectPrefix        string `json:"object_prefix"`
	FileType            string `json:"file_type"`
	FileCompressionType string `json:"file_compression_type"`
	UseSSL              bool   `json:"use_ssl"`
	AccessKey           string `json:"access_key"`
	SecretKey           string `json:"secret_key"`
	MaxRetry            int64  `json:"max_retry"`
	RetryIntervalSec    int64  `json:"retry_interval_sec"`
}

type AzureDestination struct {
	StorageAccount      string `json:"storage_account"`
	Container           string `json:"container"`
	BlobPrefix          string `json:"blob_prefix"`
	FileType            string `json:"file_type"`
	FileCompressionType string `json:"file_compression_type"`
	SASToken            string `json:"sas_token,omitempty"`
	MaxRetry            int64  `json:"max_retry"`
	RetryIntervalSec    int64  `json:"retry_interval_sec"`
}

type PostgresDestination struct {
	Host             string `json:"host"`
	Port             int64  `json:"port"`
	Database         string `json:"database"`
	TableName        string `json:"table_name"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	SSLMode          string `json:"sslmode"`
	MaxRetry         int64  `json:"max_retry"`
	RetryIntervalSec int64  `json:"retry_interval_sec"`
}

type KafkaDestination struct {
	BootstrapServers  string `json:"bootstrap_servers"`
	TopicName         string `json:"topic_name"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	Mechanisms        string `json:"mechanisms,omitempty"`
	Protocol          string `json:"protocol,omitempty"`
	CompressionType   string `json:"compression_type"`
	BatchSize         int64  `json:"batch_size"`
	LingerMs          int64  `json:"linger_ms"`
	MaxMessageBytes   int64  `json:"max_message_bytes"`
	TimeoutSec        int64  `json:"timeout_sec"`
	MaxRetry          int64  `json:"max_retry"`
	RetryIntervalSec  int64  `json:"retry_interval_sec"`
	SSLCAPEM          string `json:"ssl_ca_pem,omitempty"`
	SSLCertificatePEM string `json:"ssl_certificate_pem,omitempty"`
	SSLKeyPEM         string `json:"ssl_key_pem,omitempty"`
}

// HeaderMap is a webhook's extra request headers. The API stores header values
// as strings or numbers, so decoding accepts both, and it rejects a missing
// headers object, so a nil map encodes as an empty one.
type HeaderMap map[string]string

func (h HeaderMap) MarshalJSON() ([]byte, error) {
	if h == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]string(h))
}

func (h *HeaderMap) UnmarshalJSON(raw []byte) error {
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	decoded := make(HeaderMap, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			decoded[key] = typed
		case float64:
			decoded[key] = strconv.FormatFloat(typed, 'f', -1, 64)
		case bool:
			decoded[key] = strconv.FormatBool(typed)
		case nil:
			decoded[key] = ""
		default:
			return fmt.Errorf("header %q has unsupported value %v", key, value)
		}
	}
	*h = decoded
	return nil
}

func (d StreamDestination) Type() string {
	switch {
	case d.Webhook != nil:
		return DestinationWebhook
	case d.S3 != nil:
		return DestinationS3
	case d.Azure != nil:
		return DestinationAzure
	case d.Postgres != nil:
		return DestinationPostgres
	case d.Kafka != nil:
		return DestinationKafka
	default:
		return d.unknownType
	}
}

func (d StreamDestination) attributes() (any, error) {
	switch {
	case d.Webhook != nil:
		return d.Webhook, nil
	case d.S3 != nil:
		return d.S3, nil
	case d.Azure != nil:
		return d.Azure, nil
	case d.Postgres != nil:
		return d.Postgres, nil
	case d.Kafka != nil:
		return d.Kafka, nil
	default:
		return nil, fmt.Errorf("destination type %q is not supported", d.unknownType)
	}
}

type destinationWire struct {
	Destination string          `json:"destination"`
	Attributes  json.RawMessage `json:"destination_attributes"`
}

func (d StreamDestination) MarshalJSON() ([]byte, error) {
	attributes, err := d.attributes()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(attributes)
	if err != nil {
		return nil, err
	}
	return json.Marshal(destinationWire{Destination: d.Type(), Attributes: raw})
}

func (d *StreamDestination) UnmarshalJSON(raw []byte) error {
	var wire destinationWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	decoded, err := decodeDestination(wire.Destination, wire.Attributes)
	if err != nil {
		return err
	}
	*d = decoded
	return nil
}

func decodeDestination(kind string, raw json.RawMessage) (StreamDestination, error) {
	var destination StreamDestination
	var target any
	switch kind {
	case DestinationWebhook:
		destination.Webhook = &WebhookDestination{}
		target = destination.Webhook
	case DestinationS3:
		destination.S3 = &S3Destination{}
		target = destination.S3
	case DestinationAzure:
		destination.Azure = &AzureDestination{}
		target = destination.Azure
	case DestinationPostgres:
		destination.Postgres = &PostgresDestination{}
		target = destination.Postgres
	case DestinationKafka:
		destination.Kafka = &KafkaDestination{}
		target = destination.Kafka
	default:
		return StreamDestination{unknownType: kind}, nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		return destination, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return StreamDestination{}, fmt.Errorf("decoding %s destination: %w", kind, err)
	}
	return destination, nil
}

type streamWire struct {
	ID                    string              `json:"id"`
	Name                  string              `json:"name"`
	Status                string              `json:"status"`
	Network               string              `json:"network"`
	Dataset               string              `json:"dataset"`
	Region                string              `json:"region"`
	FilterFunction        string              `json:"filter_function"`
	FilterLanguage        string              `json:"filter_language"`
	StartRange            int64               `json:"start_range"`
	EndRange              int64               `json:"end_range"`
	DatasetBatchSize      int64               `json:"dataset_batch_size"`
	ElasticBatchEnabled   bool                `json:"elastic_batch_enabled"`
	RestreamBatchOnReorg  bool                `json:"restream_batch_on_reorg"`
	FixBlockReorgs        int64               `json:"fix_block_reorgs"`
	KeepDistanceFromTip   int64               `json:"keep_distance_from_tip"`
	NotificationEmail     string              `json:"notification_email"`
	Sequence              int64               `json:"sequence"`
	Destination           string              `json:"destination"`
	DestinationAttributes json.RawMessage     `json:"destination_attributes"`
	ExtraDestinations     []StreamDestination `json:"extra_destinations"`
}

func (w streamWire) toStream() (*Stream, error) {
	filter, err := decodeFilter(w.FilterFunction)
	if err != nil {
		return nil, err
	}
	destination, err := decodeDestination(w.Destination, w.DestinationAttributes)
	if err != nil {
		return nil, err
	}
	extras := w.ExtraDestinations
	if extras == nil {
		extras = []StreamDestination{}
	}
	return &Stream{
		ID:                   w.ID,
		Name:                 w.Name,
		Status:               w.Status,
		Network:              w.Network,
		Dataset:              w.Dataset,
		Region:               w.Region,
		FilterFunction:       filter,
		FilterLanguage:       w.FilterLanguage,
		StartRange:           w.StartRange,
		EndRange:             w.EndRange,
		DatasetBatchSize:     w.DatasetBatchSize,
		ElasticBatchEnabled:  w.ElasticBatchEnabled,
		RestreamBatchOnReorg: w.RestreamBatchOnReorg,
		FixBlockReorgs:       w.FixBlockReorgs,
		KeepDistanceFromTip:  w.KeepDistanceFromTip,
		NotificationEmail:    w.NotificationEmail,
		Sequence:             w.Sequence,
		Destination:          destination,
		ExtraDestinations:    extras,
	}, nil
}

type streamCreateWire struct {
	Name                  string              `json:"name"`
	Network               string              `json:"network"`
	Dataset               string              `json:"dataset"`
	Region                string              `json:"region"`
	Status                string              `json:"status"`
	FilterFunction        string              `json:"filter_function,omitempty"`
	FilterLanguage        string              `json:"filter_language,omitempty"`
	StartRange            *int64              `json:"start_range,omitempty"`
	EndRange              *int64              `json:"end_range,omitempty"`
	DatasetBatchSize      int64               `json:"dataset_batch_size"`
	ElasticBatchEnabled   bool                `json:"elastic_batch_enabled"`
	RestreamBatchOnReorg  *bool               `json:"restream_batch_on_reorg,omitempty"`
	FixBlockReorgs        *int64              `json:"fix_block_reorgs,omitempty"`
	KeepDistanceFromTip   *int64              `json:"keep_distance_from_tip,omitempty"`
	NotificationEmail     *string             `json:"notification_email,omitempty"`
	Destination           string              `json:"destination"`
	DestinationAttributes any                 `json:"destination_attributes"`
	ExtraDestinations     []StreamDestination `json:"extra_destinations,omitempty"`
}

type streamUpdateWire struct {
	Name                  *string              `json:"name,omitempty"`
	FilterFunction        *string              `json:"filter_function,omitempty"`
	FilterLanguage        *string              `json:"filter_language,omitempty"`
	StartRange            *int64               `json:"start_range,omitempty"`
	EndRange              *int64               `json:"end_range,omitempty"`
	DatasetBatchSize      *int64               `json:"dataset_batch_size,omitempty"`
	ElasticBatchEnabled   *bool                `json:"elastic_batch_enabled,omitempty"`
	RestreamBatchOnReorg  *bool                `json:"restream_batch_on_reorg,omitempty"`
	FixBlockReorgs        *int64               `json:"fix_block_reorgs,omitempty"`
	KeepDistanceFromTip   *int64               `json:"keep_distance_from_tip,omitempty"`
	NotificationEmail     *string              `json:"notification_email,omitempty"`
	Destination           *string              `json:"destination,omitempty"`
	DestinationAttributes any                  `json:"destination_attributes,omitempty"`
	ExtraDestinations     *[]StreamDestination `json:"extra_destinations,omitempty"`
}

func encodeFilter(source string) string {
	if source == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(source))
}

func decodeFilter(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding filter_function: %w", err)
	}
	return string(decoded), nil
}

func streamPath(id string) string {
	return streamsPath + "/" + url.PathEscape(id)
}

func (c *Client) CreateStream(ctx context.Context, input StreamInput) (*Stream, error) {
	attributes, err := input.Destination.attributes()
	if err != nil {
		return nil, fmt.Errorf("create stream: %w", err)
	}

	body := streamCreateWire{
		Name:                  input.Name,
		Network:               input.Network,
		Dataset:               input.Dataset,
		Region:                input.Region,
		Status:                input.Status,
		FilterFunction:        encodeFilter(input.FilterFunction),
		FilterLanguage:        input.FilterLanguage,
		StartRange:            input.StartRange,
		EndRange:              input.EndRange,
		DatasetBatchSize:      input.DatasetBatchSize,
		ElasticBatchEnabled:   input.ElasticBatchEnabled,
		RestreamBatchOnReorg:  input.RestreamBatchOnReorg,
		FixBlockReorgs:        input.FixBlockReorgs,
		KeepDistanceFromTip:   input.KeepDistanceFromTip,
		NotificationEmail:     input.NotificationEmail,
		Destination:           input.Destination.Type(),
		DestinationAttributes: attributes,
		ExtraDestinations:     input.ExtraDestinations,
	}

	var wire streamWire
	if err := c.doJSON(ctx, "create stream", http.MethodPost, streamsPath, body, &wire); err != nil {
		return nil, err
	}
	return wire.toStream()
}

func (c *Client) GetStream(ctx context.Context, id string) (*Stream, error) {
	var wire streamWire
	if err := c.doJSON(ctx, "get stream", http.MethodGet, streamPath(id), nil, &wire); err != nil {
		return nil, err
	}
	return wire.toStream()
}

func (c *Client) UpdateStream(ctx context.Context, id string, update StreamUpdate) (*Stream, error) {
	body := streamUpdateWire{
		Name:                 update.Name,
		FilterLanguage:       update.FilterLanguage,
		StartRange:           update.StartRange,
		EndRange:             update.EndRange,
		DatasetBatchSize:     update.DatasetBatchSize,
		ElasticBatchEnabled:  update.ElasticBatchEnabled,
		RestreamBatchOnReorg: update.RestreamBatchOnReorg,
		FixBlockReorgs:       update.FixBlockReorgs,
		KeepDistanceFromTip:  update.KeepDistanceFromTip,
		NotificationEmail:    update.NotificationEmail,
		ExtraDestinations:    update.ExtraDestinations,
	}
	if update.FilterFunction != nil {
		encoded := encodeFilter(*update.FilterFunction)
		body.FilterFunction = &encoded
	}
	if update.Destination != nil {
		attributes, err := update.Destination.attributes()
		if err != nil {
			return nil, fmt.Errorf("update stream: %w", err)
		}
		kind := update.Destination.Type()
		body.Destination = &kind
		body.DestinationAttributes = attributes
	}

	var wire streamWire
	if err := c.doJSON(ctx, "update stream", http.MethodPatch, streamPath(id), body, &wire); err != nil {
		return nil, err
	}
	return wire.toStream()
}

func (c *Client) DeleteStream(ctx context.Context, id string) error {
	return c.doJSON(ctx, "delete stream", http.MethodDelete, streamPath(id), nil, nil)
}

func (c *Client) ActivateStream(ctx context.Context, id string) error {
	return c.doJSON(ctx, "activate stream", http.MethodPost, streamPath(id)+"/activate", nil, nil)
}

func (c *Client) PauseStream(ctx context.Context, id string) error {
	return c.doJSON(ctx, "pause stream", http.MethodPost, streamPath(id)+"/pause", nil, nil)
}
