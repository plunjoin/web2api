package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"web2api/internal/model"
)

func TestWriteErrorRedactsSensitiveFragments(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadGateway,
		"账号 alice@example.com，密钥 sk-live_123456，头 Bearer abc.def.ghi 均不可用",
		"api_error", nil)

	var got model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := "账号 [REDACTED_EMAIL]，密钥 [REDACTED_API_KEY]，头 Bearer [REDACTED_BEARER_TOKEN] 均不可用"
	if got.Error.Message != want {
		t.Fatalf("错误响应未脱敏: got %q, want %q", got.Error.Message, want)
	}
}
