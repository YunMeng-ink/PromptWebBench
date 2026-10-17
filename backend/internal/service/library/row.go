package library

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// libraryFromRow 把数据库行映射为题库实体。
func libraryFromRow(row repo.Library) domain.Library {
	out := domain.Library{
		ID:            row.ID,
		OwnerID:       row.OwnerID,
		Name:          row.Name,
		Description:   row.Description,
		QuestionCount: row.QuestionCount,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	return out
}

// memberFromRow 把数据库行映射为成员实体。
func memberFromRow(row repo.LibraryMember) (domain.LibraryMember, error) {
	role, ok := domain.ParseLibraryRole(row.Role)
	if !ok {
		return domain.LibraryMember{}, unexpectedValue("题库成员", row.ID, row.Role)
	}
	return domain.LibraryMember{
		ID:        row.ID,
		LibraryID: row.LibraryID,
		UserID:    row.UserID,
		Role:      role,
		InvitedBy: int8Ptr(row.InvitedBy),
	}, nil
}

// questionFromRow 把题目行映射为服务层题目视图。
//
// 来源提示词仍然存在时，无论题目是引用还是派生，都带出它的可见性要素：
// 派生题的副本内容同样源自该提示词，不复查来源就会让副本成为绕开 C-20 的通道。
func questionFromRow(row repo.ListLibraryQuestionsRow, lib domain.Library) (Question, error) {
	kind, ok := domain.ParseLibraryQuestionKind(row.Kind)
	if !ok {
		return Question{}, unexpectedValue("题库题目", row.ID, row.Kind)
	}

	out := Question{
		ID:                  row.ID,
		LibraryID:           row.LibraryID,
		Library:             lib,
		Kind:                kind,
		SortOrder:           row.SortOrder,
		SourcePromptID:      int8Ptr(row.SourcePromptID),
		SourceVersionNo:     textValue(row.SourceVersionNo),
		SourceMajorModified: row.SourceMajorModified.Bool,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}

	var source *domain.Access
	if row.SourceVisibility.Valid {
		level, ok := domain.ParseVisibility(row.SourceVisibility.String)
		if !ok {
			return Question{}, unexpectedValue("来源提示词可见性", row.ID, row.SourceVisibility.String)
		}
		access := domain.Access{
			Level:      level,
			HighRisk:   row.SourceIsHighRisk,
			UploaderID: row.SourceUploaderID.Int64,
		}
		source = &access
		// 来源提示词已软删除时正文不可取，但仍保留可见性要素供派生题复查。
		out.SourceAvailable = !row.SourceDeletedAt.Valid
	}
	out.SourceAccess = source

	if kind == domain.QuestionReference {
		if source == nil || row.SourceDeletedAt.Valid {
			// 来源提示词已被删除：引用题没有正文可取。题目仍保留在库中并标注来源不可用，
			// 使成员可以删掉它并归还容量；来源提示词被恢复后，题目随之恢复可读。
			out.Title = domain.LibraryQuestionSourceMissing
			return out, nil
		}
		out.Title = textValue(row.SourceTitle)
		out.Body = textValue(row.SourceBody)
		out.Description = textValue(row.SourceDescription)
		out.PurposeID = int8Ptr(row.SourcePurposeID)
		out.CapabilityID = int8Ptr(row.SourceCapabilityID)
		out.DifficultyID = int8Ptr(row.SourceDifficultyID)
		out.PurposeNameZh = row.PurposeNameZh
		out.PurposeNameEn = row.PurposeNameEn
		out.CapabilityNameZh = row.CapabilityNameZh
		out.CapabilityNameEn = row.CapabilityNameEn
		level, ok := domain.ParseSensitiveLevel(textValue(row.SourceSensitiveLevel))
		if !ok {
			return Question{}, unexpectedValue("来源提示词敏感级别", row.ID, textValue(row.SourceSensitiveLevel))
		}
		out.Sensitive = level
		return out, nil
	}

	out.Title = textValue(row.Title)
	out.Body = textValue(row.Body)
	out.Description = row.Description
	out.PurposeID = int8Ptr(row.PurposeID)
	out.CapabilityID = int8Ptr(row.CapabilityID)
	out.DifficultyID = int8Ptr(row.DifficultyID)
	out.PurposeNameZh = row.PurposeNameZh
	out.PurposeNameEn = row.PurposeNameEn
	out.CapabilityNameZh = row.CapabilityNameZh
	out.CapabilityNameEn = row.CapabilityNameEn
	level, ok := domain.ParseSensitiveLevel(textValue(row.SensitiveLevel))
	if !ok {
		return Question{}, unexpectedValue("题库题目敏感级别", row.ID, textValue(row.SensitiveLevel))
	}
	out.Sensitive = level
	return out, nil
}

// promptAccessFromRow 由提示词行取出可见性判定要素。
//
// 判定规则本身只存在于 domain.Viewer；这里做的只是把数据库列整理成判定输入。
// 行类型由调用方展开为列传入：sqlc 为 GetPrompt 与 LockPrompt 生成同构但不同名的行类型。
func promptAccessFromRow(row repo.GetPromptRow) (domain.Access, error) {
	return promptAccessFromColumns(row.ID, row.Visibility, row.IsHighRisk, row.UploaderID)
}

// promptAccessFromColumns 是 promptAccessFromRow 的列展开形式，供锁定读取复用。
func promptAccessFromColumns(id int64, visibility string, isHighRisk bool, uploaderID int64) (domain.Access, error) {
	level, ok := domain.ParseVisibility(visibility)
	if !ok {
		return domain.Access{}, unexpectedValue("提示词可见性", id, visibility)
	}
	return domain.Access{
		Level:      level,
		HighRisk:   isHighRisk,
		UploaderID: uploaderID,
	}, nil
}

// oplogEntry 是一条待写入的操作日志。日志与业务变更同事务。
type oplogEntry struct {
	Action     string
	Actor      domain.Viewer
	ActorName  string
	ObjectType string
	ObjectID   int64
	Before     any
	After      any
	Note       string
	IP         string
}

// record 写入一条操作日志。
func (s *Service) record(ctx context.Context, q *repo.Queries, entry oplogEntry) error {
	return s.oplog.Record(ctx, q, oplog.Entry{
		OperatorID:   entry.Actor.UserID,
		OperatorName: entry.ActorName,
		Action:       entry.Action,
		ObjectType:   entry.ObjectType,
		ObjectID:     entry.ObjectID,
		Before:       entry.Before,
		After:        entry.After,
		Note:         entry.Note,
		IP:           entry.IP,
	})
}

// actorName 读取操作人的显示名，供操作日志与通知共用。
func (s *Service) actorName(ctx context.Context, q *repo.Queries, userID int64) (string, error) {
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", notFound("用户")
		}
		return "", wrap("查询操作人", userID, err)
	}
	return user.Username, nil
}

// questionSnapshot 是写入回收站与操作日志的删除前快照（需求 §10.6 第 5 项要素）。
func questionSnapshot(q Question, tags []string) map[string]any {
	snapshot := map[string]any{
		"id":              q.ID,
		"library_id":      q.LibraryID,
		"kind":            string(q.Kind),
		"title":           q.Title,
		"body":            q.Body,
		"description":     q.Description,
		"purpose_id":      q.PurposeID,
		"capability_id":   q.CapabilityID,
		"difficulty_id":   q.DifficultyID,
		"sensitive_level": string(q.Sensitive),
		"sort_order":      q.SortOrder,
		"tags":            tags,
	}
	if q.SourcePromptID != nil {
		snapshot["source_prompt_id"] = *q.SourcePromptID
	}
	if q.SourceVersionNo != "" {
		snapshot["source_version_no"] = q.SourceVersionNo
	}
	return snapshot
}

// librarySnapshot 是题库进入回收站时的快照。
func librarySnapshot(lib domain.Library) map[string]any {
	return map[string]any{
		"id":             lib.ID,
		"owner_id":       lib.OwnerID,
		"name":           lib.Name,
		"description":    lib.Description,
		"question_count": lib.QuestionCount,
	}
}
