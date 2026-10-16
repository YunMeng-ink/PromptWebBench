package admin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
)

// trimSpace 去除首尾空白。筛选参数与名称字段都先规范化再入库或比对。
func trimSpace(value string) string { return strings.TrimSpace(value) }

// wrap 为数据层错误附上动作与对象标识，便于在日志中定位。
func wrap(action string, id int64, err error) error {
	return fmt.Errorf("%s（对象 %d）: %w", action, id, err)
}

// notFound 构造未找到错误。
func notFound(label string) error {
	return apperr.New(apperr.CodeNotFound, label+"不存在")
}

// invalidState 构造状态不合法错误。
func invalidState(message string) error {
	return apperr.New(apperr.CodeInvalidState, message)
}

// encodePayload 把结构化快照序列化为 JSONB 取值。
func encodePayload(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("序列化字段快照: %w", err)
	}
	return raw, nil
}

// int8Ptr 把可空列取值转成可选指针。
func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

// deref 解引用可选 ID。0 与 nil 在本项目模型中同义：都表示「无值」。
func deref(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
