package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// Export 是「我的数据」导出的内容（G-08）。
//
// 三份 JSON 数组各自独立成段：提示词、评分、评论。删除了的内容不在其中
// （软删记录对用户已不可见）；回收站中的提示词按已删除处理。
type Export struct {
	Username string `json:"username"`
	Filename string `json:"filename"`

	Prompts  []ExportPrompt  `json:"prompts"`
	Ratings  []ExportRating  `json:"ratings"`
	Comments []ExportComment `json:"comments"`
}

// ExportPrompt 是导出文件里的一条提示词。
type ExportPrompt struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	Description    string `json:"description"`
	Visibility     string `json:"visibility"`
	SensitiveLevel string `json:"sensitive_level"`
	VersionNo      string `json:"version_no"`
	CategoryNameZh string `json:"category_name_zh"`
	CategoryNameEn string `json:"category_name_en"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ExportRating 是导出文件里的一条评分。
type ExportRating struct {
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ModelName   string `json:"model_name"`
	Score       int16  `json:"score"`
	EditCount   int32  `json:"edit_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ExportComment 是导出文件里的一条评论。
type ExportComment struct {
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ParentID    *int64 `json:"parent_id,omitempty"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}

// ExportMyData 导出当前账户的全部个人数据（G-08）：提示词、评分、评论。
func (s *Service) ExportMyData(ctx context.Context, userID int64) (Export, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return Export{}, err
	}
	q := repo.New(s.pool)

	prompts, err := q.ExportMyPrompts(ctx, userID)
	if err != nil {
		return Export{}, fmt.Errorf("导出提示词: %w", err)
	}
	ratings, err := q.ExportMyRatings(ctx, userID)
	if err != nil {
		return Export{}, fmt.Errorf("导出评分: %w", err)
	}
	comments, err := q.ExportMyComments(ctx, userID)
	if err != nil {
		return Export{}, fmt.Errorf("导出评论: %w", err)
	}

	out := Export{
		Username: user.Username,
		Filename: fmt.Sprintf("prompt-benchmark-%s-%s.json", user.Username, s.now().UTC().Format("20060102")),
		Prompts:  make([]ExportPrompt, 0, len(prompts)),
		Ratings:  make([]ExportRating, 0, len(ratings)),
		Comments: make([]ExportComment, 0, len(comments)),
	}
	for _, row := range prompts {
		out.Prompts = append(out.Prompts, ExportPrompt{
			ID: row.ID, Title: row.Title, Body: row.Body, Description: row.Description,
			Visibility: row.Visibility, SensitiveLevel: row.SensitiveLevel,
			VersionNo:      row.LatestVersionNo,
			CategoryNameZh: row.CategoryNameZh, CategoryNameEn: row.CategoryNameEn,
			CreatedAt: row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt: row.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	for _, row := range ratings {
		out.Ratings = append(out.Ratings, ExportRating{
			ID: row.ID, PromptID: row.PromptID, PromptTitle: row.PromptTitle,
			ModelName: row.ModelName, Score: row.Score, EditCount: row.EditCount,
			CreatedAt: row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt: row.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	for _, row := range comments {
		out.Comments = append(out.Comments, ExportComment{
			ID: row.ID, PromptID: row.PromptID, PromptTitle: row.PromptTitle,
			ParentID: int8Ptr(row.ParentID), Body: row.Body,
			CreatedAt: row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

// DeleteAccount 注销当前账户（G-09）。
//
// 软删账户并匿名化：昵称与邮箱释放、凭据清空、会话失效；名下提示词转移到
// 系统账户「沧海遗粟」并保留原上传者（与封禁转移同一约定，C-25）；
// 收藏夹、题库与关注关系随账户停用。评分与评论作为派生数据保留，
// 作者归属指向已注销的账户行。
//
// 最后一个管理员不能注销自己：注销后无人能进入后台。
func (s *Service) DeleteAccount(ctx context.Context, userID int64, ip string) error {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.IsSystem {
		return apperr.New(apperr.CodeInvalidState, "系统账户不可注销")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if user.IsAdmin() {
			admins, err := q.CountActiveAdmins(ctx)
			if err != nil {
				return fmt.Errorf("统计管理员数量: %w", err)
			}
			if admins <= 1 {
				return apperr.New(apperr.CodeInvalidState, "最后一个管理员不能注销账户")
			}
		}

		system, err := q.GetSystemUser(ctx)
		if err != nil {
			return fmt.Errorf("读取系统账户: %w", err)
		}
		moved, err := q.TransferPrompts(ctx, repo.TransferPromptsParams{
			FromUploader: userID,
			ToUploader:   system.ID,
		})
		if err != nil {
			return fmt.Errorf("转移提示词: %w", err)
		}
		for _, promptID := range moved {
			if err := q.InsertTransferLog(ctx, repo.InsertTransferLogParams{
				PromptID:   promptID,
				FromUserID: userID,
				ToUserID:   system.ID,
				Reason:     "account_deleted",
				OperatorID: repo.Int8(userID),
			}); err != nil {
				return fmt.Errorf("记录提示词转移: %w", err)
			}
		}

		if err := q.SoftDeleteUserFavorites(ctx, userID); err != nil {
			return fmt.Errorf("停用收藏夹: %w", err)
		}
		if err := q.SoftDeleteUserLibraries(ctx, userID); err != nil {
			return fmt.Errorf("停用题库: %w", err)
		}
		if err := q.DeleteUserFollows(ctx, userID); err != nil {
			return fmt.Errorf("清理关注关系: %w", err)
		}
		if err := q.DeleteUserOAuthAccounts(ctx, userID); err != nil {
			return fmt.Errorf("解绑第三方账户: %w", err)
		}
		if err := q.DeleteUserEmailVerifications(ctx, repo.Int8(userID)); err != nil {
			return fmt.Errorf("清理验证码: %w", err)
		}

		if _, err := q.SoftDeleteUser(ctx, userID); err != nil {
			return fmt.Errorf("注销账户: %w", err)
		}

		return s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   userID,
			OperatorName: user.Username,
			Action:       oplog.ActionAccountDelete,
			ObjectType:   "user",
			ObjectID:     userID,
			Before: map[string]any{
				"username": user.Username,
				"email":    user.Email,
				"role":     string(user.Role),
			},
			After: map[string]any{"transferred_prompts": len(moved)},
			IP:    ip,
		})
	})
}

// int8Ptr 把可空父评论 ID 转成可选指针。
func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

// MarshalExport 把导出内容序列化为带缩进的 JSON。
func MarshalExport(out Export) ([]byte, error) {
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化导出内容: %w", err)
	}
	return raw, nil
}
