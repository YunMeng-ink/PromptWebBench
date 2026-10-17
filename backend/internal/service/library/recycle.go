package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// RecycleEntry 是题库内回收站中的一条记录。
type RecycleEntry struct {
	Question  Question
	DeletedBy int64
	DeletedAt time.Time
	ExpiresAt time.Time
}

// LibraryRecycleEntry 是题库级回收站中的一条记录（整个题库被删除）。
type LibraryRecycleEntry struct {
	LibraryID   int64
	Name        string
	Description string
	OwnerID     int64
	DeletedBy   int64
	DeletedAt   time.Time
	ExpiresAt   time.Time
}

// recycleEntry 是一条待写入的回收站记录。
type recycleEntry struct {
	ObjectType domain.RecycleBinObjectType
	ObjectID   int64
	OwnerID    int64
	LibraryID  *int64
	Snapshot   map[string]any
	DeletedBy  int64
}

// DeleteQuestion 删除题目（需求 §10.4、§10.5）。
//
// 所有者与协作者都可以删除任何题目。按 C-34 记录六项要素：
//  1. 完整操作日志（operation_logs 的 library_question_delete）；
//  2. 删除人（快照与日志的操作人）；
//  3. 被删除题目（对象标识与标题）；
//  4. 删除时间（回收站记录的删除时刻）；
//  5. 删除前题目快照（回收站记录的 snapshot）；
//  6. 通知题库所有者，删除者本人是所有者时不通知（需求 §10.6）。
//
// 日志、回收站记录与通知都与删除同事务：任一环节缺失都会让审计出现缺口。
func (s *Service) DeleteQuestion(ctx context.Context, v domain.Viewer, questionID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return notFound("题目")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
			ID:             questionID,
			IncludeDeleted: false,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("查询题目", questionID, err)
		}
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapDeleteQuestion, "当前身份不可以删除题目"); err != nil {
			return err
		}
		question, err := s.loadQuestion(ctx, q, actor, lib, questionID, false)
		if err != nil {
			return err
		}

		tags, err := q.ListLibraryQuestionTags(ctx, questionID)
		if err != nil {
			return wrap("查询题目标签", questionID, err)
		}
		snapshot := questionSnapshot(question, tags)

		if _, err := q.SoftDeleteLibraryQuestion(ctx, questionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("删除题目", questionID, err)
		}
		if err := s.entryToRecycleBin(ctx, q, recycleEntry{
			ObjectType: domain.RecycleObjectLibraryQuestion,
			ObjectID:   questionID,
			OwnerID:    lib.OwnerID,
			LibraryID:  &lib.ID,
			Snapshot:   snapshot,
			DeletedBy:  v.UserID,
		}); err != nil {
			return err
		}

		actorName, err := s.actorName(ctx, q, v.UserID)
		if err != nil {
			return err
		}
		if err := s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionLibraryQuestionDelete,
			Actor:      v,
			ActorName:  actorName,
			ObjectType: "library_question",
			ObjectID:   questionID,
			Before:     snapshot,
			Note:       fmt.Sprintf("题库「%s」中的题目被删除", lib.Name),
			IP:         ip,
		}); err != nil {
			return err
		}

		// 所有者本人删除不通知自己（需求 §10.6）。
		if actor.IsOwner() {
			return nil
		}
		if s.notifier == nil {
			return fmt.Errorf("通知服务未注入，无法通知题库所有者（C-34）")
		}
		return s.notifier.NotifyLibraryQuestionDeleted(
			ctx, q, lib.OwnerID, lib.Name, question.Title, actorName,
		)
	})
}

// ListRecycleBin 返回题库内回收站中的题目。
//
// 与正常列表共用同一套可见性判定：来源提示词被改为开发者可见或高风险后，
// 普通用户协作者在回收站里同样看不到它。
func (s *Service) ListRecycleBin(ctx context.Context, v domain.Viewer, libraryID int64) ([]RecycleEntry, error) {
	if err := s.ensure(); err != nil {
		return nil, notFound("题库")
	}

	q := repo.New(s.pool)
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
	if err != nil {
		return nil, err
	}
	if err := requireCapability(actor, domain.CapViewRecycleBin, "当前身份不可以查看题库回收站"); err != nil {
		return nil, err
	}

	questions, err := s.visibleQuestions(ctx, q, actor, lib, true)
	if err != nil {
		return nil, err
	}
	entries, err := q.ListLibraryRecycleBin(ctx, libraryID)
	if err != nil {
		return nil, wrap("查询题库回收站", libraryID, err)
	}
	meta := make(map[int64]repo.RecycleBin, len(entries))
	for _, entry := range entries {
		meta[entry.ObjectID] = entry
	}

	out := make([]RecycleEntry, 0, len(entries))
	for _, question := range questions {
		if !question.Deleted {
			continue
		}
		entry, ok := meta[question.ID]
		if !ok {
			// 回收站记录已结清（例如保留期到期被清理），不再对外展示。
			continue
		}
		question.DeletedBy = int8Ptr(entry.DeletedBy)
		if entry.ExpiresAt.Valid {
			question.ExpiresAt = entry.ExpiresAt.Time
		}
		item := RecycleEntry{Question: question, DeletedAt: question.DeletedAt}
		if question.DeletedBy != nil {
			item.DeletedBy = *question.DeletedBy
		}
		if entry.ExpiresAt.Valid {
			item.ExpiresAt = entry.ExpiresAt.Time
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DeletedAt.After(out[j].DeletedAt)
	})
	return out, nil
}

// RestoreQuestion 从题库回收站恢复题目（需求 §10.4、§10.5）。
//
// 协作者是否可恢复由配置 collaborator_restore_allowed 决定（需求 §10.5 末条）。
func (s *Service) RestoreQuestion(ctx context.Context, v domain.Viewer, questionID int64, ip string) (Question, error) {
	if err := s.ensure(); err != nil {
		return Question{}, notFound("题目")
	}
	if !v.CanWrite() {
		return Question{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}

	var restored Question
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
			ID:             questionID,
			IncludeDeleted: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("查询题目", questionID, err)
		}
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapRestoreQuestion, "当前身份不可以恢复题目"); err != nil {
			return err
		}
		if !actor.IsOwner() && !s.boolOf(ctx, "collaborator_restore_allowed", true) {
			return forbidden("管理员已关闭协作者恢复题目的能力")
		}

		question, err := s.loadQuestion(ctx, q, actor, lib, questionID, true)
		if err != nil {
			return err
		}
		if !question.Deleted {
			return apperr.New(apperr.CodeInvalidState, "该题目不在回收站中")
		}
		entry, err := q.GetLiveRecycleBinEntry(ctx, repo.GetLiveRecycleBinEntryParams{
			ObjectType: string(domain.RecycleObjectLibraryQuestion),
			ObjectID:   questionID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该题目的回收站记录已结清")
			}
			return wrap("查询回收站记录", questionID, err)
		}

		if _, err := q.RestoreLibraryQuestion(ctx, questionID); err != nil {
			return wrap("恢复题目", questionID, err)
		}
		if err := q.MarkRecycleBinRestored(ctx, entry.ID); err != nil {
			return wrap("标记回收站记录已恢复", entry.ID, err)
		}
		actorName, err := s.actorName(ctx, q, v.UserID)
		if err != nil {
			return err
		}
		if err := s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionLibraryQuestionRestore,
			Actor:      v,
			ActorName:  actorName,
			ObjectType: "library_question",
			ObjectID:   questionID,
			Note:       fmt.Sprintf("恢复题库「%s」中的题目", lib.Name),
			IP:         ip,
		}); err != nil {
			return err
		}

		after, err := s.loadQuestion(ctx, q, actor, lib, questionID, false)
		if err != nil {
			return err
		}
		restored = after
		return nil
	})
	if err != nil {
		return Question{}, err
	}
	return restored, nil
}

// PurgeQuestion 彻底删除回收站中的题目（需求 §10.4）。仅所有者可执行。
//
// 引用题被彻底删除时才释放题库容量：回收站中的引用题仍占容量（C-22）。
func (s *Service) PurgeQuestion(ctx context.Context, v domain.Viewer, questionID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return notFound("题目")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
			ID:             questionID,
			IncludeDeleted: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("查询题目", questionID, err)
		}
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapPurgeQuestion, "只有题库所有者可以彻底删除题目"); err != nil {
			return err
		}

		before, err := s.loadQuestion(ctx, q, actor, lib, questionID, true)
		if err != nil {
			return err
		}
		if !before.Deleted {
			return apperr.New(apperr.CodeInvalidState, "该题目不在回收站中")
		}

		deleted, err := q.HardDeleteLibraryQuestion(ctx, questionID)
		if err != nil {
			return wrap("彻底删除题目", questionID, err)
		}
		if deleted == 0 {
			// 并发恢复已提交：恢复方会标记回收站记录，此处让清理方失败。
			return apperr.New(apperr.CodeInvalidState, "该题目已被恢复")
		}
		if err := q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
			ObjectType: string(domain.RecycleObjectLibraryQuestion),
			ObjectID:   questionID,
		}); err != nil {
			return wrap("标记回收站记录已清除", questionID, err)
		}
		if before.Kind == domain.QuestionReference {
			// 只有引用题计入容量，彻底删除时才需要归还（C-22）。
			if err := q.DecrementLibraryQuestionCount(ctx, ref.LibraryID); err != nil {
				return wrap("归还题库题目计数", ref.LibraryID, err)
			}
		}
		actorName, err := s.actorName(ctx, q, v.UserID)
		if err != nil {
			return err
		}
		return s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionRecyclePurge,
			Actor:      v,
			ActorName:  actorName,
			ObjectType: "library_question",
			ObjectID:   questionID,
			Before:     questionSnapshot(before, before.Tags),
			Note:       "彻底删除题库题目",
			IP:         ip,
		})
	})
}

// RestoreLibrary 从回收站恢复整个题库（需求 §11.3）。仅所有者可执行。
func (s *Service) RestoreLibrary(ctx context.Context, v domain.Viewer, libraryID int64, ip string) (Library, error) {
	if err := s.ensure(); err != nil {
		return Library{}, notFound("题库")
	}
	if !v.CanWrite() {
		return Library{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}

	var restored Library
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadDeletedLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapDeleteLibrary, "只有题库所有者可以恢复题库"); err != nil {
			return err
		}
		entry, err := q.GetLiveRecycleBinEntry(ctx, repo.GetLiveRecycleBinEntryParams{
			ObjectType: string(domain.RecycleObjectLibrary),
			ObjectID:   libraryID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该题库的回收站记录已结清")
			}
			return wrap("查询回收站记录", libraryID, err)
		}
		row, err := q.RestoreLibrary(ctx, libraryID)
		if err != nil {
			return wrap("恢复题库", libraryID, err)
		}
		if err := q.MarkRecycleBinRestored(ctx, entry.ID); err != nil {
			return wrap("标记回收站记录已恢复", entry.ID, err)
		}
		actorName, err := s.actorName(ctx, q, v.UserID)
		if err != nil {
			return err
		}
		if err := s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionRecycleRestore,
			Actor:      v,
			ActorName:  actorName,
			ObjectType: "library",
			ObjectID:   libraryID,
			Note:       fmt.Sprintf("恢复题库「%s」", lib.Name),
			IP:         ip,
		}); err != nil {
			return err
		}
		restored = Library{Entity: libraryFromRow(row), Role: domain.LibraryRoleOwner}
		return nil
	})
	if err != nil {
		return Library{}, err
	}
	return restored, nil
}

// PurgeLibrary 彻底删除回收站中的题库及其题目。仅所有者可执行。
func (s *Service) PurgeLibrary(ctx context.Context, v domain.Viewer, libraryID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return notFound("题库")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadDeletedLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapDeleteLibrary, "只有题库所有者可以彻底删除题库"); err != nil {
			return err
		}
		if _, err := q.GetLiveRecycleBinEntry(ctx, repo.GetLiveRecycleBinEntryParams{
			ObjectType: string(domain.RecycleObjectLibrary),
			ObjectID:   libraryID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该题库的回收站记录已结清")
			}
			return wrap("查询回收站记录", libraryID, err)
		}

		// 锁定题库行：并发恢复提交后删除语句不命中，恢复方胜出。
		row, err := q.LockLibraryIncludingDeleted(ctx, libraryID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该题库已被清理")
			}
			return wrap("锁定题库", libraryID, err)
		}
		if !row.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "该题库已被恢复")
		}

		// 先结清库内题目的回收站记录，再删库：题目随库级联删除后，
		// 这些记录已无对象可指向。
		if err := q.MarkLibraryRecycleBinPurged(ctx, libraryID); err != nil {
			return wrap("结清题库内题目回收站记录", libraryID, err)
		}
		deleted, err := q.HardDeleteLibrary(ctx, libraryID)
		if err != nil {
			return wrap("彻底删除题库", libraryID, err)
		}
		if deleted == 0 {
			// 行锁持有期间恢复无法提交，此处仅在并发清理时出现。
			return apperr.New(apperr.CodeInvalidState, "该题库已被清理")
		}
		if err := q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
			ObjectType: string(domain.RecycleObjectLibrary),
			ObjectID:   libraryID,
		}); err != nil {
			return wrap("标记回收站记录已清除", libraryID, err)
		}
		actorName, err := s.actorName(ctx, q, v.UserID)
		if err != nil {
			return err
		}
		return s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionRecyclePurge,
			Actor:      v,
			ActorName:  actorName,
			ObjectType: "library",
			ObjectID:   libraryID,
			Before:     librarySnapshot(lib),
			Note:       "彻底删除题库",
			IP:         ip,
		})
	})
}

// ListMyLibraryRecycleBin 返回当前用户回收站中的题库（需求 §11.3）。
func (s *Service) ListMyLibraryRecycleBin(ctx context.Context, v domain.Viewer) ([]LibraryRecycleEntry, error) {
	if err := s.ensure(); err != nil {
		return nil, notFound("题库")
	}
	if !v.Logged {
		return nil, apperr.New(apperr.CodeUnauthorized, "")
	}

	q := repo.New(s.pool)
	rows, err := q.ListOwnedLibraryRecycleBin(ctx, v.UserID)
	if err != nil {
		return nil, wrap("查询题库回收站", v.UserID, err)
	}
	out := make([]LibraryRecycleEntry, 0, len(rows))
	for _, row := range rows {
		item := LibraryRecycleEntry{LibraryID: row.ObjectID}
		if row.OwnerID.Valid {
			item.OwnerID = row.OwnerID.Int64
		}
		if row.DeletedBy.Valid {
			item.DeletedBy = row.DeletedBy.Int64
		}
		if row.DeletedAt.Valid {
			item.DeletedAt = row.DeletedAt.Time
		}
		if row.ExpiresAt.Valid {
			item.ExpiresAt = row.ExpiresAt.Time
		}
		item.Name, item.Description = snapshotName(row.Snapshot)
		out = append(out, item)
	}
	return out, nil
}

// PurgeExpired 清理到期未恢复的回收站记录（需求 §11.3：到期自动彻底删除）。
//
// 只处理题库与题库题目；提示词的彻底删除由管理后台的回收站模块执行，
// 因为提示词的清除需要同时处理评分、统计与转移记录。
func (s *Service) PurgeExpired(ctx context.Context, limit int32) (int, error) {
	if err := s.ensure(); err != nil {
		return 0, errDepsIncomplete
	}

	entries, err := repo.New(s.pool).ListExpiredRecycleBinEntries(ctx, limit)
	if err != nil {
		return 0, wrap("查询到期回收站记录", 0, err)
	}
	purged := 0
	for _, entry := range entries {
		objectType, ok := domain.ParseRecycleBinObjectType(entry.ObjectType)
		if !ok {
			return purged, unexpectedValue("回收站对象类型", entry.ID, entry.ObjectType)
		}
		if err := s.purgeExpiredEntry(ctx, objectType, entry); err != nil {
			return purged, err
		}
		purged++
	}
	return purged, nil
}

// purgeExpiredEntry 清理单条到期记录。
func (s *Service) purgeExpiredEntry(ctx context.Context, objectType domain.RecycleBinObjectType, entry repo.RecycleBin) error {
	switch objectType {
	case domain.RecycleObjectLibraryQuestion:
		return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
			ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
				ID:             entry.ObjectID,
				IncludeDeleted: true,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
						ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
					})
				}
				return wrap("查询到期题目", entry.ObjectID, err)
			}
			deleted, err := q.HardDeleteLibraryQuestion(ctx, entry.ObjectID)
			if err != nil {
				return wrap("清理到期题目", entry.ObjectID, err)
			}
			if deleted == 0 {
				// 并发恢复已提交：恢复方已标记回收站记录，此处跳过。
				return nil
			}
			if ref.Kind == string(domain.QuestionReference) {
				if err := q.DecrementLibraryQuestionCount(ctx, ref.LibraryID); err != nil {
					return wrap("归还题库题目计数", ref.LibraryID, err)
				}
			}
			return q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
				ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
			})
		})
	case domain.RecycleObjectLibrary:
		return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
			row, err := q.LockLibraryIncludingDeleted(ctx, entry.ObjectID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					// 题库行已被其他路径删除，回收站记录可以直接结清。
					return q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
						ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
					})
				}
				return wrap("锁定到期题库", entry.ObjectID, err)
			}
			if !row.DeletedAt.Valid {
				// 并发恢复已提交：恢复方已标记回收站记录，此处跳过。
				return nil
			}
			if err := q.MarkLibraryRecycleBinPurged(ctx, entry.ObjectID); err != nil {
				return wrap("结清题库内题目回收站记录", entry.ObjectID, err)
			}
			deleted, err := q.HardDeleteLibrary(ctx, entry.ObjectID)
			if err != nil {
				return wrap("清理到期题库", entry.ObjectID, err)
			}
			if deleted == 0 {
				// 行锁持有期间恢复无法提交，此处仅在并发清理时出现。
				return nil
			}
			return q.MarkRecycleBinPurged(ctx, repo.MarkRecycleBinPurgedParams{
				ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
			})
		})
	case domain.RecycleObjectPrompt:
		// 提示词的到期清理归管理后台的回收站模块，此处不作处理。
		return nil
	default:
		return unexpectedValue("回收站对象类型", entry.ID, entry.ObjectType)
	}
}

// entryToRecycleBin 写入一条回收站记录。到期时刻由配置的保留天数决定（需求 §11.3）。
func (s *Service) entryToRecycleBin(ctx context.Context, q *repo.Queries, entry recycleEntry) error {
	raw, err := encodeJSON(entry.Snapshot)
	if err != nil {
		return err
	}
	deletedAt := s.now().UTC()
	expires := domain.RecycleBinExpiry(deletedAt, s.retentionDays(ctx))
	if _, err := q.InsertRecycleBinEntry(ctx, repo.InsertRecycleBinEntryParams{
		ObjectType: string(entry.ObjectType),
		ObjectID:   entry.ObjectID,
		OwnerID:    int8OrNull(&entry.OwnerID),
		LibraryID:  int8OrNull(entry.LibraryID),
		Snapshot:   raw,
		DeletedBy:  int8OrNull(&entry.DeletedBy),
		ExpiresAt:  repo.Timestamptz(expires),
	}); err != nil {
		return wrap("写入回收站记录", entry.ObjectID, err)
	}
	return nil
}

// loadDeletedLibrary 读取回收站中的题库并构造判定主体。
func (s *Service) loadDeletedLibrary(
	ctx context.Context, q *repo.Queries, v domain.Viewer, libraryID int64,
) (domain.Library, domain.LibraryActor, error) {
	row, err := q.GetLibraryIncludingDeleted(ctx, libraryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Library{}, domain.LibraryActor{}, notFound("题库")
		}
		return domain.Library{}, domain.LibraryActor{}, wrap("查询题库", libraryID, err)
	}
	if !row.DeletedAt.Valid {
		return domain.Library{}, domain.LibraryActor{}, apperr.New(apperr.CodeInvalidState, "该题库不在回收站中")
	}
	lib := libraryFromRow(row)

	var member *domain.LibraryMember
	if v.Logged {
		memberRow, err := q.GetLibraryMember(ctx, repo.GetLibraryMemberParams{
			LibraryID: libraryID,
			UserID:    v.UserID,
		})
		switch {
		case err == nil:
			parsed, err := memberFromRow(memberRow)
			if err != nil {
				return domain.Library{}, domain.LibraryActor{}, err
			}
			member = &parsed
		case !errors.Is(err, pgx.ErrNoRows):
			return domain.Library{}, domain.LibraryActor{}, wrap("查询题库成员", libraryID, err)
		}
	}
	actor := domain.ActorFor(v, member)
	if !actor.Can(domain.CapViewLibrary) {
		return domain.Library{}, domain.LibraryActor{}, notFound("题库")
	}
	return lib, actor, nil
}

// snapshotName 从回收站快照中取出名称与描述。
func snapshotName(raw []byte) (string, string) {
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if len(raw) == 0 {
		return "", ""
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	return payload.Name, payload.Description
}
