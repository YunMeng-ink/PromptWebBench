package social

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
)

// errDepsIncomplete 表示服务构造时缺少必要依赖。
var errDepsIncomplete = errors.New("社交服务依赖不完整：Pool、Config 均不可为空")

// translatePermission 把领域层的权限判定结果翻译为错误码。
//
// 未登录与不可见一律返回 404（开发方案 §5.3）：通过状态码差异探测对象是否存在的
// 路径必须被堵死，因此「无权限」与「不存在」对外不区分。
func translatePermission(err error) error {
	switch {
	case errors.Is(err, errDepsIncomplete):
		return apperr.New(apperr.CodeInternal, err.Error())
	default:
		return apperr.New(apperr.CodeNotFound, "")
	}
}

// notFound 构造统一的 404。
func notFound(what string) *apperr.Error {
	return apperr.New(apperr.CodeNotFound, what+"不存在或对当前身份不可见")
}

// forbidden 构造统一的 403。
func forbidden(what string) *apperr.Error {
	return apperr.New(apperr.CodeForbidden, what)
}

// wrap 为数据层错误补上操作与对象，使错误信息能定位到具体动作。
func wrap(action string, id int64, err error) error {
	return fmt.Errorf("%s（对象 %d）: %w", action, id, err)
}

// unexpectedValue 表示数据库里出现未登记的枚举取值。
//
// 这类情形说明库被外部改动或被写坏，必须报错而不是静默降级：
// 静默返回零值会让上层把「未知类型」当成某个具体类型处理。
func unexpectedValue(what string, id int64, value string) error {
	return fmt.Errorf("%s（对象 %d）的取值 %q 未登记", what, id, value)
}

// int8OrNull 把可选 ID 转为可空整数。nil 表示该字段无值（如顶层评论的 parent_id）。
func int8OrNull(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
