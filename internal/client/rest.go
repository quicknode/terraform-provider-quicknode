package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// restErrorBody is the failure shape of the Streams and Key-Value REST APIs.
// A validation failure reports message as a list of strings rather than one.
type restErrorBody struct {
	Message json.RawMessage `json:"message"`
	Msg     string          `json:"msg"`
}

func (c *Client) doJSON(ctx context.Context, operation, method, path string, body, out any) error {
	var payload io.Reader
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return restError(operation, resp.StatusCode, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("%s: decoding response: %w", operation, err)
	}
	return nil
}

func restError(operation string, status int, body []byte) error {
	var parsed restErrorBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return statusError(operation, status, body)
	}

	message := parsed.Msg
	var single string
	var many []string
	switch {
	case json.Unmarshal(parsed.Message, &single) == nil && single != "":
		message = single
	case json.Unmarshal(parsed.Message, &many) == nil && len(many) > 0:
		message = strings.Join(many, "; ")
	}
	if message == "" {
		return statusError(operation, status, body)
	}
	return &Error{Operation: operation, Status: status, Message: message}
}
