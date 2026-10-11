package api

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
)

// ErrorBody 是错误对象本体。
type ErrorBody struct {
	Code    string   `json:"code" doc:"稳定的 snake_case 错误码"`
	Message string   `json:"message" doc:"当前语言的错误文案"`
	Details []string `json:"details,omitempty" doc:"字段级错误说明"`
}

// ErrorEnvelope 是全部错误响应的线格式：{"error":{"code","message"}}。
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// APIError 实现 huma.StatusError。Huma 在 handler 返回实现了该接口的错误时
// 直接以本对象作为响应体写出，因此错误形状完全由本类型决定。
type APIError struct {
	ErrorEnvelope
	status int
}

func (e *APIError) Error() string { return e.ErrorEnvelope.Error.Message }

func (e *APIError) GetStatus() int { return e.status }

// ContentType 保持不变，使错误响应仍为 application/json 而非 RFC 9457 的 problem+json。
func (e *APIError) ContentType(ct string) string { return ct }

// WithDetails 追加字段级错误说明。
func (e *APIError) WithDetails(details ...string) *APIError {
	e.ErrorEnvelope.Error.Details = append(e.ErrorEnvelope.Error.Details, details...)
	return e
}

// FromAppError 把业务错误转换为线格式。
func FromAppError(err *apperr.Error) *APIError {
	return &APIError{
		ErrorEnvelope: ErrorEnvelope{Error: ErrorBody{
			Code:    string(err.Code),
			Message: err.Message,
			Details: err.Details,
		}},
		status: err.Status(),
	}
}

// NewError 按错误码构造响应错误。
func NewError(code apperr.Code, message string) *APIError {
	return FromAppError(apperr.New(code, message))
}

func Errorf(code apperr.Code, format string, args ...any) *APIError {
	return FromAppError(apperr.Errorf(code, format, args...))
}

func NotFound() *APIError         { return NewError(apperr.CodeNotFound, "") }
func Unauthorized() *APIError     { return NewError(apperr.CodeUnauthorized, "") }
func Forbidden() *APIError        { return NewError(apperr.CodeForbidden, "") }
func Internal() *APIError         { return NewError(apperr.CodeInternal, "") }
func QuotaExceeded() *APIError    { return NewError(apperr.CodeQuotaExceeded, "") }
func InvalidState() *APIError     { return NewError(apperr.CodeInvalidState, "") }
func PermissionDenied() *APIError { return NewError(apperr.CodePermissionDenied, "") }

// Translate 把服务层返回的业务错误转换为线格式错误，其他错误原样透传。
// 它是 handler 返回值进入 Huma 写出流程前的唯一收敛点。
func Translate(err error) error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return FromAppError(appErr)
	}
	return err
}

// statusToCode 覆盖框架自身构造错误时会用到的状态码。
var statusToCode = map[int]apperr.Code{
	http.StatusBadRequest:            apperr.CodeBadRequest,
	http.StatusUnauthorized:          apperr.CodeUnauthorized,
	http.StatusForbidden:             apperr.CodeForbidden,
	http.StatusNotFound:              apperr.CodeNotFound,
	http.StatusMethodNotAllowed:      apperr.CodeMethodNotAllowed,
	http.StatusNotAcceptable:         apperr.CodeNotAcceptable,
	http.StatusConflict:              apperr.CodeConflict,
	http.StatusRequestEntityTooLarge: apperr.CodeRequestEntityTooLarge,
	http.StatusUnsupportedMediaType:  apperr.CodeUnsupportedMediaType,
	http.StatusUnprocessableEntity:   apperr.CodeValidationFailed,
	http.StatusTooManyRequests:       apperr.CodeRateLimited,
	http.StatusInternalServerError:   apperr.CodeInternal,
	http.StatusBadGateway:            apperr.CodeBadGateway,
	http.StatusServiceUnavailable:    apperr.CodeServiceUnavailable,
}

// codeForStatus 把状态码映射为错误码；未知状态码按 4xx/5xx 归类。
func codeForStatus(status int) apperr.Code {
	if code, ok := statusToCode[status]; ok {
		return code
	}
	if status >= http.StatusInternalServerError {
		return apperr.CodeInternal
	}
	return apperr.CodeUnprocessableEntity
}

// InstallErrorModel 覆盖 huma.NewError，使框架自身构造的全部错误
// （内置构造函数、形态校验失败、路由未匹配等）同样输出 {error:{code,message}}。
// 必须在创建 API 之前调用一次。
func InstallErrorModel() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		details := make([]string, 0, len(errs))
		for _, err := range errs {
			if err != nil {
				details = append(details, err.Error())
			}
		}
		out := NewError(codeForStatus(status), "")
		if len(details) > 0 {
			out.ErrorEnvelope.Error.Details = details
		}
		return out
	}
}
