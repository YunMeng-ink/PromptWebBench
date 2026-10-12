package social

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// Comment 是评论的服务层视图。
type Comment struct {
	ID         int64
	PromptID   int64
	UserID     int64
	AuthorName string
	ParentID   *int64
	Body       string
	LikeCount  int32
	Liked      bool
	ReplyCount int64
	CreatedAt  time.Time
}

// CommentPage 是评论列表的一页。
type CommentPage struct {
	Items    []Comment
	Page     int32
	PageSize int32
	Total    int64
}

// CreateComment 发表评论或回复（需求 §12.1）。
//
// 回复时通知被回复评论的作者；回复自己的评论不通知（需求 §12.3 的评论回复通知）。
func (s *Service) CreateComment(ctx context.Context, v domain.Viewer, promptID int64, parentID *int64, body string) (Comment, error) {
	if err := s.ensure(); err != nil {
		return Comment{}, translatePermission(err)
	}
	if !v.CanWrite() {
		return Comment{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := domain.ValidateComment(body); err != nil {
		return Comment{}, apperr.New(apperr.CodeValidationFailed, err.Error())
	}

	var created Comment
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if _, err := loadVisiblePrompt(ctx, q, v, promptID); err != nil {
			return err
		}

		// 回复的目标必须先校验：不能回复一条已被删除或不存在的评论。
		var parent *domain.Comment
		if parentID != nil {
			row, err := q.GetComment(ctx, *parentID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return notFound("被回复的评论")
				}
				return wrap("查询被回复评论", *parentID, err)
			}
			// 回复必须落在同一条提示词下，否则会跨提示词串话。
			if row.PromptID != promptID {
				return notFound("被回复的评论")
			}
			target := domain.Comment{
				ID:       row.ID,
				PromptID: row.PromptID,
				UserID:   row.UserID,
			}
			parent = &target
		}

		row, err := q.InsertComment(ctx, repo.InsertCommentParams{
			PromptID: promptID,
			UserID:   v.UserID,
			ParentID: int8OrNull(parentID),
			Body:     body,
		})
		if err != nil {
			return wrap("写入评论", promptID, err)
		}

		if parent != nil && parent.UserID != v.UserID {
			if err := s.notify(ctx, q, notification{
				UserID: parent.UserID,
				Kind:   domain.NotifyCommentReply,
				Title:  "你的评论收到了回复",
				Body:   body,
				Link:   promptLink(promptID),
			}); err != nil {
				return err
			}
		}

		comment, err := commentFromRow(repo.ListCommentsRow{
			ID: row.ID, PromptID: row.PromptID, UserID: row.UserID,
			ParentID: row.ParentID, Body: row.Body, LikeCount: row.LikeCount,
			CreatedAt: row.CreatedAt,
		}, "")
		if err != nil {
			return err
		}
		comment.AuthorName = ""
		created = comment
		return nil
	})
	if err != nil {
		return Comment{}, err
	}
	return created, nil
}

// ListComments 返回某提示词的顶层评论。游客可读（需求 §12.1）。
func (s *Service) ListComments(ctx context.Context, v domain.Viewer, promptID int64, page, pageSize int32) (CommentPage, error) {
	if err := s.ensure(); err != nil {
		return CommentPage{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	if _, err := loadVisiblePrompt(ctx, q, v, promptID); err != nil {
		return CommentPage{}, err
	}

	rows, err := q.ListComments(ctx, repo.ListCommentsParams{
		ViewerID:    v.UserID,
		PromptID:    promptID,
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return CommentPage{}, wrap("查询评论列表", promptID, err)
	}
	total, err := q.CountComments(ctx, promptID)
	if err != nil {
		return CommentPage{}, wrap("统计评论数量", promptID, err)
	}

	items := make([]Comment, 0, len(rows))
	for _, row := range rows {
		item, err := commentFromRow(row, row.AuthorName)
		if err != nil {
			return CommentPage{}, err
		}
		item.Liked = row.Liked
		item.ReplyCount = row.ReplyCount
		items = append(items, item)
	}
	return CommentPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// ListReplies 返回某条评论下的回复（需求 §12.1）。
func (s *Service) ListReplies(ctx context.Context, v domain.Viewer, commentID int64, page, pageSize int32) (CommentPage, error) {
	if err := s.ensure(); err != nil {
		return CommentPage{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	parent, err := q.GetComment(ctx, commentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommentPage{}, notFound("评论")
		}
		return CommentPage{}, wrap("查询评论", commentID, err)
	}
	if _, err := loadVisiblePrompt(ctx, q, v, parent.PromptID); err != nil {
		return CommentPage{}, err
	}

	rows, err := q.ListReplies(ctx, repo.ListRepliesParams{
		ViewerID:    v.UserID,
		ParentID:    int8OrNull(&commentID),
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return CommentPage{}, wrap("查询回复列表", commentID, err)
	}
	total, err := q.CountReplies(ctx, int8OrNull(&commentID))
	if err != nil {
		return CommentPage{}, wrap("统计回复数量", commentID, err)
	}

	items := make([]Comment, 0, len(rows))
	for _, row := range rows {
		item := Comment{
			ID:         row.ID,
			PromptID:   row.PromptID,
			UserID:     row.UserID,
			AuthorName: row.AuthorName,
			Body:       row.Body,
			LikeCount:  row.LikeCount,
			Liked:      row.Liked,
		}
		if row.ParentID.Valid {
			parentID := row.ParentID.Int64
			item.ParentID = &parentID
		}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		items = append(items, item)
	}
	return CommentPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// DeleteComment 删除自己的评论。
//
// 初版评论删除为直接删除而不进回收站（需求 §11.2），但必须记录日志，
// 日志是这次删除行为的唯一留痕。
func (s *Service) DeleteComment(ctx context.Context, v domain.Viewer, commentID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		row, err := q.GetComment(ctx, commentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("评论")
			}
			return wrap("查询评论", commentID, err)
		}
		// 只能删除自己的评论，管理员同样走删除接口，权限由服务层判定。
		if row.UserID != v.UserID && !v.IsAdmin() {
			return notFound("评论")
		}

		if _, err := q.SoftDeleteComment(ctx, commentID); err != nil {
			return wrap("删除评论", commentID, err)
		}
		return s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionCommentDelete,
			Actor:      v,
			ObjectType: "comment",
			ObjectID:   commentID,
			Before: map[string]any{
				"prompt_id":  row.PromptID,
				"body":       row.Body,
				"like_count": row.LikeCount,
			},
			IP: ip,
		})
	})
}

// LikeComment 点赞评论。重复点赞为幂等成功。
func (s *Service) LikeComment(ctx context.Context, v domain.Viewer, commentID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		row, err := q.GetComment(ctx, commentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("评论")
			}
			return wrap("查询评论", commentID, err)
		}
		if _, err := loadVisiblePrompt(ctx, q, v, row.PromptID); err != nil {
			return err
		}

		if _, err := q.InsertCommentLike(ctx, repo.InsertCommentLikeParams{
			CommentID: commentID,
			UserID:    v.UserID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// 已经点过赞：幂等成功，不重复计数。
				return nil
			}
			return wrap("写入点赞", commentID, err)
		}
		if _, err := q.IncrementCommentLikeCount(ctx, commentID); err != nil {
			return wrap("更新点赞数", commentID, err)
		}
		return nil
	})
}

// UnlikeComment 取消点赞。未点过赞时为幂等成功。
func (s *Service) UnlikeComment(ctx context.Context, v domain.Viewer, commentID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if _, err := q.GetComment(ctx, commentID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("评论")
			}
			return wrap("查询评论", commentID, err)
		}
		removed, err := q.DeleteCommentLike(ctx, repo.DeleteCommentLikeParams{
			CommentID: commentID,
			UserID:    v.UserID,
		})
		if err != nil {
			return wrap("删除点赞", commentID, err)
		}
		// 未点过赞时影响 0 行，计数不能递减，否则会与点赞记录脱节。
		if removed == 0 {
			return nil
		}
		if _, err := q.DecrementCommentLikeCount(ctx, commentID); err != nil {
			return wrap("更新点赞数", commentID, err)
		}
		return nil
	})
}

// commentFromRow 把数据库行映射为服务层结构。
func commentFromRow(row repo.ListCommentsRow, authorName string) (Comment, error) {
	out := Comment{
		ID:         row.ID,
		PromptID:   row.PromptID,
		UserID:     row.UserID,
		AuthorName: authorName,
		Body:       row.Body,
		LikeCount:  row.LikeCount,
	}
	if row.ParentID.Valid {
		parentID := row.ParentID.Int64
		out.ParentID = &parentID
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	return out, nil
}
