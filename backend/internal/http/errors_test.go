package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestErrorEnvelopeShape(t *testing.T) {
	err := NewError(CodeNotFound, "")
	if err.GetStatus() != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 %d", err.GetStatus(), http.StatusNotFound)
	}

	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("序列化错误对象失败: %v", marshalErr)
	}
	const want = `{"error":{"code":"not_found","message":"对象不存在"}}`
	if string(raw) != want {
		t.Fatalf("错误线格式 = %s，期望 %s", raw, want)
	}
}

func TestErrorEnvelopeDetails(t *testing.T) {
	err := NewError(CodeValidationFailed, "").WithDetails("body.score: 必须介于 1 与 5 之间")
	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("序列化错误对象失败: %v", marshalErr)
	}

	var decoded ErrorEnvelope
	if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
		t.Fatalf("反序列化错误对象失败: %v", unmarshalErr)
	}
	if decoded.Error.Code != string(CodeValidationFailed) {
		t.Fatalf("错误码 = %s，期望 %s", decoded.Error.Code, CodeValidationFailed)
	}
	if len(decoded.Error.Details) != 1 {
		t.Fatalf("details 数量 = %d，期望 1", len(decoded.Error.Details))
	}
}

func TestErrorContentTypeStaysJSON(t *testing.T) {
	err := NewError(CodeNotFound, "")
	var filter huma.ContentTypeFilter = err
	if got := filter.ContentType("application/json"); got != "application/json" {
		t.Fatalf("ContentType = %q，期望 application/json", got)
	}
}

func TestErrorCodeTableIsComplete(t *testing.T) {
	// 每个错误码都必须有状态码与默认文案，避免出现空响应。
	for code, spec := range codes {
		if spec.status < 400 || spec.status > 599 {
			t.Errorf("错误码 %s 的状态码 %d 不在 4xx/5xx 区间", code, spec.status)
		}
		if spec.message == "" {
			t.Errorf("错误码 %s 缺少默认文案", code)
		}
	}
	// 除故意保留的 CodeUnprocessableEntity 兜底码外，其余码不应共享同一状态码与文案的组合。
	seen := map[int]int{}
	for _, spec := range codes {
		seen[spec.status]++
	}
	if len(seen) < 10 {
		t.Fatalf("状态码种类 = %d，期望覆盖 400/401/403/404/409/422/429/500/502/503 等", len(seen))
	}
}

func TestInstallErrorModelOverridesFrameworkErrors(t *testing.T) {
	InstallErrorModel()

	cases := []struct {
		status int
		code   string
	}{
		{http.StatusNotFound, string(CodeNotFound)},
		{http.StatusBadRequest, string(CodeBadRequest)},
		{http.StatusUnauthorized, string(CodeUnauthorized)},
		{http.StatusTooManyRequests, string(CodeRateLimited)},
		{http.StatusInternalServerError, string(CodeInternal)},
		{http.StatusUnprocessableEntity, string(CodeValidationFailed)},
		{http.StatusBadGateway, string(CodeBadGateway)},
	}
	for _, tc := range cases {
		got := huma.NewError(tc.status, "framework message")
		var apiErr *APIError
		if !errors.As(got, &apiErr) {
			t.Fatalf("状态码 %d 返回的错误不是 *APIError", tc.status)
		}
		if apiErr.ErrorEnvelope.Error.Code != tc.code {
			t.Errorf("状态码 %d 的错误码 = %s，期望 %s", tc.status, apiErr.ErrorEnvelope.Error.Code, tc.code)
		}
		if apiErr.GetStatus() != tc.status {
			t.Errorf("状态码 %d 往返后变为 %d", tc.status, apiErr.GetStatus())
		}
	}
}

func TestInstallErrorModelKeepsValidationDetails(t *testing.T) {
	InstallErrorModel()

	got := huma.NewError(http.StatusUnprocessableEntity, "validation failed",
		errors.New("body.score: expected integer"))
	var apiErr *APIError
	if !errors.As(got, &apiErr) {
		t.Fatal("错误不是 *APIError")
	}
	if len(apiErr.ErrorEnvelope.Error.Details) != 1 {
		t.Fatalf("details = %v，期望包含一条字段级说明", apiErr.ErrorEnvelope.Error.Details)
	}
}

func TestNewErrorPanicsOnUnknownCode(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("未登记的错误码应当 panic")
		}
	}()
	NewError(Code("not_registered"), "")
}
