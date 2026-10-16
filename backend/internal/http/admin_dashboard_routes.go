package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// 模块 1：统计仪表盘（需求 §17.1）。
func (s *Server) registerAdminDashboardRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/dashboard",
		OperationID: "adminDashboard", Summary: "统计仪表盘", Tags: []string{"后台"},
	}, s.handleAdminDashboard)
}

func (s *Server) handleAdminDashboard(ctx context.Context, _ *struct{}) (*dashboardResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	board, err := s.deps.Admin.LoadDashboard(ctx)
	if err != nil {
		return nil, err
	}

	out := &dashboardResponse{}
	counts := board.Counts
	out.Body.UserTotal = counts.UserTotal
	out.Body.UserToday = counts.UserToday
	out.Body.UserRoleUser = counts.UserRoleUser
	out.Body.UserRoleDeveloper = counts.UserRoleDeveloper
	out.Body.UserBanned = counts.UserBanned
	out.Body.PromptTotal = counts.PromptTotal
	out.Body.PromptPublic = counts.PromptPublic
	out.Body.PromptPrivate = counts.PromptPrivate
	out.Body.PromptDeveloper = counts.PromptDeveloper
	out.Body.PromptToday = counts.PromptToday
	out.Body.RatingTotal = counts.RatingTotal
	out.Body.CommentTotal = counts.CommentTotal
	out.Body.ReportPending = counts.ReportPending
	out.Body.RecycleTotal = counts.RecycleTotal
	out.Body.ModelTotal = counts.ModelTotal
	out.Body.InviteTotal = counts.InviteTotal
	out.Body.RecommendPending = counts.RecommendPending
	out.Body.HotPrompts = toDashboardPrompts(board.Hot)
	out.Body.RecentPrompts = toDashboardPrompts(board.Recent)
	return out, nil
}
