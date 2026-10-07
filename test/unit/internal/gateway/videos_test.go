//go:build web2api_unit

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aistudio "web2api/internal/aistudio2api/aistudio"
	"web2api/internal/model"
)

func TestVideoErrorResponse(t *testing.T) {
	quotaErr := aistudio.DecodeRPCError("GenerateVideo", http.StatusTooManyRequests,
		[]byte(`[8,"You exceeded your current quota, please check your plan and billing details."]`))
	for _, test := range []struct {
		name   string
		err    error
		status int
		kind   string
		code   any
	}{
		{"upstream quota", quotaErr, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded"},
		{"wrapped quota", fmt.Errorf("create video: %w", quotaErr), http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded"},
		{"joined quota", errors.Join(quotaErr, errors.New("lease release failed")), http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded"},
		{"all accounts cooling", &aistudio.AllCoolingError{ModelID: "veo-3.1-fast-generate-preview", Until: time.Now().Add(time.Hour)}, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded"},
		{"invalid options", fmt.Errorf("%w: unsupported resolution", aistudio.ErrInvalidArgument), http.StatusBadRequest, "invalid_request_error", nil},
		{"upstream unavailable", &aistudio.RPCError{Method: "GenerateVideo", StatusCode: http.StatusServiceUnavailable, Message: "unavailable"}, http.StatusBadGateway, "api_error", nil},
		{"transport failure", errors.New("connection reset"), http.StatusBadGateway, "api_error", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeVideoError(rec, test.err)
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d", rec.Code, test.status)
			}
			var response model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error.Type != test.kind || response.Error.Code != test.code {
				t.Fatalf("error = %#v", response.Error)
			}
			if response.Error.Message != test.err.Error() {
				t.Fatalf("upstream error lost: %s", response.Error.Message)
			}
		})
	}
}
