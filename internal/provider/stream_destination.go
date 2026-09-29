package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var webhookDestinationAttrTypes = map[string]attr.Type{
	"url":                types.StringType,
	"compression":        types.StringType,
	"headers":            types.MapType{ElemType: types.StringType},
	"security_token":     types.StringType,
	"max_retry":          types.Int64Type,
	"retry_interval_sec": types.Int64Type,
	"post_timeout_sec":   types.Int64Type,
}

var s3DestinationAttrTypes = map[string]attr.Type{
	"endpoint":           types.StringType,
	"bucket":             types.StringType,
	"region":             types.StringType,
	"object_prefix":      types.StringType,
	"file_type":          types.StringType,
	"compression":        types.StringType,
	"use_ssl":            types.BoolType,
	"access_key":         types.StringType,
	"secret_key":         types.StringType,
	"max_retry":          types.Int64Type,
	"retry_interval_sec": types.Int64Type,
}

var azureDestinationAttrTypes = map[string]attr.Type{
	"storage_account":    types.StringType,
	"container":          types.StringType,
	"blob_prefix":        types.StringType,
	"file_type":          types.StringType,
	"compression":        types.StringType,
	"sas_token":          types.StringType,
	"max_retry":          types.Int64Type,
	"retry_interval_sec": types.Int64Type,
}

var postgresDestinationAttrTypes = map[string]attr.Type{
	"host":               types.StringType,
	"port":               types.Int64Type,
	"database":           types.StringType,
	"table_name":         types.StringType,
	"username":           types.StringType,
	"password":           types.StringType,
	"sslmode":            types.StringType,
	"max_retry":          types.Int64Type,
	"retry_interval_sec": types.Int64Type,
}

var kafkaDestinationAttrTypes = map[string]attr.Type{
	"bootstrap_servers":   types.StringType,
	"topic_name":          types.StringType,
	"username":            types.StringType,
	"password":            types.StringType,
	"mechanisms":          types.StringType,
	"protocol":            types.StringType,
	"compression":         types.StringType,
	"batch_size":          types.Int64Type,
	"linger_ms":           types.Int64Type,
	"max_message_bytes":   types.Int64Type,
	"timeout_sec":         types.Int64Type,
	"ssl_ca_pem":          types.StringType,
	"ssl_certificate_pem": types.StringType,
	"ssl_key_pem":         types.StringType,
	"max_retry":           types.Int64Type,
	"retry_interval_sec":  types.Int64Type,
}

var streamDestinationAttrTypes = map[string]attr.Type{
	client.DestinationWebhook:  types.ObjectType{AttrTypes: webhookDestinationAttrTypes},
	client.DestinationS3:       types.ObjectType{AttrTypes: s3DestinationAttrTypes},
	client.DestinationAzure:    types.ObjectType{AttrTypes: azureDestinationAttrTypes},
	client.DestinationPostgres: types.ObjectType{AttrTypes: postgresDestinationAttrTypes},
	client.DestinationKafka:    types.ObjectType{AttrTypes: kafkaDestinationAttrTypes},
}

type streamDestinationModel struct {
	Webhook  types.Object `tfsdk:"webhook"`
	S3       types.Object `tfsdk:"s3"`
	Azure    types.Object `tfsdk:"azure"`
	Postgres types.Object `tfsdk:"postgres"`
	Kafka    types.Object `tfsdk:"kafka"`
}

type webhookDestinationModel struct {
	URL              types.String `tfsdk:"url"`
	Compression      types.String `tfsdk:"compression"`
	Headers          types.Map    `tfsdk:"headers"`
	SecurityToken    types.String `tfsdk:"security_token"`
	MaxRetry         types.Int64  `tfsdk:"max_retry"`
	RetryIntervalSec types.Int64  `tfsdk:"retry_interval_sec"`
	PostTimeoutSec   types.Int64  `tfsdk:"post_timeout_sec"`
}

type s3DestinationModel struct {
	Endpoint         types.String `tfsdk:"endpoint"`
	Bucket           types.String `tfsdk:"bucket"`
	Region           types.String `tfsdk:"region"`
	ObjectPrefix     types.String `tfsdk:"object_prefix"`
	FileType         types.String `tfsdk:"file_type"`
	Compression      types.String `tfsdk:"compression"`
	UseSSL           types.Bool   `tfsdk:"use_ssl"`
	AccessKey        types.String `tfsdk:"access_key"`
	SecretKey        types.String `tfsdk:"secret_key"`
	MaxRetry         types.Int64  `tfsdk:"max_retry"`
	RetryIntervalSec types.Int64  `tfsdk:"retry_interval_sec"`
}

type azureDestinationModel struct {
	StorageAccount   types.String `tfsdk:"storage_account"`
	Container        types.String `tfsdk:"container"`
	BlobPrefix       types.String `tfsdk:"blob_prefix"`
	FileType         types.String `tfsdk:"file_type"`
	Compression      types.String `tfsdk:"compression"`
	SASToken         types.String `tfsdk:"sas_token"`
	MaxRetry         types.Int64  `tfsdk:"max_retry"`
	RetryIntervalSec types.Int64  `tfsdk:"retry_interval_sec"`
}

type postgresDestinationModel struct {
	Host             types.String `tfsdk:"host"`
	Port             types.Int64  `tfsdk:"port"`
	Database         types.String `tfsdk:"database"`
	TableName        types.String `tfsdk:"table_name"`
	Username         types.String `tfsdk:"username"`
	Password         types.String `tfsdk:"password"`
	SSLMode          types.String `tfsdk:"sslmode"`
	MaxRetry         types.Int64  `tfsdk:"max_retry"`
	RetryIntervalSec types.Int64  `tfsdk:"retry_interval_sec"`
}

type kafkaDestinationModel struct {
	BootstrapServers  types.String `tfsdk:"bootstrap_servers"`
	TopicName         types.String `tfsdk:"topic_name"`
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	Mechanisms        types.String `tfsdk:"mechanisms"`
	Protocol          types.String `tfsdk:"protocol"`
	Compression       types.String `tfsdk:"compression"`
	BatchSize         types.Int64  `tfsdk:"batch_size"`
	LingerMs          types.Int64  `tfsdk:"linger_ms"`
	MaxMessageBytes   types.Int64  `tfsdk:"max_message_bytes"`
	TimeoutSec        types.Int64  `tfsdk:"timeout_sec"`
	SSLCAPEM          types.String `tfsdk:"ssl_ca_pem"`
	SSLCertificatePEM types.String `tfsdk:"ssl_certificate_pem"`
	SSLKeyPEM         types.String `tfsdk:"ssl_key_pem"`
	MaxRetry          types.Int64  `tfsdk:"max_retry"`
	RetryIntervalSec  types.Int64  `tfsdk:"retry_interval_sec"`
}

func requiredString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
}

func optionalString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: description,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
}

func secretString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Sensitive:           true,
		MarkdownDescription: description,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
}

func requiredSecret(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		Sensitive:           true,
		MarkdownDescription: description,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
}

func defaultedString(description, value string, allowed ...string) schema.StringAttribute {
	attribute := schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(value),
		MarkdownDescription: fmt.Sprintf("%s Defaults to `%s`.", description, value),
	}
	if len(allowed) > 0 {
		attribute.Validators = []validator.String{stringvalidator.OneOf(allowed...)}
	}
	return attribute
}

func defaultedInt(description string, value, minimum int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(value),
		MarkdownDescription: fmt.Sprintf("%s Defaults to `%d`.", description, value),
		Validators:          []validator.Int64{int64validator.AtLeast(minimum)},
	}
}

func retryAttributes(attributes map[string]schema.Attribute) map[string]schema.Attribute {
	attributes["max_retry"] = defaultedInt("Delivery attempts before the stream is terminated.", 3, 1)
	attributes["retry_interval_sec"] = defaultedInt("Seconds between delivery attempts.", 1, 1)
	return attributes
}

// streamDestinationAttributes is shared by destination and each entry of
// extra_destinations. Exactly one type may be set; the resource checks that in
// ValidateConfig.
func streamDestinationAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		client.DestinationWebhook: schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Deliver each batch as an HTTP POST.",
			Attributes: retryAttributes(map[string]schema.Attribute{
				"url":         requiredString("URL the batches are posted to."),
				"compression": defaultedString("`none` or `gzip`.", "none", "none", "gzip"),
				"headers": schema.MapAttribute{
					Optional:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "Extra request headers. Omit the attribute rather than setting an empty map.",
					Validators:          []validator.Map{mapvalidator.SizeAtLeast(1)},
				},
				"security_token": schema.StringAttribute{
					Optional:  true,
					Computed:  true,
					Sensitive: true,
					MarkdownDescription: "Token used to sign each request, so the receiver can verify it came from Quicknode. " +
						"When omitted on the primary destination, Quicknode generates one and this attribute reports it.",
					PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				},
				"post_timeout_sec": defaultedInt("Seconds to wait for the receiver to respond.", 30, 1),
			}),
		},
		client.DestinationS3: schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Write each batch as an object to Amazon S3 or an S3-compatible store.",
			Attributes: retryAttributes(map[string]schema.Attribute{
				"endpoint":      defaultedString("S3 endpoint host.", "s3.amazonaws.com"),
				"bucket":        requiredString("Bucket name."),
				"region":        optionalString("Bucket region, for example `us-east-1`."),
				"object_prefix": defaultedString("Prefix added to each object name. It is joined to the file name with no separator, so end it with `/` to write into a folder.", ""),
				"file_type":     defaultedString("`.json` or `.parquet`.", ".json", ".json", ".parquet"),
				"compression":   defaultedString("`none` or `gzip`.", "none", "none", "gzip"),
				"use_ssl": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(true),
					MarkdownDescription: "Connect to the endpoint over TLS. Defaults to `true`.",
				},
				"access_key": requiredString("Access key id."),
				"secret_key": requiredSecret("Secret access key."),
			}),
		},
		client.DestinationAzure: schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Write each batch as a blob to Azure Storage.",
			Attributes: retryAttributes(map[string]schema.Attribute{
				"storage_account": requiredString("Storage account name."),
				"container":       requiredString("Container name."),
				"blob_prefix":     defaultedString("Prefix added to each blob name.", ""),
				"file_type":       defaultedString("`.json` or `.parquet`.", ".json", ".json", ".parquet"),
				"compression":     defaultedString("`none` or `gzip`.", "none", "none", "gzip"),
				"sas_token":       requiredSecret("Shared access signature token with write access to the container."),
			}),
		},
		client.DestinationPostgres: schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Insert each batch into a PostgreSQL table.",
			Attributes: retryAttributes(map[string]schema.Attribute{
				"host":       requiredString("Database host."),
				"port":       defaultedInt("Database port.", 5432, 1),
				"database":   requiredString("Database name."),
				"table_name": requiredString("Table the rows are written to."),
				"username":   requiredString("Database user."),
				"password":   requiredSecret("Database password."),
				"sslmode":    defaultedString("`require` or `disable`.", "require", "require", "disable"),
			}),
		},
		client.DestinationKafka: schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Produce each batch as messages to a Kafka topic.",
			Attributes: retryAttributes(map[string]schema.Attribute{
				"bootstrap_servers":   requiredString("Comma-separated `host:port` list of brokers."),
				"topic_name":          requiredString("Topic the messages are produced to."),
				"username":            optionalString("SASL user."),
				"password":            secretString("SASL password."),
				"mechanisms":          optionalString("SASL mechanism: `PLAIN`, `GSSAPI`, `SCRAM-SHA-256`, `SCRAM-SHA-512` or `OAUTHBEARER`."),
				"protocol":            defaultedString("`plaintext`, `ssl`, `sasl_ssl` or `sasl_plaintext`.", "plaintext", "plaintext", "ssl", "sasl_ssl", "sasl_plaintext"),
				"compression":         defaultedString("`none`, `gzip`, `snappy`, `lz4` or `zstd`.", "zstd", "none", "gzip", "snappy", "lz4", "zstd"),
				"batch_size":          defaultedInt("Producer batch size in bytes.", 16384, 1),
				"linger_ms":           defaultedInt("Milliseconds the producer waits to fill a batch.", 5, 0),
				"max_message_bytes":   defaultedInt("Largest message the producer sends, in bytes.", 1048576, 1),
				"timeout_sec":         defaultedInt("Seconds to wait for the brokers to acknowledge.", 30, 1),
				"ssl_ca_pem":          optionalString("PEM-encoded CA certificate that signed the brokers' certificates."),
				"ssl_certificate_pem": optionalString("PEM-encoded client certificate."),
				"ssl_key_pem":         secretString("PEM-encoded client private key."),
			}),
		},
	}
}

var streamDestinationObjectType = types.ObjectType{AttrTypes: streamDestinationAttrTypes}

var objectAsOptions = basetypes.ObjectAsOptions{}

func knownString(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func destinationFromObject(ctx context.Context, object types.Object) (client.StreamDestination, diag.Diagnostics) {
	var diags diag.Diagnostics
	var model streamDestinationModel
	diags.Append(object.As(ctx, &model, objectAsOptions)...)
	if diags.HasError() {
		return client.StreamDestination{}, diags
	}

	var destination client.StreamDestination
	switch {
	case !model.Webhook.IsNull():
		var webhook webhookDestinationModel
		diags.Append(model.Webhook.As(ctx, &webhook, objectAsOptions)...)
		headers := client.HeaderMap{}
		if !webhook.Headers.IsNull() && !webhook.Headers.IsUnknown() {
			diags.Append(webhook.Headers.ElementsAs(ctx, &headers, false)...)
		}
		destination.Webhook = &client.WebhookDestination{
			URL:              webhook.URL.ValueString(),
			Compression:      webhook.Compression.ValueString(),
			Headers:          headers,
			SecurityToken:    knownString(webhook.SecurityToken),
			MaxRetry:         webhook.MaxRetry.ValueInt64(),
			RetryIntervalSec: webhook.RetryIntervalSec.ValueInt64(),
			PostTimeoutSec:   webhook.PostTimeoutSec.ValueInt64(),
		}
	case !model.S3.IsNull():
		var s3 s3DestinationModel
		diags.Append(model.S3.As(ctx, &s3, objectAsOptions)...)
		destination.S3 = &client.S3Destination{
			Endpoint:            s3.Endpoint.ValueString(),
			Bucket:              s3.Bucket.ValueString(),
			Region:              knownString(s3.Region),
			ObjectPrefix:        s3.ObjectPrefix.ValueString(),
			FileType:            s3.FileType.ValueString(),
			FileCompressionType: s3.Compression.ValueString(),
			UseSSL:              s3.UseSSL.ValueBool(),
			AccessKey:           s3.AccessKey.ValueString(),
			SecretKey:           s3.SecretKey.ValueString(),
			MaxRetry:            s3.MaxRetry.ValueInt64(),
			RetryIntervalSec:    s3.RetryIntervalSec.ValueInt64(),
		}
	case !model.Azure.IsNull():
		var azure azureDestinationModel
		diags.Append(model.Azure.As(ctx, &azure, objectAsOptions)...)
		destination.Azure = &client.AzureDestination{
			StorageAccount:      azure.StorageAccount.ValueString(),
			Container:           azure.Container.ValueString(),
			BlobPrefix:          azure.BlobPrefix.ValueString(),
			FileType:            azure.FileType.ValueString(),
			FileCompressionType: azure.Compression.ValueString(),
			SASToken:            azure.SASToken.ValueString(),
			MaxRetry:            azure.MaxRetry.ValueInt64(),
			RetryIntervalSec:    azure.RetryIntervalSec.ValueInt64(),
		}
	case !model.Postgres.IsNull():
		var postgres postgresDestinationModel
		diags.Append(model.Postgres.As(ctx, &postgres, objectAsOptions)...)
		destination.Postgres = &client.PostgresDestination{
			Host:             postgres.Host.ValueString(),
			Port:             postgres.Port.ValueInt64(),
			Database:         postgres.Database.ValueString(),
			TableName:        postgres.TableName.ValueString(),
			Username:         postgres.Username.ValueString(),
			Password:         postgres.Password.ValueString(),
			SSLMode:          postgres.SSLMode.ValueString(),
			MaxRetry:         postgres.MaxRetry.ValueInt64(),
			RetryIntervalSec: postgres.RetryIntervalSec.ValueInt64(),
		}
	case !model.Kafka.IsNull():
		var kafka kafkaDestinationModel
		diags.Append(model.Kafka.As(ctx, &kafka, objectAsOptions)...)
		destination.Kafka = &client.KafkaDestination{
			BootstrapServers:  kafka.BootstrapServers.ValueString(),
			TopicName:         kafka.TopicName.ValueString(),
			Username:          knownString(kafka.Username),
			Password:          knownString(kafka.Password),
			Mechanisms:        knownString(kafka.Mechanisms),
			Protocol:          kafka.Protocol.ValueString(),
			CompressionType:   kafka.Compression.ValueString(),
			BatchSize:         kafka.BatchSize.ValueInt64(),
			LingerMs:          kafka.LingerMs.ValueInt64(),
			MaxMessageBytes:   kafka.MaxMessageBytes.ValueInt64(),
			TimeoutSec:        kafka.TimeoutSec.ValueInt64(),
			SSLCAPEM:          knownString(kafka.SSLCAPEM),
			SSLCertificatePEM: knownString(kafka.SSLCertificatePEM),
			SSLKeyPEM:         knownString(kafka.SSLKeyPEM),
			MaxRetry:          kafka.MaxRetry.ValueInt64(),
			RetryIntervalSec:  kafka.RetryIntervalSec.ValueInt64(),
		}
	default:
		diags.AddError("Stream destination has no type", "Set exactly one of `webhook`, `s3`, `azure`, `postgres` or `kafka`.")
	}
	return destination, diags
}

func destinationObject(destination client.StreamDestination) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := map[string]attr.Value{}
	for name, attrType := range streamDestinationAttrTypes {
		values[name] = types.ObjectNull(attrType.(types.ObjectType).AttrTypes)
	}

	var child types.Object
	var childDiags diag.Diagnostics
	switch {
	case destination.Webhook != nil:
		webhook := destination.Webhook
		headers := types.MapNull(types.StringType)
		if len(webhook.Headers) > 0 {
			headers, childDiags = types.MapValueFrom(context.Background(), types.StringType, map[string]string(webhook.Headers))
			diags.Append(childDiags...)
		}
		child, childDiags = types.ObjectValue(webhookDestinationAttrTypes, map[string]attr.Value{
			"url":                types.StringValue(webhook.URL),
			"compression":        types.StringValue(webhook.Compression),
			"headers":            headers,
			"security_token":     stringOrNull(webhook.SecurityToken),
			"max_retry":          types.Int64Value(webhook.MaxRetry),
			"retry_interval_sec": types.Int64Value(webhook.RetryIntervalSec),
			"post_timeout_sec":   types.Int64Value(webhook.PostTimeoutSec),
		})
	case destination.S3 != nil:
		s3 := destination.S3
		child, childDiags = types.ObjectValue(s3DestinationAttrTypes, map[string]attr.Value{
			"endpoint":           types.StringValue(s3.Endpoint),
			"bucket":             types.StringValue(s3.Bucket),
			"region":             stringOrNull(s3.Region),
			"object_prefix":      types.StringValue(s3.ObjectPrefix),
			"file_type":          types.StringValue(s3.FileType),
			"compression":        types.StringValue(s3.FileCompressionType),
			"use_ssl":            types.BoolValue(s3.UseSSL),
			"access_key":         types.StringValue(s3.AccessKey),
			"secret_key":         types.StringValue(s3.SecretKey),
			"max_retry":          types.Int64Value(s3.MaxRetry),
			"retry_interval_sec": types.Int64Value(s3.RetryIntervalSec),
		})
	case destination.Azure != nil:
		azure := destination.Azure
		child, childDiags = types.ObjectValue(azureDestinationAttrTypes, map[string]attr.Value{
			"storage_account":    types.StringValue(azure.StorageAccount),
			"container":          types.StringValue(azure.Container),
			"blob_prefix":        types.StringValue(azure.BlobPrefix),
			"file_type":          types.StringValue(azure.FileType),
			"compression":        types.StringValue(azure.FileCompressionType),
			"sas_token":          types.StringValue(azure.SASToken),
			"max_retry":          types.Int64Value(azure.MaxRetry),
			"retry_interval_sec": types.Int64Value(azure.RetryIntervalSec),
		})
	case destination.Postgres != nil:
		postgres := destination.Postgres
		child, childDiags = types.ObjectValue(postgresDestinationAttrTypes, map[string]attr.Value{
			"host":               types.StringValue(postgres.Host),
			"port":               types.Int64Value(postgres.Port),
			"database":           types.StringValue(postgres.Database),
			"table_name":         types.StringValue(postgres.TableName),
			"username":           types.StringValue(postgres.Username),
			"password":           types.StringValue(postgres.Password),
			"sslmode":            types.StringValue(postgres.SSLMode),
			"max_retry":          types.Int64Value(postgres.MaxRetry),
			"retry_interval_sec": types.Int64Value(postgres.RetryIntervalSec),
		})
	case destination.Kafka != nil:
		kafka := destination.Kafka
		child, childDiags = types.ObjectValue(kafkaDestinationAttrTypes, map[string]attr.Value{
			"bootstrap_servers":   types.StringValue(kafka.BootstrapServers),
			"topic_name":          types.StringValue(kafka.TopicName),
			"username":            stringOrNull(kafka.Username),
			"password":            stringOrNull(kafka.Password),
			"mechanisms":          stringOrNull(kafka.Mechanisms),
			"protocol":            types.StringValue(kafka.Protocol),
			"compression":         types.StringValue(kafka.CompressionType),
			"batch_size":          types.Int64Value(kafka.BatchSize),
			"linger_ms":           types.Int64Value(kafka.LingerMs),
			"max_message_bytes":   types.Int64Value(kafka.MaxMessageBytes),
			"timeout_sec":         types.Int64Value(kafka.TimeoutSec),
			"ssl_ca_pem":          stringOrNull(kafka.SSLCAPEM),
			"ssl_certificate_pem": stringOrNull(kafka.SSLCertificatePEM),
			"ssl_key_pem":         stringOrNull(kafka.SSLKeyPEM),
			"max_retry":           types.Int64Value(kafka.MaxRetry),
			"retry_interval_sec":  types.Int64Value(kafka.RetryIntervalSec),
		})
	default:
		diags.AddError(
			"Unsupported stream destination",
			fmt.Sprintf("The stream delivers to a %q destination, which the provider cannot manage. Change it in the dashboard to one of webhook, s3, azure, postgres or kafka.", destination.Type()),
		)
		return types.ObjectNull(streamDestinationAttrTypes), diags
	}
	diags.Append(childDiags...)
	values[destination.Type()] = child

	object, objectDiags := types.ObjectValue(streamDestinationAttrTypes, values)
	diags.Append(objectDiags...)
	return object, diags
}

func destinationsFromList(ctx context.Context, list types.List) ([]client.StreamDestination, diag.Diagnostics) {
	var diags diag.Diagnostics
	destinations := []client.StreamDestination{}
	if list.IsNull() || list.IsUnknown() {
		return destinations, diags
	}
	var objects []types.Object
	diags.Append(list.ElementsAs(ctx, &objects, false)...)
	for _, object := range objects {
		destination, destinationDiags := destinationFromObject(ctx, object)
		diags.Append(destinationDiags...)
		destinations = append(destinations, destination)
	}
	return destinations, diags
}

// destinationsList reports no extra destinations as null, so a configuration
// that omits the attribute sees no difference.
func destinationsList(destinations []client.StreamDestination) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(destinations) == 0 {
		return types.ListNull(streamDestinationObjectType), diags
	}
	objects := make([]attr.Value, 0, len(destinations))
	for _, destination := range destinations {
		object, objectDiags := destinationObject(destination)
		diags.Append(objectDiags...)
		objects = append(objects, object)
	}
	list, listDiags := types.ListValue(streamDestinationObjectType, objects)
	diags.Append(listDiags...)
	return list, diags
}

// destinationTypeCount counts the types set on one destination, so
// ValidateConfig can require exactly one. known is false while any type is
// still unknown, such as one picked by a conditional on a variable.
func destinationTypeCount(ctx context.Context, object types.Object) (count int, known bool, diags diag.Diagnostics) {
	var model streamDestinationModel
	diags = object.As(ctx, &model, basetypes.ObjectAsOptions{UnhandledUnknownAsEmpty: true})
	for _, child := range []types.Object{model.Webhook, model.S3, model.Azure, model.Postgres, model.Kafka} {
		if child.IsUnknown() {
			return 0, false, diags
		}
		if !child.IsNull() {
			count++
		}
	}
	return count, true, diags
}
