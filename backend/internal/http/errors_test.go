package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
)

func TestErrorEnvelopeShape(t *testing.T) {
	err := NewError(apperr.CodeNotFound, "")
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
	err := NewError(apperr.CodeValidationFailed, "").WithDetails("body.score: 必须介于 1 与 5 之间")
	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("序列化错误对象失败: %v", marshalErr)
	}

	var decoded ErrorEnvelope
	if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
		t.Fatalf("反序列化错误对象失败: %v", unmarshalErr)
	}
	if decoded.Error.Code != string(apperr.CodeValidationFailed) {
		t.Fatalf("错误码 = %s，期望 %s", decoded.Error.Code, apperr.CodeValidationFailed)
	}
	if len(decoded.Error.Details) != 1 {
		t.Fatalf("details 数量 = %d，期望 1", len(decoded.Error.Details))
	}
}

func TestErrorContentTypeStaysJSON(t *testing.T) {
	err := NewError(apperr.CodeNotFound, "")
	var filter huma.ContentTypeFilter = err
	if got := filter.ContentType("application/json"); got != "application/json" {
		t.Fatalf("ContentType = %q，期望 application/json", got)
	}
}

func TestFromAppErrorPreservesCodeStatusAndDetails(t *testing.T) {
	source := apperr.New(apperr.CodeQuotaExceeded, "题库数量已达上限").WithDetails("普通用户上限 1 个")
	converted := FromAppError(source)

	if converted.ErrorEnvelope.Error.Code != string(apperr.CodeQuotaExceeded) {
		t.Fatalf("错误码 = %s，期望 %s", converted.ErrorEnvelope.Error.Code, apperr.CodeQuotaExceeded)
	}
	if converted.GetStatus() != http.StatusConflict {
		t.Fatalf("状态码 = %d，期望 409", converted.GetStatus())
	}
	if len(converted.ErrorEnvelope.Error.Details) != 1 {
		t.Fatalf("details 数量 = %d，期望 1", len(converted.ErrorEnvelope.Error.Details))
	}
}

func TestTranslateConvertsBusinessErrorsOnly(t *testing.T) {
	business := apperr.New(apperr.CodeEmailTaken, "")
	translated := Translate(business)
	var apiErr *APIError
	if !errors.As(translated, &apiErr) {
		t.Fatal("业务错误应被转换为 APIError")
	}
	if apiErr.GetStatus() != http.StatusConflict {
		t.Fatalf("状态码 = %d，期望 409", apiErr.GetStatus())
	}

	plain := errors.New("数据库连接失败")
	if Translate(plain) != plain {
		t.Fatal("非业务错误应原样透传")
	}
}

func TestErrorCodeTableIsComplete(t *testing.T) {
	// 每个错误码都必须有状态码与默认文案，避免出现空响应。
	for _, code := range apperr.AllCodes() {
		status := apperr.StatusFor(code)
		if status < 400 || status > 599 {
			t.Errorf("错误码 %s 的状态码 %d 不在 4xx/5xx 区间", code, status)
		}
		if apperr.DefaultMessage(code) == "" {
			t.Errorf("错误码 %s 缺少默认文案", code)
		}
	}

	seen := map[int]bool{}
	for _, code := range apperr.AllCodes() {
		seen[apperr.StatusFor(code)] = true
	}
	for _, status := range []int{400, 401, 403, 404, 409, 422, 429, 500, 502, 503} {
		if !seen[status] {
			t.Errorf("错误码表未覆盖状态码 %d", status)
		}
	}
}

func TestInstallErrorModelOverridesFrameworkErrors(t *testing.T) {
	InstallErrorModel()

	cases := []struct {
		status int
		code   string
	}{
		{http.StatusNotFound, string(apperr.CodeNotFound)},
		{http.StatusBadRequest, string(apperr.CodeBadRequest)},
		{http.StatusUnauthorized, string(apperr.CodeUnauthorized)},
		{http.StatusTooManyRequests, string(apperr.CodeRateLimited)},
		{http.StatusInternalServerError, string(apperr.CodeInternal)},
		{http.StatusUnprocessableEntity, string(apperr.CodeValidationFailed)},
		{http.StatusBadGateway, string(apperr.CodeBadGateway)},
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
	apperr.New(apperr.Code("not_registered"), "")
}
