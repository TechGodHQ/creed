// Package httpapi exposes Creed service operations over a generated JSON HTTP surface.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/techgodhq/creed/internal/httpapi/gen"
	opsgen "github.com/techgodhq/creed/internal/ops/gen"
	"github.com/techgodhq/creed/internal/service"
)

// NewHandler returns an HTTP handler backed by the generated service operation surface.
func NewHandler(s service.Service) http.Handler {
	mux := http.NewServeMux()
	operations := gen.GeneratedOperations(s)

	mux.HandleFunc("/v1/operations", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/operations" {
			notFound(w)
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		descriptors := make([]opsgen.OperationDescriptor, 0, len(operations))
		for _, operation := range operations {
			descriptors = append(descriptors, operation.Descriptor)
		}
		writeJSON(w, http.StatusOK, catalogResponse{Operations: descriptors})
	})

	for _, operation := range operations {
		operation := operation
		mux.HandleFunc(operation.Descriptor.HTTPRoute, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != operation.Descriptor.HTTPRoute {
				notFound(w)
				return
			}
			if r.Method != http.MethodPost {
				methodNotAllowed(w, http.MethodPost)
				return
			}
			defer r.Body.Close()
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{OK: false, Error: err.Error()})
				return
			}
			if !json.Valid(defaultPayload(payload)) {
				writeJSON(w, http.StatusBadRequest, errorResponse{OK: false, Error: "invalid JSON payload"})
				return
			}
			result, err := operation.Handler(r.Context(), json.RawMessage(payload))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{OK: false, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, callResponse{OK: true, Result: result})
		})
	}

	return mux
}

type catalogResponse struct {
	Operations []opsgen.OperationDescriptor `json:"operations"`
}

type callResponse struct {
	OK     bool `json:"ok"`
	Result any  `json:"result,omitempty"`
}

type errorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{OK: false, Error: "method not allowed"})
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, errorResponse{OK: false, Error: "operation not found"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func defaultPayload(payload []byte) []byte {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return []byte(`{}`)
	}
	return payload
}

// OperationNameFromPath extracts the generated operation name from a call route.
func OperationNameFromPath(path string) string {
	return strings.TrimPrefix(path, "/v1/operations/")
}
