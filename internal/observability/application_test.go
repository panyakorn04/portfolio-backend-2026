package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestApplicationErrorFieldsIncludeRequestID(t *testing.T) {
	ctx := context.WithValue(context.Background(), requestIDContextKey{}, "request-123")
	fields := applicationErrorFields(ctx, "studio.execution.failed", errors.New("failed"))

	values := make(map[string]any, len(fields))
	for _, field := range fields {
		values[field.Key] = field.Value
	}
	if values["event"] != "studio.execution.failed" {
		t.Fatalf("event = %#v", values["event"])
	}
	if values["request_id"] != "request-123" {
		t.Fatalf("request_id = %#v", values["request_id"])
	}
	if values["error_type"] == nil {
		t.Fatal("error_type field is missing")
	}
	if _, ok := values["error"]; ok {
		t.Fatal("arbitrary error text must not be logged")
	}
}

func TestApplicationErrorFieldsDoNotSerializeUpstreamResponse(t *testing.T) {
	err := errors.New(`ollama returned 500: {"token":"query-secret","visitor":"visitor-123","message":"raw AI content"}`)
	fields := applicationErrorFields(context.Background(), "dependency.failed", err)

	for _, field := range fields {
		value := fmt.Sprint(field.Value)
		for _, secret := range []string{"query-secret", "visitor-123", "raw AI content"} {
			if strings.Contains(value, secret) {
				t.Fatalf("upstream response leaked through field %q: %v", field.Key, field.Value)
			}
		}
	}
}
