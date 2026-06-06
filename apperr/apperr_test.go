package apperr

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kusold/grove/httpx"
)

func TestError_Error(t *testing.T) {
	t.Run("formats code and message without cause", func(t *testing.T) {
		e := &Error{Code: "test_code", Message: "test message"}
		want := "grove: test_code: test message"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("includes cause when present", func(t *testing.T) {
		cause := errors.New("underlying failure")
		e := &Error{Code: "test_code", Message: "test message", Cause: cause}
		got := e.Error()
		if !strings.Contains(got, "underlying failure") {
			t.Errorf("Error() = %q, want to contain 'underlying failure'", got)
		}
		if !strings.Contains(got, "grove:") {
			t.Errorf("Error() = %q, want to contain 'grove:' prefix", got)
		}
	})
}

func TestError_Unwrap(t *testing.T) {
	t.Run("returns nil when no cause", func(t *testing.T) {
		e := &Error{Code: "test", Message: "msg"}
		if got := e.Unwrap(); got != nil {
			t.Errorf("Unwrap() = %v, want nil", got)
		}
	})

	t.Run("returns cause for errors.Is chaining", func(t *testing.T) {
		cause := errors.New("root cause")
		e := &Error{Code: "test", Message: "msg", Cause: cause}
		if !errors.Is(e, cause) {
			t.Error("expected errors.Is to find the cause")
		}
	})
}

func TestWriteError(t *testing.T) {
	t.Run("writes JSON error response", func(t *testing.T) {
		appErr := &Error{
			Code:       "test_error",
			Message:    "something went wrong",
			StatusCode: http.StatusBadRequest,
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		WriteError(rec, req, appErr)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj, ok := body["error"].(map[string]any)
		if !ok {
			t.Fatal("expected 'error' object in response")
		}
		if errObj["code"] != "test_error" {
			t.Errorf("code = %v, want %q", errObj["code"], "test_error")
		}
		if errObj["message"] != "something went wrong" {
			t.Errorf("message = %v, want %q", errObj["message"], "something went wrong")
		}
	})

	t.Run("includes request_id when present in context", func(t *testing.T) {
		appErr := &Error{
			Code:       "test_error",
			Message:    "msg",
			StatusCode: http.StatusBadRequest,
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := httpx.WithRequestID(req.Context(), "req-xyz-789")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		WriteError(rec, req, appErr)

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj, ok := body["error"].(map[string]any)
		if !ok {
			t.Fatal("expected 'error' object in response")
		}
		if errObj["request_id"] != "req-xyz-789" {
			t.Errorf("request_id = %v, want %q", errObj["request_id"], "req-xyz-789")
		}
	})

	t.Run("omits request_id when not in context", func(t *testing.T) {
		appErr := &Error{
			Code:       "test_error",
			Message:    "msg",
			StatusCode: http.StatusInternalServerError,
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		WriteError(rec, req, appErr)

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj, ok := body["error"].(map[string]any)
		if !ok {
			t.Fatal("expected 'error' object in response")
		}
		if _, exists := errObj["request_id"]; exists {
			t.Error("request_id should be omitted when not present")
		}
	})

	t.Run("never exposes Cause in response", func(t *testing.T) {
		appErr := &Error{
			Code:       "internal_error",
			Message:    "something failed",
			StatusCode: http.StatusInternalServerError,
			Cause:      errors.New("secret database connection string: postgres://admin:pass@host"),
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		WriteError(rec, req, appErr)

		body := rec.Body.String()
		if strings.Contains(body, "secret database") {
			t.Error("response body should not contain internal cause details")
		}
		if strings.Contains(body, "postgres://") {
			t.Error("response body should not contain connection strings")
		}
	})

	t.Run("writes different status codes correctly", func(t *testing.T) {
		cases := []struct {
			name       string
			statusCode int
		}{
			{"400 Bad Request", http.StatusBadRequest},
			{"404 Not Found", http.StatusNotFound},
			{"422 Unprocessable Entity", http.StatusUnprocessableEntity},
			{"500 Internal Server Error", http.StatusInternalServerError},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				appErr := &Error{
					Code:       "test",
					Message:    "msg",
					StatusCode: tc.statusCode,
				}

				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				WriteError(rec, req, appErr)

				if rec.Code != tc.statusCode {
					t.Errorf("status = %d, want %d", rec.Code, tc.statusCode)
				}
			})
		}
	})
}

func TestTenantRequired(t *testing.T) {
	t.Run("returns correct code and message", func(t *testing.T) {
		e := TenantRequired()
		if e.Code != "tenant_required" {
			t.Errorf("Code = %q, want %q", e.Code, "tenant_required")
		}
		if !strings.Contains(e.Message, "tenant is required") {
			t.Errorf("Message = %q, want to contain 'tenant is required'", e.Message)
		}
	})

	t.Run("returns 422 status code", func(t *testing.T) {
		e := TenantRequired()
		if e.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("StatusCode = %d, want %d", e.StatusCode, http.StatusUnprocessableEntity)
		}
	})

	t.Run("has no cause by default", func(t *testing.T) {
		e := TenantRequired()
		if e.Cause != nil {
			t.Errorf("Cause = %v, want nil", e.Cause)
		}
	})

	t.Run("produces correct JSON response", func(t *testing.T) {
		e := TenantRequired()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		WriteError(rec, req, e)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj := body["error"].(map[string]any)
		if errObj["code"] != "tenant_required" {
			t.Errorf("code = %v, want %q", errObj["code"], "tenant_required")
		}
		if errObj["message"] != "tenant is required" {
			t.Errorf("message = %v, want %q", errObj["message"], "tenant is required")
		}
	})
}

func TestInvalidTenant(t *testing.T) {
	t.Run("returns correct code and message", func(t *testing.T) {
		e := InvalidTenant()
		if e.Code != "invalid_tenant" {
			t.Errorf("Code = %q, want %q", e.Code, "invalid_tenant")
		}
		if !strings.Contains(e.Message, "invalid tenant") {
			t.Errorf("Message = %q, want to contain 'invalid tenant'", e.Message)
		}
	})

	t.Run("returns 400 status code", func(t *testing.T) {
		e := InvalidTenant()
		if e.StatusCode != http.StatusBadRequest {
			t.Errorf("StatusCode = %d, want %d", e.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("has no cause by default", func(t *testing.T) {
		e := InvalidTenant()
		if e.Cause != nil {
			t.Errorf("Cause = %v, want nil", e.Cause)
		}
	})

	t.Run("produces correct JSON response", func(t *testing.T) {
		e := InvalidTenant()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		WriteError(rec, req, e)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj := body["error"].(map[string]any)
		if errObj["code"] != "invalid_tenant" {
			t.Errorf("code = %v, want %q", errObj["code"], "invalid_tenant")
		}
	})
}
