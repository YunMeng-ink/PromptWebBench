// Package api 承载 HTTP 接口层：Echo 与 Huma 的装配、统一错误模型、限流与观测。
// 本层只做绑定、校验、调用服务与序列化，不含业务规则。
package api

import (
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Code 是错误响应中的稳定标识符（C-41）。前端按 error.<code> 做文案覆盖。
type Code string

const (
	CodeNotFound                  Code = "not_found"
	CodeUnauthorized              Code = "unauthorized"
	CodeForbidden                 Code = "forbidden"
	CodeBanned                    Code = "banned"
	CodeInvalidCSRF               Code = "invalid_csrf"
	CodeRateLimited               Code = "rate_limited"
	CodeBadRequest                Code = "bad_request"
	CodeValidationFailed          Code = "validation_failed"
	CodeInvalidState              Code = "invalid_state"
	CodeInvalidCategory           Code = "invalid_category"
	CodeInvalidCredentials        Code = "invalid_credentials"
	CodeEmailTaken                Code = "email_taken"
	CodeEmailNotVerified          Code = "email_not_verified"
	CodeAlreadyVerified           Code = "already_verified"
	CodeInvalidCode               Code = "invalid_code"
	CodeInvalidInvite             Code = "invalid_invite"
	CodeRegistrationClosed        Code = "registration_closed"
	CodeEmailNotAllowed           Code = "email_not_allowed"
	CodeMailSendFailed            Code = "mail_send_failed"
	CodeMailError                 Code = "mail_error"
	CodeOAuthNotConfigured        Code = "oauth_not_configured"
	CodeQuotaExceeded             Code = "quota_exceeded"
	CodeRatingExists              Code = "rating_exists"
	CodeRatingEditLimitExceeded   Code = "rating_edit_limit_exceeded"
	CodeRatingDeleteLimitExceeded Code = "rating_delete_limit_exceeded"
	CodeDuplicateModel            Code = "duplicate_model"
	CodeModelInUse                Code = "model_in_use"
	CodePromptInUse               Code = "prompt_in_use"
	CodePermissionDenied          Code = "permission_denied"
	CodeSecretUnavailable         Code = "secret_unavailable"
	CodeBadGateway                Code = "bad_gateway"
	CodeInternal                  Code = "internal"
)

// 以下错误码不由业务代码抛出，只用于描述框架自身产生的响应，使
// 「code ↔ HTTP 状态码」的映射保持单值，不把 409 之类的状态挤进语义不符的码上。
const (
	CodeConflict              Code = "conflict"
	CodeMethodNotAllowed      Code = "method_not_allowed"
	CodeNotAcceptable         Code = "not_acceptable"
	CodeRequestEntityTooLarge Code = "request_entity_too_large"
	CodeUnsupportedMediaType  Code = "unsupported_media_type"
	CodeServiceUnavailable    Code = "service_unavailable"
	CodeUnprocessableEntity   Code = "unprocessable_entity"
)

type codeSpec struct {
	status  int
	message string
}

// codes 是错误码表（开发方案 §5.4）的唯一取值依据。
var codes = map[Code]codeSpec{
	CodeNotFound:                  {http.StatusNotFound, "对象不存在"},
	CodeUnauthorized:              {http.StatusUnauthorized, "未登录或会话已失效"},
	CodeForbidden:                 {http.StatusForbidden, "权限不足"},
	CodeBanned:                    {http.StatusForbidden, "账户已被封禁"},
	CodeInvalidCSRF:               {http.StatusForbidden, "CSRF 校验未通过"},
	CodeRateLimited:               {http.StatusTooManyRequests, "请求过于频繁"},
	CodeBadRequest:                {http.StatusBadRequest, "请求无法解析"},
	CodeValidationFailed:          {http.StatusUnprocessableEntity, "请求字段校验未通过"},
	CodeInvalidState:              {http.StatusUnprocessableEntity, "当前状态不允许该操作"},
	CodeInvalidCategory:           {http.StatusUnprocessableEntity, "类别、能力或难度不合法"},
	CodeInvalidCredentials:        {http.StatusUnauthorized, "邮箱或密码错误"},
	CodeEmailTaken:                {http.StatusConflict, "邮箱已注册"},
	CodeEmailNotVerified:          {http.StatusForbidden, "邮箱未验证"},
	CodeAlreadyVerified:           {http.StatusConflict, "邮箱已验证"},
	CodeInvalidCode:               {http.StatusUnprocessableEntity, "验证码错误或已过期"},
	CodeInvalidInvite:             {http.StatusUnprocessableEntity, "邀请码无效、已用完或已禁用"},
	CodeRegistrationClosed:        {http.StatusForbidden, "注册已关闭"},
	CodeEmailNotAllowed:           {http.StatusUnprocessableEntity, "邮箱后缀不在白名单内"},
	CodeMailSendFailed:            {http.StatusUnprocessableEntity, "验证码邮件发送失败"},
	CodeMailError:                 {http.StatusUnprocessableEntity, "测试邮件发送失败"},
	CodeOAuthNotConfigured:        {http.StatusServiceUnavailable, "未配置 GitHub OAuth"},
	CodeQuotaExceeded:             {http.StatusConflict, "超出配额"},
	CodeRatingExists:              {http.StatusConflict, "已对该提示词与模型组合评过分"},
	CodeRatingEditLimitExceeded:   {http.StatusConflict, "评分修改次数已超限"},
	CodeRatingDeleteLimitExceeded: {http.StatusConflict, "评分删除次数已超限"},
	CodeDuplicateModel:            {http.StatusConflict, "模型重复"},
	CodeModelInUse:                {http.StatusConflict, "模型已被引用，不可删除"},
	CodePromptInUse:               {http.StatusConflict, "提示词已被题库引用，不可彻底删除"},
	CodePermissionDenied:          {http.StatusForbidden, "权限不足"},
	CodeSecretUnavailable:         {http.StatusUnprocessableEntity, "未配置主密钥，无法保存密钥类配置"},
	CodeBadGateway:                {http.StatusBadGateway, "上游服务请求失败"},
	CodeInternal:                  {http.StatusInternalServerError, "服务器内部错误"},

	CodeConflict:              {http.StatusConflict, "请求与当前资源状态冲突"},
	CodeMethodNotAllowed:      {http.StatusMethodNotAllowed, "请求方法不被允许"},
	CodeNotAcceptable:         {http.StatusNotAcceptable, "无法满足请求的响应格式"},
	CodeRequestEntityTooLarge: {http.StatusRequestEntityTooLarge, "请求体超出大小上限"},
	CodeUnsupportedMediaType:  {http.StatusUnsupportedMediaType, "不支持的请求体类型"},
	CodeServiceUnavailable:    {http.StatusServiceUnavailable, "服务暂不可用"},
	CodeUnprocessableEntity:   {http.StatusUnprocessableEntity, "请求无法处理"},
}

// statusToCode 覆盖框架自身构造错误时会用到的状态码。
var statusToCode = map[int]Code{
	http.StatusBadRequest:            CodeBadRequest,
	http.StatusUnauthorized:          CodeUnauthorized,
	http.StatusForbidden:             CodeForbidden,
	http.StatusNotFound:              CodeNotFound,
	http.StatusMethodNotAllowed:      CodeMethodNotAllowed,
	http.StatusNotAcceptable:         CodeNotAcceptable,
	http.StatusConflict:              CodeConflict,
	http.StatusRequestEntityTooLarge: CodeRequestEntityTooLarge,
	http.StatusUnsupportedMediaType:  CodeUnsupportedMediaType,
	http.StatusUnprocessableEntity:   CodeValidationFailed,
	http.StatusTooManyRequests:       CodeRateLimited,
	http.StatusInternalServerError:   CodeInternal,
	http.StatusBadGateway:            CodeBadGateway,
	http.StatusServiceUnavailable:    CodeServiceUnavailable,
}

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

// APIError 实现 huma.StatusError。Huma 在 handler 返回实现了该接口的错误时，
// 直接以本对象作为响应体写出，因此错误形状完全由本类型决定。
type APIError struct {
	ErrorEnvelope
	status int
}

func (e *APIError) Error() string { return e.ErrorEnvelope.Error.Message }

func (e *APIError) GetStatus() int { return e.status }

// ContentType 保持不变，使错误响应仍为 application/json 而非 RFC 9457 的 problem+json。
func (e *APIError) ContentType(ct string) string { return ct }

// NewError 按错误码构造错误，message 为空时取码表默认文案。
func NewError(code Code, message string) *APIError {
	spec, ok := codes[code]
	if !ok {
		panic(fmt.Errorf("未登记的错误码 %q", code))
	}
	if message == "" {
		message = spec.message
	}
	return &APIError{
		ErrorEnvelope: ErrorEnvelope{Error: ErrorBody{Code: string(code), Message: message}},
		status:        spec.status,
	}
}

// WithDetails 追加字段级错误说明。
func (e *APIError) WithDetails(details ...string) *APIError {
	e.ErrorEnvelope.Error.Details = append(e.ErrorEnvelope.Error.Details, details...)
	return e
}

func Errorf(code Code, format string, args ...any) *APIError {
	return NewError(code, fmt.Sprintf(format, args...))
}

func NotFound() *APIError         { return NewError(CodeNotFound, "") }
func Unauthorized() *APIError     { return NewError(CodeUnauthorized, "") }
func Forbidden() *APIError        { return NewError(CodeForbidden, "") }
func Internal() *APIError         { return NewError(CodeInternal, "") }
func ValidationFailed() *APIError { return NewError(CodeValidationFailed, "") }
func BadRequest() *APIError       { return NewError(CodeBadRequest, "") }
func QuotaExceeded() *APIError    { return NewError(CodeQuotaExceeded, "") }
func InvalidState() *APIError     { return NewError(CodeInvalidState, "") }
func PermissionDenied() *APIError { return NewError(CodePermissionDenied, "") }
func EmailNotVerified() *APIError { return NewError(CodeEmailNotVerified, "") }
func Banned() *APIError           { return NewError(CodeBanned, "") }

// InstallErrorModel 覆盖 huma.NewError，使框架自身构造的全部错误
// （内置构造函数、形态校验失败、路由未匹配等）同样输出 {error:{code,message}}。
// 必须在创建 API 之前调用一次。
func InstallErrorModel() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		code, ok := statusToCode[status]
		if !ok {
			if status >= 500 {
				code = CodeInternal
			} else {
				code = CodeUnprocessableEntity
			}
		}
		details := make([]string, 0, len(errs))
		for _, err := range errs {
			if err != nil {
				details = append(details, err.Error())
			}
		}
		out := NewError(code, "")
		if len(details) > 0 {
			out.WithDetails(details...)
		}
		return out
	}
}
