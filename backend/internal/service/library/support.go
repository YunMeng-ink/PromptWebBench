package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
)

// errDepsIncomplete 表示服务构造时缺少必要依赖。
var errDepsIncomplete = errors.New("题库服务依赖不完整：Pool、Config 均不可为空")

// notFound 构造统一的 404。
//
// 无权限与不存在对外不区分（开发方案 §5.3）：非成员、被引用提示词不可见的题目、
// 以及确实不存在的对象都返回同一个结果，使状态码差异无法用于探测对象存在性。
func notFound(what string) *apperr.Error {
	return apperr.New(apperr.CodeNotFound, what+"不存在或对当前身份不可见")
}

// forbidden 构造统一的 403。仅用于「对象可见但该操作超出角色权限」的情形，
// 题库本身不可见时一律返回 404。
func forbidden(message string) *apperr.Error {
	return apperr.New(apperr.CodeForbidden, message)
}

// wrap 为数据层错误补上操作与对象，使错误信息能定位到具体动作。
func wrap(action string, id int64, err error) error {
	return fmt.Errorf("%s（对象 %d）: %w", action, id, err)
}

// unexpectedValue 表示数据库里出现未登记的枚举取值。
func unexpectedValue(what string, id int64, value string) error {
	return fmt.Errorf("%s（对象 %d）的取值 %q 未登记", what, id, value)
}

// validateError 把领域层的校验错误翻译为字段校验失败。
func validateError(err error) error {
	return apperr.New(apperr.CodeValidationFailed, err.Error())
}

// errTagLength 是题目标签长度不合法时的错误。标签的合法区间在领域层声明，
// 此处只负责把它翻译成校验失败。
var errTagLength = errors.New("题目标签不能为空且不能超过 " +
	strconv.Itoa(domain.LibraryQuestionTagMaxRunes) + " 个字")

// encodeJSON 序列化快照与修订记录。序列化失败必须就地报错：
// 写入一半的快照会让回收站丢失恢复所需的依据。
func encodeJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("序列化快照: %w", err)
	}
	return raw, nil
}

// int8OrNull 把可选整数转为可空整数列。
func int8OrNull(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// int8Ptr 把可空整数列转为可选整数。无效值返回 nil。
func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

// textOrNull 把字符串转为可空文本列，空串视为无值。
func textOrNull(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// textValue 读取可空文本列，无效值返回空串。
func textValue(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// itoa 把整数写成十进制字符串，供导出与站内链接拼接使用。
func itoa(v int) string { return strconv.Itoa(v) }

// itoa64 把 64 位整数写成十进制字符串。
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
