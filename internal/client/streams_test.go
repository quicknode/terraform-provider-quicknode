package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// streamBody is a live GET response with the secrets replaced. The primary
// destination carries the server's version marker, the extra one does not.
const streamBody = `{"id":"aca58bf5","name":"tftrial","status":"paused","created_at":"2026-09-28T22:31:43Z","current_hash":"",
"dataset":"block","dataset_batch_size":1,"destination":"webhook",
"destination_attributes":{"compression":"none","headers":{"X-Probe":"1","X-Count":2},"max_retry":3,"post_timeout_sec":10,"retry_interval_sec":1,"security_token":"token","url":"https://example.com/a","version":"v5"},
"end_range":-1,"filter_function":"ZnVuY3Rpb24gbWFpbihzdHJlYW0pIHsgcmV0dXJuIHN0cmVhbTsgfQ==","filter_language":"javascript",
"fix_block_reorgs":0,"keep_distance_from_tip":0,"network":"ethereum-sepolia","notification_email":"","region":"usa_east",
"sequence":0,"start_range":11803526,"updated_at":"2026-09-28T22:31:43Z","elastic_batch_enabled":false,"restream_batch_on_reorg":false,
"extra_destinations":[{"destination":"webhook","destination_attributes":{"compression":"none","headers":{},"max_retry":1,"post_timeout_sec":10,"retry_interval_sec":1,"url":"https://example.com/b"}}]}`

type recordedRequest struct {
	method string
	path   string
	body   map[string]any
}

func newRecordingClient(t *testing.T, status int, response string) (*Client, *recordedRequest) {
	t.Helper()

	recorded := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &recorded.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return quicknode, recorded
}

func TestGetStreamDecodesFilterAndDestinations(t *testing.T) {
	quicknode, _ := newRecordingClient(t, http.StatusOK, streamBody)

	stream, err := quicknode.GetStream(context.Background(), "aca58bf5")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}

	if stream.FilterFunction != "function main(stream) { return stream; }" {
		t.Errorf("filter was not decoded: %q", stream.FilterFunction)
	}
	if stream.StartRange != 11803526 || stream.EndRange != StreamRangeUnset {
		t.Errorf("ranges = %d..%d", stream.StartRange, stream.EndRange)
	}

	webhook := stream.Destination.Webhook
	if webhook == nil {
		t.Fatalf("primary destination type = %q, want webhook", stream.Destination.Type())
	}
	if webhook.URL != "https://example.com/a" || webhook.SecurityToken != "token" {
		t.Errorf("webhook = %+v", *webhook)
	}
	if webhook.Headers["X-Count"] != "2" {
		t.Errorf("a numeric header value was not kept as a string: %v", webhook.Headers)
	}

	if len(stream.ExtraDestinations) != 1 || stream.ExtraDestinations[0].Webhook == nil {
		t.Fatalf("extra destinations = %+v", stream.ExtraDestinations)
	}
	if stream.ExtraDestinations[0].Webhook.URL != "https://example.com/b" {
		t.Errorf("extra webhook = %+v", *stream.ExtraDestinations[0].Webhook)
	}
}

func TestGetStreamReportsUnmodeledDestination(t *testing.T) {
	body := `{"id":"1","destination":"snowflake","destination_attributes":{"account":"x"},"extra_destinations":null}`
	quicknode, _ := newRecordingClient(t, http.StatusOK, body)

	stream, err := quicknode.GetStream(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if stream.Destination.Type() != "snowflake" {
		t.Errorf("type = %q, want snowflake", stream.Destination.Type())
	}
	if stream.ExtraDestinations == nil {
		t.Error("a null extra_destinations should decode as an empty list")
	}
}

func TestCreateStreamEncodesFilterAndDestination(t *testing.T) {
	quicknode, recorded := newRecordingClient(t, http.StatusCreated, streamBody)

	endRange := int64(100)
	_, err := quicknode.CreateStream(context.Background(), StreamInput{
		Name:             "tftrial",
		Network:          "ethereum-sepolia",
		Dataset:          "block",
		Region:           "usa_east",
		Status:           StreamStatusPaused,
		FilterFunction:   "function main(stream) { return stream; }",
		EndRange:         &endRange,
		DatasetBatchSize: 1,
		Destination: StreamDestination{Webhook: &WebhookDestination{
			URL: "https://example.com/a", Compression: "none", MaxRetry: 3, RetryIntervalSec: 1, PostTimeoutSec: 10,
		}},
		ExtraDestinations: []StreamDestination{{S3: &S3Destination{Bucket: "archive"}}},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if recorded.method != http.MethodPost || recorded.path != "/streams/rest/v1/streams" {
		t.Errorf("request = %s %s", recorded.method, recorded.path)
	}
	body := recorded.body
	if body["filter_function"] != "ZnVuY3Rpb24gbWFpbihzdHJlYW0pIHsgcmV0dXJuIHN0cmVhbTsgfQ==" {
		t.Errorf("filter_function = %v", body["filter_function"])
	}
	if _, sent := body["start_range"]; sent {
		t.Error("an unset start_range must be omitted so the server picks the latest block")
	}
	if body["end_range"] != float64(100) {
		t.Errorf("end_range = %v", body["end_range"])
	}
	if body["destination"] != "webhook" {
		t.Errorf("destination = %v", body["destination"])
	}

	attributes := body["destination_attributes"].(map[string]any)
	if _, sent := attributes["security_token"]; sent {
		t.Error("an unset security_token must be omitted so the server generates one")
	}
	if headers, ok := attributes["headers"].(map[string]any); !ok || len(headers) != 0 {
		t.Errorf("headers should be sent as an empty object, got %v", attributes["headers"])
	}

	extras := body["extra_destinations"].([]any)
	extra := extras[0].(map[string]any)
	if extra["destination"] != "s3" {
		t.Errorf("extra destination = %v", extra["destination"])
	}
}

func TestUpdateStreamSendsOnlySetFields(t *testing.T) {
	quicknode, recorded := newRecordingClient(t, http.StatusOK, streamBody)

	name := "renamed"
	noExtras := []StreamDestination{}
	_, err := quicknode.UpdateStream(context.Background(), "aca58bf5", StreamUpdate{
		Name:              &name,
		ExtraDestinations: &noExtras,
	})
	if err != nil {
		t.Fatalf("UpdateStream: %v", err)
	}

	if recorded.method != http.MethodPatch || recorded.path != "/streams/rest/v1/streams/aca58bf5" {
		t.Errorf("request = %s %s", recorded.method, recorded.path)
	}
	if len(recorded.body) != 2 {
		t.Errorf("body should carry only name and extra_destinations, got %v", recorded.body)
	}
	if extras, ok := recorded.body["extra_destinations"].([]any); !ok || len(extras) != 0 {
		t.Errorf("clearing extra destinations should send an empty list, got %v", recorded.body["extra_destinations"])
	}
}

func TestStreamErrorsCarryAPIMessage(t *testing.T) {
	quicknode, _ := newRecordingClient(t, http.StatusNotFound,
		`{"statusCode":404,"timestamp":"2026-09-28T00:00:00Z","path":"/streams/rest/v1/streams/x","message":"STREAM_NOT_FOUND"}`)

	_, err := quicknode.GetStream(context.Background(), "x")
	if !IsNotFound(err) {
		t.Fatalf("want a not-found error, got %v", err)
	}

	quicknode, _ = newRecordingClient(t, http.StatusBadRequest,
		`{"statusCode":400,"message":["name should not be empty","region must be one of the following values"]}`)

	_, err = quicknode.GetStream(context.Background(), "x")
	var apiErr *Error
	if err == nil || !errors.As(err, &apiErr) {
		t.Fatalf("want an API error, got %v", err)
	}
	if apiErr.Message != "name should not be empty; region must be one of the following values" {
		t.Errorf("message = %q", apiErr.Message)
	}
}
