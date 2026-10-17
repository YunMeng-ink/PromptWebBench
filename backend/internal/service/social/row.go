package social

import (
	"context"
	"fmt"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// promptFromRow 把 GetPrompt 的行映射为提示词实体。
//
// 高风险标记必须一并带入：漏掉它会让「考察目的高风险 + 可见性 public」的提示词
// 对普通用户可见，正是 C-20 要防止的绕过形态。
func promptFromRow(row repo.GetPromptRow) (domain.Prompt, error) {
	visibility, ok := domain.ParseVisibility(row.Visibility)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的可见性取值 %q 未登记", row.ID, row.Visibility)
	}
	sensitive, ok := domain.ParseSensitiveLevel(row.SensitiveLevel)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的敏感级别取值 %q 未登记", row.ID, row.SensitiveLevel)
	}
	version, err := domain.ParseVersion(row.LatestVersionNo)
	if err != nil {
		return domain.Prompt{}, fmt.Errorf("提示词 %d: %w", row.ID, err)
	}

	out := domain.Prompt{
		ID:            row.ID,
		Title:         row.Title,
		Description:   row.Description,
		PurposeID:     row.PurposeID,
		CapabilityID:  row.CapabilityID,
		Visibility:    visibility,
		Sensitive:     sensitive,
		HighRisk:      row.IsHighRisk,
		UploaderID:    row.UploaderID,
		LatestVersion: version,
		MajorModified: row.MajorModified,
		CommentCount:  row.CommentCount,
		FavoriteCount: row.FavoriteCount,
	}
	if row.DifficultyID.Valid {
		out.DifficultyID = &row.DifficultyID.Int64
	}
	if row.CurrentVersionID.Valid {
		out.CurrentVersionID = &row.CurrentVersionID.Int64
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	return out, nil
}

// promptFromItemRow 把收藏夹条目的行映射为提示词实体。
func promptFromItemRow(row repo.ListFavoriteItemsRow) (domain.Prompt, error) {
	visibility, ok := domain.ParseVisibility(row.Visibility)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的可见性取值 %q 未登记", row.ID, row.Visibility)
	}
	sensitive, ok := domain.ParseSensitiveLevel(row.SensitiveLevel)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的敏感级别取值 %q 未登记", row.ID, row.SensitiveLevel)
	}
	version, err := domain.ParseVersion(row.LatestVersionNo)
	if err != nil {
		return domain.Prompt{}, fmt.Errorf("提示词 %d: %w", row.ID, err)
	}

	out := domain.Prompt{
		ID:            row.ID,
		Title:         row.Title,
		Description:   row.Description,
		PurposeID:     row.PurposeID,
		CapabilityID:  row.CapabilityID,
		Visibility:    visibility,
		Sensitive:     sensitive,
		HighRisk:      row.IsHighRisk,
		UploaderID:    row.UploaderID,
		LatestVersion: version,
		MajorModified: row.MajorModified,
		CommentCount:  row.CommentCount,
		FavoriteCount: row.FavoriteCount,
	}
	if row.DifficultyID.Valid {
		out.DifficultyID = &row.DifficultyID.Int64
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	return out, nil
}

// oplogEntry 是一条待写入的操作日志。
type oplogEntry struct {
	Action     string
	Actor      domain.Viewer
	ObjectType string
	ObjectID   int64
	Before     any
	After      any
	Note       string
	IP         string
}

// record 写入操作日志。日志与业务变更同事务，避免出现有变更无记录的状态。
func (s *Service) record(ctx context.Context, q *repo.Queries, entry oplogEntry) error {
	return s.oplog.Record(ctx, q, oplog.Entry{
		OperatorID: entry.Actor.UserID,
		Action:     entry.Action,
		ObjectType: entry.ObjectType,
		ObjectID:   entry.ObjectID,
		Before:     entry.Before,
		After:      entry.After,
		Note:       entry.Note,
		IP:         entry.IP,
	})
}
