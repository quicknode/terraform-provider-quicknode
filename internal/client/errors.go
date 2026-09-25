package client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error carries an Admin API failure. The Admin API can report a failure either
// through the HTTP status or through the "error" field of a 200 response body,
// so both paths produce this type.
type Error struct {
	Operation string
	Status    int
	Message   string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s: unexpected HTTP %d", e.Operation, e.Status)
	}
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Operation, e.Message, e.Status)
}

func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func IsAlreadyExists(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "ALREADY_EXISTS")
}

// IsUnauthorized reports the two ways the Admin API rejects a key: an invalid
// key, and a key on a plan without Admin API access.
func IsUnauthorized(err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden
}

func statusError(operation string, status int, body []byte) error {
	message := ""
	if len(body) > 0 && len(body) < 512 {
		message = string(body)
	}
	return &Error{Operation: operation, Status: status, Message: message}
}

func envelopeError(operation string, status int, apiError *string) error {
	if apiError == nil || *apiError == "" {
		return nil
	}
	return &Error{Operation: operation, Status: status, Message: *apiError}
}
