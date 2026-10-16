package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 8：操作日志（需求 §17.8、C-43）。
//
// 只有读端点：日志的不可篡改性来自「不存在编辑与删除接口」。
func (s *Server) registerAdminOperationLogRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/operation-logs",
		OperationID: "adminListOperationLogs", Summary: "操作日志", Tags: []string{"后台"},
	}, s.handleAdminListOperationLogs)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/operation-logs/actions",
		OperationID: "adminListOperationActions", Summary: "操作类型清单", Tags: []string{"后台"},
	}, s.handleAdminListOperationActions)
}

type adminListOperationLogsInput struct {
	Page       int32  `query:"page" default:"1" minimum:"1"`
	PageSize   int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Action     string `query:"action" maxLength:"50" doc:"按操作类型筛选"`
	ObjectType string `query:"object_type" maxLength:"50" doc:"按操作对象类型筛选"`
	Operator   string `query:"operator" maxLength:"100" doc:"按操作人昵称搜索"`
	Since      string `query:"since" doc:"起始时刻（RFC 3339，含）"`
	Until      string `query:"until" doc:"结束时刻（RFC 3339，不含）"`
}

// parseTime 解析可选的时间边界参数。
func parseTime(raw, label string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidationFailed, label+"需要 RFC 3339 形态")
	}
	value := parsed.UTC()
	return &value, nil
}

func (s *Server) handleAdminListOperationLogs(ctx context.Context, in *adminListOperationLogsInput) (*adminOperationLogListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	since, err := parseTime(in.Since, "起始时刻")
	if err != nil {
		return nil, err
	}
	until, err := parseTime(in.Until, "结束时刻")
	if err != nil {
		return nil, err
	}

	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListOperationLogs(ctx, admin.OplogFilter{
		Action: in.Action, ObjectType: in.ObjectType, Operator: in.Operator,
		Since: since, Until: until,
	}, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminOperationLogListResponse{}
	out.Body.Items = make([]adminOperationLogDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminOperationLogDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListOperationActions(ctx context.Context, _ *struct{}) (*adminOperationLogActionsResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	out := &adminOperationLogActionsResponse{}
	out.Body.Actions = oplog.AllActions()
	out.Body.Required = oplog.RequiredActions()
	return out, nil
}
