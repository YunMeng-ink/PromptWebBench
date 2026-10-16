package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// OplogFilter 是操作日志的筛选条件。空串与 nil 表示该维度不参与筛选。
type OplogFilter struct {
	Action     string
	ObjectType string
	Operator   string
	Since      *time.Time
	Until      *time.Time
}

// ListOperationLogs 返回操作日志（需求 §17.8、C-43）。
//
// 只提供读取：日志的不可篡改性来自「没有编辑与删除接口」，
// 因此本模块不提供任何写方法，handler 也不注册写路由。
func (s *Service) ListOperationLogs(ctx context.Context, f OplogFilter, page, pageSize int32) (Page[repo.OperationLog], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.OperationLog]{}, err
	}
	if f.Action != "" && !isKnownAction(f.Action) {
		return Page[repo.OperationLog]{}, apperr.New(apperr.CodeValidationFailed, "操作类型取值非法")
	}
	if f.Since != nil && f.Until != nil && !f.Until.After(*f.Since) {
		return Page[repo.OperationLog]{}, apperr.New(apperr.CodeValidationFailed, "结束时刻必须晚于起始时刻")
	}

	q := repo.New(s.pool)
	params := repo.AdminListOperationLogsParams{
		Action:      f.Action,
		ObjectType:  f.ObjectType,
		Operator:    f.Operator,
		Since:       repo.TimestamptzOrNull(f.Since),
		Until:       repo.TimestamptzOrNull(f.Until),
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	}
	rows, err := q.AdminListOperationLogs(ctx, params)
	if err != nil {
		return Page[repo.OperationLog]{}, fmt.Errorf("查询操作日志: %w", err)
	}
	total, err := q.AdminCountOperationLogs(ctx, repo.AdminCountOperationLogsParams{
		Action:     f.Action,
		ObjectType: f.ObjectType,
		Operator:   f.Operator,
		Since:      params.Since,
		Until:      params.Until,
	})
	if err != nil {
		return Page[repo.OperationLog]{}, fmt.Errorf("统计操作日志数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// isKnownAction 判定操作类型是否为已登记取值。
func isKnownAction(action string) bool {
	for _, known := range oplog.AllActions() {
		if action == known {
			return true
		}
	}
	return false
}
