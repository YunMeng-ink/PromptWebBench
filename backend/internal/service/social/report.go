package social

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Report 是举报的服务层视图。
type Report struct {
	ID         int64
	ReporterID int64
	ObjectType domain.ReportObjectType
	ObjectID   int64
	Reason     domain.ReportReason
	Detail     string
}

// ReportInput 是提交举报的输入。
type ReportInput struct {
	ObjectType domain.ReportObjectType
	ObjectID   int64
	Reason     domain.ReportReason
	Detail     string
}

// CreateReport 提交举报（需求 §12.2）。
//
// 仅登录用户可举报（C-32）：权限矩阵显示游客「举报：否」。
// 举报原因存英文枚举键（C-31），中文文案只保留在前端语言包，
// 使英文界面能独立本地化，且与平台其余枚举的英文 slug 惯例一致。
//
// 同一用户对同一对象同时只保留一条待处理举报：重复提交会让管理端的待处理列表
// 被同一件事刷屏，而管理员处理一次就够了。
func (s *Service) CreateReport(ctx context.Context, v domain.Viewer, in ReportInput) (Report, error) {
	if err := s.ensure(); err != nil {
		return Report{}, translatePermission(err)
	}
	if !v.CanWrite() {
		return Report{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := domain.ValidateReportDetail(in.Detail); err != nil {
		return Report{}, apperr.New(apperr.CodeValidationFailed, err.Error())
	}

	var created Report
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		// 被举报对象必须真实存在且对举报人可见：对不可见对象举报会泄漏其存在性。
		switch in.ObjectType {
		case domain.ReportObjectPrompt:
			if _, err := loadVisiblePrompt(ctx, q, v, in.ObjectID); err != nil {
				return err
			}
		case domain.ReportObjectComment:
			row, err := q.GetCommentForReport(ctx, in.ObjectID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return notFound("评论")
				}
				return wrap("查询被举报评论", in.ObjectID, err)
			}
			if _, err := loadVisiblePrompt(ctx, q, v, row.PromptID); err != nil {
				return err
			}
		default:
			return apperr.New(apperr.CodeValidationFailed, "举报对象类型非法")
		}

		pending, err := q.CountPendingReportByReporter(ctx, repo.CountPendingReportByReporterParams{
			ReporterID: v.UserID,
			ObjectType: string(in.ObjectType),
			ObjectID:   in.ObjectID,
		})
		if err != nil {
			return wrap("统计待处理举报", in.ObjectID, err)
		}
		if pending > 0 {
			return apperr.New(apperr.CodeInvalidState, "你已举报过该内容，管理员正在处理")
		}

		row, err := q.InsertReport(ctx, repo.InsertReportParams{
			ReporterID: v.UserID,
			ObjectType: string(in.ObjectType),
			ObjectID:   in.ObjectID,
			Reason:     string(in.Reason),
			Detail:     in.Detail,
		})
		if err != nil {
			return wrap("写入举报", in.ObjectID, err)
		}

		reason, ok := domain.ParseReportReason(row.Reason)
		if !ok {
			return unexpectedValue("举报原因", row.ID, row.Reason)
		}
		objectType, ok := domain.ParseReportObjectType(row.ObjectType)
		if !ok {
			return unexpectedValue("举报对象类型", row.ID, row.ObjectType)
		}
		created = Report{
			ID:         row.ID,
			ReporterID: row.ReporterID,
			ObjectType: objectType,
			ObjectID:   row.ObjectID,
			Reason:     reason,
			Detail:     row.Detail,
		}
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	return created, nil
}
