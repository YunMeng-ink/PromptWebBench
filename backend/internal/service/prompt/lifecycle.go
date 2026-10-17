package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// Delete 软删除提示词，进入回收站（需求 §11.1、§11.2）。仅所有者或管理员可删除（C-18）。
//
// 删除同时完成四件事，且在同一事务内：
//   - 落回收站记录（快照 + 到期时刻），使恢复与彻底删除有据可依；
//   - 推荐标签一律转已过期（C-14），因为对应的内容已不在前台；
//   - 写入操作日志（需求 §17.8「删除提示词」）；
//   - 作废统计缓存。
func (s *Service) Delete(ctx context.Context, actor domain.User, id int64, ip string) error {
	viewer := domain.ViewerFor(actor)
	if !viewer.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		locked, err := lockPrompt(ctx, q, id)
		if err != nil {
			return err
		}
		if !viewer.CanManage(locked.Access()) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		if _, err := q.SoftDeletePrompt(ctx, id); err != nil {
			return fmt.Errorf("软删除提示词: %w", err)
		}
		if err := s.enterRecycleBin(ctx, q, locked, actor.ID); err != nil {
			return err
		}
		if err := q.ExpireRecommendTagsByPrompt(ctx, id); err != nil {
			return fmt.Errorf("推荐标签转已过期: %w", err)
		}
		return s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   actor.ID,
			OperatorName: actor.Username,
			Action:       oplog.ActionPromptDelete,
			ObjectType:   "prompt",
			ObjectID:     id,
			Before:       promptSnapshot(locked),
			IP:           ip,
		})
	})
	if err != nil {
		return err
	}
	s.markStale(id)
	return nil
}

// Restore 从回收站恢复提示词（需求 §11.3）。仅所有者或管理员可恢复。
func (s *Service) Restore(ctx context.Context, actor domain.User, id int64, ip string) (domain.Prompt, error) {
	viewer := domain.ViewerFor(actor)

	var restored domain.Prompt
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		row, err := q.GetPromptIncludingDeleted(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询提示词: %w", err)
		}
		locked, err := promptFromRow(fieldsOfIncludingDeleted(row))
		if err != nil {
			return err
		}
		if !viewer.CanManage(locked.Access()) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		if !row.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "该提示词不在回收站中")
		}
		entry, err := q.GetLiveRecycleBinEntryByObject(ctx, repo.GetLiveRecycleBinEntryByObjectParams{
			ObjectType: string(domain.RecycleObjectPrompt),
			ObjectID:   id,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该提示词的回收站记录已结清")
			}
			return fmt.Errorf("查询回收站记录: %w", err)
		}
		if _, err := q.RestorePrompt(ctx, id); err != nil {
			return fmt.Errorf("恢复提示词: %w", err)
		}
		if err := q.MarkRecycleBinRestored(ctx, entry.ID); err != nil {
			return fmt.Errorf("标记回收站记录已恢复: %w", err)
		}
		if err := s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   actor.ID,
			OperatorName: actor.Username,
			Action:       oplog.ActionPromptRestore,
			ObjectType:   "prompt",
			ObjectID:     id,
			Note:         fmt.Sprintf("恢复提示词「%s」", locked.Title),
			IP:           ip,
		}); err != nil {
			return err
		}
		restored = locked
		return nil
	})
	if err != nil {
		return domain.Prompt{}, err
	}
	s.markStale(id)
	return restored, nil
}

// Purge 彻底删除回收站中的提示词（需求 §11.3）。仅所有者或管理员可执行。
//
// 删除前做引用检查：被私有题库题目引用的提示词不可彻底删除。
// 引用题的外键是 ON DELETE SET NULL，而题目形状约束要求引用题必须有来源，
// 直接删除会违反数据库约束；此处提前给出 prompt_in_use，让管理员先处理题库引用。
func (s *Service) Purge(ctx context.Context, actor domain.User, id int64, ip string) error {
	viewer := domain.ViewerFor(actor)
	if !viewer.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		// 先锁定行再判定状态：与恢复、引用写入串行化，杜绝清理竞态。
		row, err := q.LockPromptIncludingDeleted(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询提示词: %w", err)
		}
		locked, err := promptFromRow(fieldsOfLockIncludingDeleted(row))
		if err != nil {
			return err
		}
		if !viewer.CanManage(locked.Access()) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		if !row.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "该提示词不在回收站中")
		}

		references, err := q.CountLibraryQuestionsByPrompt(ctx, id)
		if err != nil {
			return fmt.Errorf("统计题库引用: %w", err)
		}
		if references > 0 {
			return apperr.New(apperr.CodePromptInUse,
				fmt.Sprintf("该提示词被 %d 道题库题目引用，请先解除引用", references))
		}

		// 彻底删除会级联清除 favorite_items：先统计再删除，随后校正各收藏夹计数，
		// 否则 item_count 虚高会侵蚀收藏夹容量。
		favoriteCounts, err := q.ListFavoriteItemCountsByPrompt(ctx, id)
		if err != nil {
			return fmt.Errorf("统计收藏夹条目: %w", err)
		}

		deleted, err := q.HardDeletePrompt(ctx, id)
		if err != nil {
			return fmt.Errorf("彻底删除提示词: %w", err)
		}
		if deleted == 0 {
			// 行锁持有期间恢复事务不可能提交，此处只在行已被其他清理者删除时出现。
			return apperr.New(apperr.CodeInvalidState, "该提示词已被清理")
		}
		for _, fc := range favoriteCounts {
			if err := q.AdjustFavoriteItemCount(ctx, repo.AdjustFavoriteItemCountParams{
				ID:    fc.FavoriteID,
				Delta: fc.ItemCount,
			}); err != nil {
				return fmt.Errorf("校正收藏夹条目计数: %w", err)
			}
		}
		if err := q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
			ObjectType: string(domain.RecycleObjectPrompt),
			ObjectID:   id,
		}); err != nil {
			return fmt.Errorf("标记回收站记录已清除: %w", err)
		}
		return s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   actor.ID,
			OperatorName: actor.Username,
			Action:       oplog.ActionPromptPurge,
			ObjectType:   "prompt",
			ObjectID:     id,
			Before:       promptSnapshot(locked),
			IP:           ip,
		})
	})
	if err != nil {
		return err
	}
	s.markStale(id)
	return nil
}

// enterRecycleBin 为被删除的提示词写入回收站记录，到期时刻由配置的保留天数决定。
func (s *Service) enterRecycleBin(ctx context.Context, q *repo.Queries, p domain.Prompt, deletedBy int64) error {
	raw, err := json.Marshal(promptSnapshot(p))
	if err != nil {
		return fmt.Errorf("序列化提示词快照: %w", err)
	}
	deletedAt := s.now().UTC()
	if _, err := q.InsertRecycleBinEntry(ctx, repo.InsertRecycleBinEntryParams{
		ObjectType: string(domain.RecycleObjectPrompt),
		ObjectID:   p.ID,
		OwnerID:    repo.Int8(p.UploaderID),
		Snapshot:   raw,
		DeletedBy:  repo.Int8(deletedBy),
		ExpiresAt:  repo.Timestamptz(domain.RecycleBinExpiry(deletedAt, s.retentionDays(ctx))),
	}); err != nil {
		return fmt.Errorf("写入回收站记录: %w", err)
	}
	return nil
}

// retentionDays 返回回收站保留天数（需求 §11.3）。
func (s *Service) retentionDays(ctx context.Context) int64 {
	days, err := s.config.GetInt(ctx, "recycle_retention_days")
	if err != nil || days <= 0 {
		return int64(domain.RecycleRetentionDays)
	}
	return days
}

// promptSnapshot 是写入操作日志与回收站的提示词摘要。
func promptSnapshot(p domain.Prompt) map[string]any {
	return map[string]any{
		"id":            p.ID,
		"title":         p.Title,
		"visibility":    string(p.Visibility),
		"uploader_id":   p.UploaderID,
		"version_no":    p.LatestVersion.String(),
		"purpose_id":    p.PurposeID,
		"capability_id": p.CapabilityID,
		"high_risk":     p.HighRisk,
		"sensitive":     string(p.Sensitive),
		"major_modify":  p.MajorModified,
	}
}
