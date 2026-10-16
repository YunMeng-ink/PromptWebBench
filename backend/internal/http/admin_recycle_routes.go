package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// 模块 10：回收站管理（需求 §11.3、§2.4）。管理员可查看全部对象；
// 恢复与彻底删除限定于提示词，题库与题库题目由所有者在前台处置。
func (s *Server) registerAdminRecycleRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/recycle-bin",
		OperationID: "adminListRecycleBin", Summary: "回收站列表", Tags: []string{"后台"},
	}, s.handleAdminListRecycleBin)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/recycle-bin/{objectType}/{objectID}/restore",
		OperationID: "adminRestoreRecycleEntry", Summary: "恢复回收站对象", Tags: []string{"后台"},
	}, s.handleAdminRestoreRecycleEntry)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/recycle-bin/{objectType}/{objectID}/purge",
		OperationID: "adminPurgeRecycleEntry", Summary: "彻底删除回收站对象", Tags: []string{"后台"},
	}, s.handleAdminPurgeRecycleEntry)
}

type adminListRecycleInput struct {
	Page       int32  `query:"page" default:"1" minimum:"1"`
	PageSize   int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	ObjectType string `query:"object_type" enum:",prompt,library,library_question" doc:"按对象类型筛选"`
}

func (s *Server) handleAdminListRecycleBin(ctx context.Context, in *adminListRecycleInput) (*adminRecycleListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListRecycleBin(ctx, in.ObjectType, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminRecycleListResponse{}
	out.Body.Items = make([]adminRecycleEntryDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminRecycleEntryDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminRecycleObjectInput struct {
	ObjectType string `path:"objectType" doc:"对象类型：prompt、library、library_question"`
	ObjectID   int64  `path:"objectID" doc:"对象 ID"`
}

func (s *Server) handleAdminRestoreRecycleEntry(ctx context.Context, in *adminRecycleObjectInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.RestoreRecycleEntry(ctx, operator, in.ObjectType, in.ObjectID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleAdminPurgeRecycleEntry(ctx context.Context, in *adminRecycleObjectInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.PurgeRecycleEntry(ctx, operator, in.ObjectType, in.ObjectID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
