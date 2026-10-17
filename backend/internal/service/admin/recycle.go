package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// ListRecycleBin 返回回收站中的全部对象（需求 §11.3「管理员是否可查看回收站：是」）。
//
// 三类对象都列出：提示词、整个私有题库、题库中的题目。objectType 为空时不过滤。
func (s *Service) ListRecycleBin(ctx context.Context, objectType string, page, pageSize int32) (Page[repo.AdminListRecycleBinRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListRecycleBinRow]{}, err
	}
	if objectType != "" {
		if _, ok := domain.ParseRecycleBinObjectType(objectType); !ok {
			return Page[repo.AdminListRecycleBinRow]{}, apperr.New(apperr.CodeValidationFailed, "回收站对象类型取值非法")
		}
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListRecycleBin(ctx, repo.AdminListRecycleBinParams{
		ObjectType: objectType, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListRecycleBinRow]{}, fmt.Errorf("查询回收站: %w", err)
	}
	total, err := q.AdminCountRecycleBin(ctx, objectType)
	if err != nil {
		return Page[repo.AdminListRecycleBinRow]{}, fmt.Errorf("统计回收站对象数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// RestoreRecycleEntry 从回收站恢复一个提示词（需求 §11.3）。
func (s *Service) RestoreRecycleEntry(ctx context.Context, operator domain.User, objectType string, objectID int64, ip string) error {
	if err := requirePromptRecycleObject(objectType); err != nil {
		return err
	}
	if s.prompts == nil {
		return apperr.New(apperr.CodeInternal, "提示词写入能力未注入")
	}
	_, err := s.prompts.Restore(ctx, operator, objectID, ip)
	return err
}

// PurgeRecycleEntry 彻底删除回收站中的一个提示词（需求 §11.3）。
func (s *Service) PurgeRecycleEntry(ctx context.Context, operator domain.User, objectType string, objectID int64, ip string) error {
	if err := requirePromptRecycleObject(objectType); err != nil {
		return err
	}
	if s.prompts == nil {
		return apperr.New(apperr.CodeInternal, "提示词写入能力未注入")
	}
	return s.prompts.Purge(ctx, operator, objectID, ip)
}

// requirePromptRecycleObject 校验对象类型，并限定管理端只能恢复或彻底删除提示词。
//
// 题库与题库题目属于用户的私有数据：管理员在他人题库上的权限按 C-18
// 限定为查看、查看回收站与导出，恢复与彻底删除只能由题库所有者在前台完成。
func requirePromptRecycleObject(objectType string) error {
	if _, ok := domain.ParseRecycleBinObjectType(objectType); !ok {
		return apperr.New(apperr.CodeValidationFailed, "回收站对象类型取值非法")
	}
	if objectType != string(domain.RecycleObjectPrompt) {
		return apperr.New(apperr.CodePermissionDenied,
			"题库与题库题目由所有者在前台的题库回收站中恢复或彻底删除")
	}
	return nil
}

// PurgeExpiredPrompts 清理到期未恢复的提示词回收站记录（需求 §11.3：到期自动彻底删除）。
//
// 被题库题目引用的提示词无法彻底删除，此时跳过该条并保留记录：
// 引用解除后，下一次清理会成功，不需要人工干预。
func (s *Service) PurgeExpiredPrompts(ctx context.Context, limit int32) (int, error) {
	if err := s.ensure(); err != nil {
		return 0, err
	}
	entries, err := repo.New(s.pool).ListExpiredRecycleBinEntries(ctx, limit)
	if err != nil {
		return 0, wrap("查询到期回收站记录", 0, err)
	}

	purged := 0
	for _, entry := range entries {
		if entry.ObjectType != string(domain.RecycleObjectPrompt) {
			// 题库与题库题目的到期清理由题库域负责。
			continue
		}
		done, err := s.purgeExpiredPrompt(ctx, entry)
		if err != nil {
			return purged, err
		}
		if done {
			purged++
		}
	}
	return purged, nil
}

// purgeExpiredPrompt 清理一条到期的提示词记录。返回是否真的删除了对象：
// 被题库引用的提示词会被跳过，此时不算作已清理。
func (s *Service) purgeExpiredPrompt(ctx context.Context, entry repo.RecycleBin) (bool, error) {
	purged := false
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		// 先锁定提示词行：并发恢复提交后删除语句不命中，恢复方胜出。
		row, err := q.LockPromptIncludingDeleted(ctx, entry.ObjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// 提示词行已被其他路径删除，回收站记录可以直接结清。
				return q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
					ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
				})
			}
			return wrap("锁定到期提示词", entry.ObjectID, err)
		}
		if !row.DeletedAt.Valid {
			// 已被并发恢复：恢复事务会在恢复时标记回收站记录，此处跳过。
			return nil
		}

		references, err := q.CountLibraryQuestionsByPrompt(ctx, entry.ObjectID)
		if err != nil {
			return wrap("统计题库引用", entry.ObjectID, err)
		}
		if references > 0 {
			// 引用未解除：保留回收站记录，等待下一次清理。
			s.logger.Warn("提示词仍被题库引用，暂不彻底删除",
				"prompt_id", entry.ObjectID, "references", references)
			return nil
		}

		// 彻底删除会级联清除 favorite_items：先统计再删除，随后校正各收藏夹计数。
		favoriteCounts, err := q.ListFavoriteItemCountsByPrompt(ctx, entry.ObjectID)
		if err != nil {
			return wrap("统计收藏夹条目", entry.ObjectID, err)
		}
		deleted, err := q.HardDeletePrompt(ctx, entry.ObjectID)
		if err != nil {
			return wrap("清理到期提示词", entry.ObjectID, err)
		}
		if deleted == 0 {
			// 行锁持有期间恢复无法提交，此处仅在并发清理时出现。
			return nil
		}
		for _, fc := range favoriteCounts {
			if err := q.AdjustFavoriteItemCount(ctx, repo.AdjustFavoriteItemCountParams{
				ID:    fc.FavoriteID,
				Delta: fc.ItemCount,
			}); err != nil {
				return wrap("校正收藏夹条目计数", fc.FavoriteID, err)
			}
		}
		if err := q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
			ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
		}); err != nil {
			return wrap("标记回收站记录已清除", entry.ObjectID, err)
		}
		purged = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return purged, nil
}

// RecycleEntryTitle 从回收站快照中取出对象名称，供接口层回显。
// 快照键由各域的写入路径产生：提示词与题目用 title，题库用 name。
func RecycleEntryTitle(snapshot []byte) string {
	if len(snapshot) == 0 {
		return ""
	}
	var payload struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(snapshot, &payload); err != nil {
		return ""
	}
	if payload.Title != "" {
		return payload.Title
	}
	return payload.Name
}
