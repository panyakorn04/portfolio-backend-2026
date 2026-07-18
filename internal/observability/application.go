package observability

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
)

// Error records a stable event and error type without serializing arbitrary
// error text, which may contain upstream response bodies, URLs, tokens, or PII.
func Error(ctx context.Context, event, message string, err error, fields ...logx.LogField) {
	base := applicationErrorFields(ctx, event, err)
	logx.WithContext(ctx).Errorw(message, append(base, fields...)...)
}

// ErrorType is retained for call sites that explicitly require type-only
// logging. Error already applies the same fail-closed policy.
func ErrorType(ctx context.Context, event, message string, err error, fields ...logx.LogField) {
	Error(ctx, event, message, err, fields...)
}

func applicationErrorFields(ctx context.Context, event string, err error) []logx.LogField {
	fields := []logx.LogField{logx.Field("event", event)}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		fields = append(fields, logx.Field("request_id", requestID))
	}
	return append(fields, logx.Field("error_type", fmt.Sprintf("%T", err)))
}
