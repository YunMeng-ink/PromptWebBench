// Package apperr 定义全系统的错误码与业务错误类型。
// 错误码是前后端契约的一部分：数据库、服务与接口层都只使用这里的取值，
// 前端按 error.<code> 做文案覆盖。
package apperr

import "fmt"

// Code 是错误响应中的稳定标识符（C-41）。
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
	CodeInvalidOAuthState         Code = "oauth_state_invalid"
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

// 以下错误码不由业务代码抛出，只用于描述框架自身产生的响应，
// 使「code 与 HTTP 状态码」的映射保持单值。
const (
	CodeConflict              Code = "conflict"
	CodeMethodNotAllowed      Code = "method_not_allowed"
	CodeNotAcceptable         Code = "not_acceptable"
	CodeRequestEntityTooLarge Code = "request_entity_too_large"
	CodeUnsupportedMediaType  Code = "unsupported_media_type"
	CodeServiceUnavailable    Code = "service_unavailable"
	CodeUnprocessableEntity   Code = "unprocessable_entity"
)

type spec struct {
	status  int
	message string
}

// specs 是错误码表的唯一取值依据：状态码与默认文案都只在这里声明。
var specs = map[Code]spec{
	CodeNotFound:                  {404, "对象不存在"},
	CodeUnauthorized:              {401, "未登录或会话已失效"},
	CodeForbidden:                 {403, "权限不足"},
	CodeBanned:                    {403, "账户已被封禁"},
	CodeInvalidCSRF:               {403, "CSRF 校验未通过"},
	CodeRateLimited:               {429, "请求过于频繁"},
	CodeBadRequest:                {400, "请求无法解析"},
	CodeValidationFailed:          {422, "请求字段校验未通过"},
	CodeInvalidState:              {422, "当前状态不允许该操作"},
	CodeInvalidCategory:           {422, "类别、能力或难度不合法"},
	CodeInvalidCredentials:        {401, "邮箱或密码错误"},
	CodeEmailTaken:                {409, "邮箱已注册"},
	CodeEmailNotVerified:          {403, "邮箱未验证"},
	CodeAlreadyVerified:           {409, "邮箱已验证"},
	CodeInvalidCode:               {422, "验证码错误或已过期"},
	CodeInvalidOAuthState:         {422, "OAuth 登录请求已过期，请重新发起"},
	CodeInvalidInvite:             {422, "邀请码无效、已用完或已禁用"},
	CodeRegistrationClosed:        {403, "注册已关闭"},
	CodeEmailNotAllowed:           {422, "邮箱后缀不在白名单内"},
	CodeMailSendFailed:            {422, "验证码邮件发送失败"},
	CodeMailError:                 {422, "测试邮件发送失败"},
	CodeOAuthNotConfigured:        {503, "未配置 GitHub OAuth"},
	CodeQuotaExceeded:             {409, "超出配额"},
	CodeRatingExists:              {409, "已对该提示词与模型组合评过分"},
	CodeRatingEditLimitExceeded:   {409, "评分修改次数已超限"},
	CodeRatingDeleteLimitExceeded: {409, "评分删除次数已超限"},
	CodeDuplicateModel:            {409, "模型重复"},
	CodeModelInUse:                {409, "模型已被引用，不可删除"},
	CodePromptInUse:               {409, "提示词已被题库引用，不可彻底删除"},
	CodePermissionDenied:          {403, "权限不足"},
	CodeSecretUnavailable:         {422, "未配置主密钥，无法保存密钥类配置"},
	CodeBadGateway:                {502, "上游服务请求失败"},
	CodeInternal:                  {500, "服务器内部错误"},

	CodeConflict:              {409, "请求与当前资源状态冲突"},
	CodeMethodNotAllowed:      {405, "请求方法不被允许"},
	CodeNotAcceptable:         {406, "无法满足请求的响应格式"},
	CodeRequestEntityTooLarge: {413, "请求体超出大小上限"},
	CodeUnsupportedMediaType:  {415, "不支持的请求体类型"},
	CodeServiceUnavailable:    {503, "服务暂不可用"},
	CodeUnprocessableEntity:   {422, "请求无法处理"},
}

// Error 是承载错误码的业务错误。
type Error struct {
	Code    Code
	Message string
	Details []string

	status int
}

// New 按错误码构造错误；message 为空时取码表中的默认文案。
func New(code Code, message string) *Error {
	sp, ok := specs[code]
	if !ok {
		panic(fmt.Errorf("未登记的错误码 %q", code))
	}
	if message == "" {
		message = sp.message
	}
	return &Error{Code: code, Message: message, status: sp.status}
}

func Errorf(code Code, format string, args ...any) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

func (e *Error) Error() string { return e.Message }

// Status 返回该错误码对应的 HTTP 状态码。
func (e *Error) Status() int { return e.status }

// WithDetails 追加字段级说明。
func (e *Error) WithDetails(details ...string) *Error {
	e.Details = append(e.Details, details...)
	return e
}

// StatusFor 返回错误码对应的 HTTP 状态码。
func StatusFor(code Code) int {
	sp, ok := specs[code]
	if !ok {
		panic(fmt.Errorf("未登记的错误码 %q", code))
	}
	return sp.status
}

// DefaultMessage 返回错误码的默认文案。
func DefaultMessage(code Code) string {
	sp, ok := specs[code]
	if !ok {
		panic(fmt.Errorf("未登记的错误码 %q", code))
	}
	return sp.message
}

// AllCodes 返回全部已登记的错误码。
func AllCodes() []Code {
	out := make([]Code, 0, len(specs))
	for code := range specs {
		out = append(out, code)
	}
	return out
}
