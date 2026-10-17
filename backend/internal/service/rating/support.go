package rating

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// rating_change_logs 的动作取值，与该表 CHECK 约束一致。
const (
	actionCreate = "create"
	actionUpdate = "update"
	actionDelete = "delete"
)

// logEntry 是一条评分变更日志。修改与删除的限额都以本表为计数依据，
// 因此每一次变更都必须留痕（需求 §3.4「是否记录日志：是」）。
type logEntry struct {
	RatingID        int64
	Actor           domain.User
	PromptID        int64
	ModelID         int64
	Action          string
	BeforeScore     int16
	AfterScore      int16
	BeforeEditCount int32
	AfterEditCount  int32
	IP              string
}

func (s *Service) logChange(ctx context.Context, q *repo.Queries, e logEntry) error {
	_, err := q.InsertRatingChangeLog(ctx, repo.InsertRatingChangeLogParams{
		RatingID:        repo.Int8(e.RatingID),
		UserID:          e.Actor.ID,
		PromptID:        e.PromptID,
		ModelID:         e.ModelID,
		Action:          e.Action,
		BeforeScore:     nullableInt2(e.BeforeScore),
		AfterScore:      nullableInt2(e.AfterScore),
		BeforeEditCount: nullableInt4(e.BeforeEditCount),
		AfterEditCount:  nullableInt4(e.AfterEditCount),
		Ip:              e.IP,
	})
	if err != nil {
		return fmt.Errorf("记录评分变更日志: %w", err)
	}
	return nil
}

// loadRateablePrompt 读取可被评分的提示词：必须是当前可见且版本完整。
// 私有与开发者可见提示词只对可见身份开放（需求 §8.3、§8.4）。
func loadRateablePrompt(ctx context.Context, q *repo.Queries, v domain.Viewer, id int64) (domain.Prompt, error) {
	row, err := q.GetPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Prompt{}, apperr.New(apperr.CodeNotFound, "")
		}
		return domain.Prompt{}, fmt.Errorf("查询提示词: %w", err)
	}
	prompt, err := promptFromRow(row)
	if err != nil {
		return domain.Prompt{}, err
	}
	if !v.CanView(prompt.Access()) {
		return domain.Prompt{}, apperr.New(apperr.CodeNotFound, "")
	}
	return prompt, nil
}

// promptFromRow 把 GetPrompt 的行映射为提示词实体。
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
		Body:          row.Body,
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

func ratingFromRow(row repo.Rating) (domain.Rating, error) {
	out := domain.Rating{
		ID:              row.ID,
		UserID:          row.UserID,
		PromptID:        row.PromptID,
		PromptVersionID: row.PromptVersionID,
		ModelID:         row.ModelID,
		Score:           row.Score,
		EditCount:       row.EditCount,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	return out, nil
}

// nullableInt2 把 0 视为无值：评分量表从 1 起，0 只出现在「尚未发生」的位置。
func nullableInt2(v int16) pgtype.Int2 {
	if v == 0 {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: v, Valid: true}
}

func nullableInt4(v int32) pgtype.Int4 {
	if v == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: v, Valid: true}
}
