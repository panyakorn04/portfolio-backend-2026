package response

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// ErrorDetail mirrors the frontend's ApiErrorDetail shape.
type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type okEnvelope struct {
	Ok   bool `json:"ok"`
	Data any  `json:"data"`
}

type errorBody struct {
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
}

type errorEnvelope struct {
	Ok    bool      `json:"ok"`
	Error errorBody `json:"error"`
}

// Ok writes a success envelope: { ok: true, data }.
func Ok(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, okEnvelope{Ok: true, Data: data})
}

// MarshalOk returns a JSON success envelope: { ok: true, data }.
func MarshalOk(data any) ([]byte, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(okEnvelope{Ok: true, Data: data})
	return buf.Bytes(), err
}

// WriteJSONBytes writes a pre-marshaled JSON response body.
func WriteJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Error writes an error envelope: { ok: false, error: { message, details } }.
func Error(w http.ResponseWriter, status int, message string, details ...ErrorDetail) {
	if details == nil {
		details = []ErrorDetail{}
	}
	writeJSON(w, status, errorEnvelope{
		Ok:    false,
		Error: errorBody{Message: message, Details: details},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
