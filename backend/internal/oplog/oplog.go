// Package oplog 写入操作日志与提示词转移日志。
// 操作日志只增不改不删（C-43），因此这里只提供写入。
package oplog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/promptbench/backend/internal/repo"
)

// 操作类型。取值必须与 operation_logs.action 的 CHECK 约束一致。
const (
	ActionAdminLogin          = "admin_login"
	ActionUserBan             = "user_ban"
	ActionUserUnban           = "user_unban"
	ActionUserTransferPrompts = "user_transfer_prompts"
	ActionDeveloperGrant      = "developer_grant"
	ActionDeveloperRevoke     = "developer_revoke"
	ActionAPITokenReset       = "api_token_reset"

	// 社交与协作域的审计动作。初版评论与收藏夹为直接删除，
	// 因此这两条日志是删除行为的唯一留痕（需求 §11.2）。
	ActionCommentDelete  = "comment_delete"
	ActionFavoriteDelete = "favorite_delete"

	// 题库域的审计动作。删除题目是需求 §17.8 列明的 18 类操作之一，
	// 恢复与彻底删除取自回收站操作（需求 §11.3）。
	ActionLibraryQuestionDelete  = "library_question_delete"
	ActionLibraryQuestionRestore = "library_question_restore"
	ActionRecycleRestore         = "recycle_restore"
	ActionRecyclePurge           = "recycle_purge"

	// 提示词域的审计动作（需求 §17.2、§17.8）。
	ActionPromptDelete    = "prompt_delete"
	ActionPromptRestore   = "prompt_restore"
	ActionPromptPurge     = "prompt_purge"
	ActionPromptTagUpdate = "prompt_tag_update"
	ActionPromptMajorMark = "prompt_major_mark"

	// 推荐标签的处置动作（需求 §6.3、§17.5）。
	ActionRecommendTagConfirm = "recommend_tag_confirm"
	ActionRecommendTagIgnore  = "recommend_tag_ignore"

	// 注册与邀请码（需求 §17.7）。
	ActionRegistrationPolicyUpdate = "registration_policy_update"
	ActionInviteCodeGenerate       = "invite_code_generate"

	// 模型管理（需求 §13.3、§13.4）。
	ActionModelImport    = "model_import"
	ActionModelOverwrite = "model_overwrite"

	// 举报与公告（需求 §17.6、§2.4）。
	ActionReportHandle         = "report_handle"
	ActionAnnouncementPublish  = "announcement_publish"
	ActionAnnouncementWithdraw = "announcement_withdraw"

	// 系统设置（需求 §20）。
	ActionConfigUpdate = "config_update"
	ActionEmailTest    = "email_test"

	// 统计与评分域的审计动作（需求 §4.3、§3.4）。
	ActionRatingUpdate           = "rating_update"
	ActionRatingDelete           = "rating_delete"
	ActionDiscriminationRecovery = "discrimination_recovery"

	// 阶段 7 的审计动作（G-09、G-12）。
	ActionSensitiveContentAccess = "sensitive_content_access"
	ActionAccountDelete          = "account_delete"

	// 模型结果展示区的处置动作：管理员删除违规图片或模型输出（需求 §3.4 附则）。
	ActionShowcaseImageDelete  = "showcase_image_delete"
	ActionShowcaseOutputDelete = "showcase_output_delete"
)

// AllActions 返回 operation_logs.action 的全部合法取值，供筛选参数校验使用。
func AllActions() []string {
	return []string{
		ActionAdminLogin, ActionPromptDelete, ActionPromptRestore, ActionPromptPurge,
		ActionPromptTagUpdate, ActionPromptMajorMark,
		ActionRecommendTagConfirm, ActionRecommendTagIgnore,
		ActionUserBan, ActionUserUnban, ActionUserTransferPrompts,
		ActionDeveloperGrant, ActionDeveloperRevoke,
		ActionRegistrationPolicyUpdate, ActionInviteCodeGenerate,
		ActionModelImport, ActionModelOverwrite,
		ActionReportHandle, ActionAnnouncementPublish, ActionAnnouncementWithdraw,
		ActionLibraryQuestionDelete, ActionLibraryQuestionRestore,
		ActionAPITokenReset, ActionConfigUpdate, ActionEmailTest,
		ActionRecycleRestore, ActionRecyclePurge,
		ActionRatingUpdate, ActionRatingDelete, ActionCommentDelete,
		ActionFavoriteDelete, ActionDiscriminationRecovery,
		ActionSensitiveContentAccess, ActionAccountDelete,
		ActionShowcaseImageDelete, ActionShowcaseOutputDelete,
	}
}

// RequiredActions 是需求 §17.8 逐条列出的操作类型（18 类），也是操作日志模块
// 筛选器的取值来源。取值必须与 operation_logs.action 的 CHECK 约束一致。
func RequiredActions() []string {
	return []string{
		ActionAdminLogin,
		ActionPromptDelete,
		ActionPromptRestore,
		ActionPromptPurge,
		ActionPromptTagUpdate,
		ActionRecommendTagConfirm,
		ActionRecommendTagIgnore,
		ActionUserBan,
		ActionUserUnban,
		ActionDeveloperGrant,
		ActionRegistrationPolicyUpdate,
		ActionInviteCodeGenerate,
		ActionModelImport,
		ActionModelOverwrite,
		ActionReportHandle,
		ActionAnnouncementPublish,
		ActionLibraryQuestionDelete,
		ActionAPITokenReset,
	}
}

// Entry 是一条待写入的操作日志。
type Entry struct {
	OperatorID   int64
	OperatorName string
	Action       string
	ObjectType   string
	ObjectID     int64
	Before       any
	After        any
	Note         string
	IP           string
}

// Recorder 写入操作日志。写入必须与业务变更处于同一事务，避免出现有业务无日志的状态。
type Recorder struct{}

func NewRecorder() *Recorder { return &Recorder{} }

func (r *Recorder) Record(ctx context.Context, q *repo.Queries, entry Entry) error {
	before, err := encode(entry.Before)
	if err != nil {
		return err
	}
	after, err := encode(entry.After)
	if err != nil {
		return err
	}

	_, err = q.InsertOperationLog(ctx, repo.InsertOperationLogParams{
		OperatorID:   repo.Int8OrNull(entry.OperatorID),
		OperatorName: entry.OperatorName,
		Action:       entry.Action,
		ObjectType:   entry.ObjectType,
		ObjectID:     repo.Int8OrNull(entry.ObjectID),
		BeforeData:   before,
		AfterData:    after,
		Note:         entry.Note,
		Ip:           entry.IP,
	})
	if err != nil {
		return fmt.Errorf("写入操作日志 %s: %w", entry.Action, err)
	}
	return nil
}

func encode(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("序列化日志快照: %w", err)
	}
	return raw, nil
}
