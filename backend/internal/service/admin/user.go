package admin

import (
	"context"
	"fmt"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Page 是后台列表的统一分页结果，形状遵循 C-40。
type Page[T any] struct {
	Items    []T
	Page     int32
	PageSize int32
	Total    int64
}

// NewPage 组装一页结果。
func NewPage[T any](items []T, page, pageSize int32, total int64) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Page: page, PageSize: pageSize, Total: total}
}

// UserFilter 是用户列表的筛选条件。空串表示该维度不参与筛选。
type UserFilter struct {
	Keyword string
	Role    string
	Status  string
}

// ListUsers 返回用户列表（需求 §17.4）。角色与状态取值在服务层校验，
// 非法取值直接报错，避免把拼错的筛选条件当成「无结果」。
func (s *Service) ListUsers(ctx context.Context, f UserFilter, page, pageSize int32) (Page[repo.AdminListUsersRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListUsersRow]{}, err
	}
	if f.Role != "" {
		if _, err := domain.ParseRole(f.Role); err != nil {
			return Page[repo.AdminListUsersRow]{}, apperr.New(apperr.CodeValidationFailed, "身份组取值非法")
		}
	}
	if f.Status != "" {
		if _, err := domain.ParseStatus(f.Status); err != nil {
			return Page[repo.AdminListUsersRow]{}, apperr.New(apperr.CodeValidationFailed, "账户状态取值非法")
		}
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListUsers(ctx, repo.AdminListUsersParams{
		Keyword:     f.Keyword,
		Role:        f.Role,
		Status:      f.Status,
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListUsersRow]{}, fmt.Errorf("查询用户列表: %w", err)
	}
	total, err := q.AdminCountUsers(ctx, repo.AdminCountUsersParams{
		Keyword: f.Keyword, Role: f.Role, Status: f.Status,
	})
	if err != nil {
		return Page[repo.AdminListUsersRow]{}, fmt.Errorf("统计用户数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// ListUserRatings 返回某用户提交的评分（需求 §17.4「查看用户评分」）。
func (s *Service) ListUserRatings(ctx context.Context, userID int64, page, pageSize int32) (Page[repo.AdminListUserRatingsRow], error) {
	return listForUser(ctx, s, page, pageSize, userID,
		func(q *repo.Queries, userID int64, limit, offset int32) ([]repo.AdminListUserRatingsRow, error) {
			return q.AdminListUserRatings(ctx, repo.AdminListUserRatingsParams{
				UserID: userID, LimitCount: limit, OffsetCount: offset,
			})
		},
		func(q *repo.Queries, userID int64) (int64, error) { return q.AdminCountUserRatings(ctx, userID) },
		"查询用户评分")
}

// ListUserComments 返回某用户发表的评论（需求 §17.4「查看用户评论」）。
func (s *Service) ListUserComments(ctx context.Context, userID int64, page, pageSize int32) (Page[repo.AdminListUserCommentsRow], error) {
	return listForUser(ctx, s, page, pageSize, userID,
		func(q *repo.Queries, userID int64, limit, offset int32) ([]repo.AdminListUserCommentsRow, error) {
			return q.AdminListUserComments(ctx, repo.AdminListUserCommentsParams{
				UserID: userID, LimitCount: limit, OffsetCount: offset,
			})
		},
		func(q *repo.Queries, userID int64) (int64, error) { return q.AdminCountUserComments(ctx, userID) },
		"查询用户评论")
}

// ListUserLibraries 返回某用户拥有的题库（需求 §17.4「查看用户题库」）。
func (s *Service) ListUserLibraries(ctx context.Context, userID int64, page, pageSize int32) (Page[repo.AdminListUserLibrariesRow], error) {
	return listForUser(ctx, s, page, pageSize, userID,
		func(q *repo.Queries, userID int64, limit, offset int32) ([]repo.AdminListUserLibrariesRow, error) {
			return q.AdminListUserLibraries(ctx, repo.AdminListUserLibrariesParams{
				UserID: userID, LimitCount: limit, OffsetCount: offset,
			})
		},
		func(q *repo.Queries, userID int64) (int64, error) { return q.AdminCountUserLibraries(ctx, userID) },
		"查询用户题库")
}

// ListUserReports 返回某用户提交的举报（需求 §17.4「查看用户举报记录」）。
func (s *Service) ListUserReports(ctx context.Context, userID int64, page, pageSize int32) (Page[repo.AdminListUserReportsRow], error) {
	return listForUser(ctx, s, page, pageSize, userID,
		func(q *repo.Queries, userID int64, limit, offset int32) ([]repo.AdminListUserReportsRow, error) {
			return q.AdminListUserReports(ctx, repo.AdminListUserReportsParams{
				UserID: userID, LimitCount: limit, OffsetCount: offset,
			})
		},
		func(q *repo.Queries, userID int64) (int64, error) { return q.AdminCountUserReports(ctx, userID) },
		"查询用户举报记录")
}

// listForUser 收敛「按用户分页查询 + 统计」的四段重复结构。
//
// 目标用户不存在时报 not_found：四类记录都为空与用户不存在给出相同的空列表，
// 会让管理员无法分辨自己是不是点错了 ID。
func listForUser[T any](
	ctx context.Context,
	s *Service,
	page, pageSize int32,
	userID int64,
	query func(q *repo.Queries, userID int64, limit, offset int32) ([]T, error),
	count func(q *repo.Queries, userID int64) (int64, error),
	action string,
) (Page[T], error) {
	if err := s.ensure(); err != nil {
		return Page[T]{}, err
	}
	if _, err := s.UserByID(ctx, userID); err != nil {
		return Page[T]{}, err
	}
	q := repo.New(s.pool)
	rows, err := query(q, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return Page[T]{}, fmt.Errorf("%s: %w", action, err)
	}
	total, err := count(q, userID)
	if err != nil {
		return Page[T]{}, fmt.Errorf("%s: %w", action, err)
	}
	return NewPage(rows, page, pageSize, total), nil
}
