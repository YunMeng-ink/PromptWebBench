package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// ReportFilter 是举报列表的筛选条件。空串表示该维度不参与筛选。
type ReportFilter struct {
	ObjectType string
	Status     string
}

// ReportHandleInput 是一次举报处置的输入（需求 §17.6）。
type ReportHandleInput struct {
	// Status 取 ignored 或 resolved；pending 用于把已处置的举报退回待处理。
	Status domain.ReportStatus
	// Note 是处理备注，会一并写入日志。
	Note string
	// DeleteContent 为真时删除被举报的提示词或评论。
	DeleteContent bool
	// BanAuthor 为真时封禁被举报内容的作者。
	BanAuthor bool
}

// ListReports 返回举报列表（需求 §17.6「查看提示词举报、查看评论举报」）。
func (s *Service) ListReports(ctx context.Context, f ReportFilter, page, pageSize int32) (Page[repo.AdminListReportsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListReportsRow]{}, err
	}
	if f.ObjectType != "" {
		if _, ok := domain.ParseReportObjectType(f.ObjectType); !ok {
			return Page[repo.AdminListReportsRow]{}, apperr.New(apperr.CodeValidationFailed, "举报对象类型取值非法")
		}
	}
	if f.Status != "" {
		if _, ok := domain.ParseReportStatus(f.Status); !ok {
			return Page[repo.AdminListReportsRow]{}, apperr.New(apperr.CodeValidationFailed, "举报状态取值非法")
		}
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListReports(ctx, repo.AdminListReportsParams{
		ObjectType: f.ObjectType, Status: f.Status,
		LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListReportsRow]{}, fmt.Errorf("查询举报列表: %w", err)
	}
	total, err := q.AdminCountReports(ctx, repo.AdminCountReportsParams{
		ObjectType: f.ObjectType, Status: f.Status,
	})
	if err != nil {
		return Page[repo.AdminListReportsRow]{}, fmt.Errorf("统计举报数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// HandleReport 处置一条举报（需求 §17.6）。
//
// 删除内容、封禁作者与标记状态在同一事务内完成：一次处置要么全部生效，
// 要么全部不生效，不会留下「用户被封禁但举报仍是待处理」这类中间状态。
//
// 删除动作委托给内容所属的域：提示词走 prompt.Service，评论走社交服务，
// 使删除的连带影响（回收站记录、推荐标签过期、删除日志）只有一处实现。
func (s *Service) HandleReport(ctx context.Context, operator domain.User, id int64, in ReportHandleInput, ip string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if _, ok := domain.ParseReportStatus(string(in.Status)); !ok {
		return apperr.New(apperr.CodeValidationFailed, "举报状态取值非法")
	}
	if len([]rune(in.Note)) > domain.ReportDetailMaxRunes {
		return apperr.New(apperr.CodeValidationFailed, "处理备注超出长度上限")
	}

	// 内容删除委托给内容所属的域，那个域拥有自己的事务边界；
	// 封禁、状态标记与日志则在同一个事务内完成，使三者要么全生效要么全不生效。
	report, err := s.reportByID(ctx, id)
	if err != nil {
		return err
	}

	var authorID int64
	if in.DeleteContent || in.BanAuthor {
		authorID, err = s.reportAuthor(ctx, report)
		if err != nil {
			return err
		}
	}

	if in.DeleteContent {
		if err := s.deleteReportedContent(ctx, operator, report, ip); err != nil {
			return err
		}
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		current, err := q.AdminGetReport(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("举报")
			}
			return wrap("查询举报", id, err)
		}
		if in.BanAuthor {
			if _, err := s.banUserTx(ctx, q, operator, authorID, "被举报内容处理", ip, true); err != nil {
				return err
			}
		}
		if _, err := q.HandleReport(ctx, repo.HandleReportParams{
			ID:         id,
			Status:     string(in.Status),
			HandleNote: in.Note,
			HandlerID:  repo.Int8(operator.ID),
		}); err != nil {
			return wrap("标记举报状态", id, err)
		}
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionReportHandle,
			ObjectType: "report",
			ObjectID:   id,
			Before:     map[string]any{"status": current.Status},
			After: map[string]any{
				"status":         string(in.Status),
				"note":           in.Note,
				"delete_content": in.DeleteContent,
				"ban_author":     in.BanAuthor,
			},
			IP: ip,
		})
	})
}

// reportByID 读取一条举报。
func (s *Service) reportByID(ctx context.Context, id int64) (repo.Report, error) {
	row, err := repo.New(s.pool).AdminGetReport(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Report{}, notFound("举报")
		}
		return repo.Report{}, wrap("查询举报", id, err)
	}
	return row, nil
}

// reportAuthor 解析被举报内容的作者。提示词取上传者，评论取评论人。
//
// 对象已被彻底删除时无法确定作者，此处直接报错：让管理员改用用户管理封禁，
// 比默默封禁一个猜测出来的账户安全。
func (s *Service) reportAuthor(ctx context.Context, report repo.Report) (int64, error) {
	q := repo.New(s.pool)
	switch domain.ReportObjectType(report.ObjectType) {
	case domain.ReportObjectPrompt:
		prompt, err := q.GetPromptIncludingDeleted(ctx, report.ObjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, invalidState("被举报提示词已不存在，无法确定其作者")
			}
			return 0, wrap("查询被举报提示词", report.ObjectID, err)
		}
		return prompt.UploaderID, nil
	case domain.ReportObjectComment:
		comment, err := q.GetCommentIncludingDeleted(ctx, report.ObjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, invalidState("被举报评论已不存在，无法确定其作者")
			}
			return 0, wrap("查询被举报评论", report.ObjectID, err)
		}
		return comment.UserID, nil
	default:
		return 0, apperr.New(apperr.CodeValidationFailed, "举报对象类型非法")
	}
}

// deleteReportedContent 删除被举报的内容。
func (s *Service) deleteReportedContent(ctx context.Context, operator domain.User, report repo.Report, ip string) error {
	switch domain.ReportObjectType(report.ObjectType) {
	case domain.ReportObjectPrompt:
		if s.prompts == nil {
			return apperr.New(apperr.CodeInternal, "提示词写入能力未注入")
		}
		if err := s.prompts.Delete(ctx, operator, report.ObjectID, ip); err != nil {
			// 已删除的提示词视为处置完成，重复处置不报错。
			if isCode(err, apperr.CodeNotFound) {
				return nil
			}
			return err
		}
		return nil
	case domain.ReportObjectComment:
		if s.moderate == nil {
			return apperr.New(apperr.CodeInternal, "内容处置能力未注入")
		}
		if err := s.moderate.DeleteComment(ctx, domain.ViewerFor(operator), report.ObjectID, ip); err != nil {
			if isCode(err, apperr.CodeNotFound) {
				return nil
			}
			return err
		}
		return nil
	default:
		return apperr.New(apperr.CodeValidationFailed, "举报对象类型非法")
	}
}

// isCode 判定错误是否为指定错误码。
func isCode(err error, code apperr.Code) bool {
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == code
}
