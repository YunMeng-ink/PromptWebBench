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
)

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
