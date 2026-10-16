package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// InviteInput 是一次邀请码生成的输入（需求 §14.5、§17.7）。
type InviteInput struct {
	// Kind 取 normal 或 developer：开发者邀请码注册后为开发者用户（需求 §14.4）。
	Kind string
	// Count 是一次生成的数量，上限见 domain.InviteBatchLimit。
	Count int32
	// MaxUses 是单个邀请码的最大使用次数。
	MaxUses int32
	// ExpiresInDays 为 0 表示永不过期。
	ExpiresInDays int32
	Note          string
}

// CreateInviteCodes 生成一批邀请码（需求 §17.7「生成普通邀请码、生成开发者邀请码、
// 批量生成邀请码、设置有效期、设置使用次数」）。
//
// 整批在一个事务内写入，任一环节失败都不留下半批邀请码。
func (s *Service) CreateInviteCodes(ctx context.Context, operator domain.User, in InviteInput, ip string) ([]repo.InviteCode, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	kind, err := domain.ParseInviteKind(in.Kind)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidationFailed, "邀请码类型取值非法")
	}
	if in.Count < 1 || in.Count > domain.InviteBatchLimit {
		return nil, apperr.New(apperr.CodeValidationFailed,
			fmt.Sprintf("单次生成数量必须在 1 到 %d 之间", domain.InviteBatchLimit))
	}
	if in.MaxUses < 1 || in.MaxUses > domain.InviteMaxUsesLimit {
		return nil, apperr.New(apperr.CodeValidationFailed,
			fmt.Sprintf("最大使用次数必须在 1 到 %d 之间", domain.InviteMaxUsesLimit))
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > domain.InviteExpiryDaysLimit {
		return nil, apperr.New(apperr.CodeValidationFailed,
			fmt.Sprintf("有效期天数必须在 0 到 %d 之间", domain.InviteExpiryDaysLimit))
	}
	note := trimSpace(in.Note)
	if len([]rune(note)) > domain.InviteNoteMaxRunes {
		return nil, apperr.New(apperr.CodeValidationFailed, "备注超出长度上限")
	}

	var expiry *time.Time
	if in.ExpiresInDays > 0 {
		value := s.now().UTC().AddDate(0, 0, int(in.ExpiresInDays))
		expiry = &value
	}

	codes := make([]repo.InviteCode, 0, in.Count)
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		codes = codes[:0]
		for i := int32(0); i < in.Count; i++ {
			code, err := generateInviteCode()
			if err != nil {
				return err
			}
			row, err := q.InsertInviteCode(ctx, repo.InsertInviteCodeParams{
				Code:      code,
				Kind:      string(kind),
				MaxUses:   in.MaxUses,
				Note:      note,
				CreatedBy: repo.Int8(operator.ID),
				ExpiresAt: repo.TimestamptzOrNull(expiry),
			})
			if err != nil {
				return wrap("写入邀请码", 0, err)
			}
			codes = append(codes, row)
		}
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionInviteCodeGenerate,
			ObjectType: "invite_code",
			After: map[string]any{
				"kind":            string(kind),
				"count":           in.Count,
				"max_uses":        in.MaxUses,
				"expires_in_days": in.ExpiresInDays,
			},
			Note: note,
			IP:   ip,
		})
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// ListInviteCodes 返回邀请码列表（需求 §17.7）。disabled 取 ""、"true"、"false"。
func (s *Service) ListInviteCodes(ctx context.Context, kind, disabled string, page, pageSize int32) (Page[repo.AdminListInviteCodesRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListInviteCodesRow]{}, err
	}
	if kind != "" {
		if _, err := domain.ParseInviteKind(kind); err != nil {
			return Page[repo.AdminListInviteCodesRow]{}, apperr.New(apperr.CodeValidationFailed, "邀请码类型取值非法")
		}
	}
	if disabled != "" && disabled != "true" && disabled != "false" {
		return Page[repo.AdminListInviteCodesRow]{}, apperr.New(apperr.CodeValidationFailed, "禁用状态取值非法")
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListInviteCodes(ctx, repo.AdminListInviteCodesParams{
		Kind: kind, Disabled: disabled,
		LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListInviteCodesRow]{}, fmt.Errorf("查询邀请码列表: %w", err)
	}
	total, err := q.AdminCountInviteCodes(ctx, repo.AdminCountInviteCodesParams{Kind: kind, Disabled: disabled})
	if err != nil {
		return Page[repo.AdminListInviteCodesRow]{}, fmt.Errorf("统计邀请码数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// SetInviteCodeDisabled 禁用或重新启用邀请码（需求 §17.7「禁用邀请码」）。
func (s *Service) SetInviteCodeDisabled(ctx context.Context, id int64, disabled bool) (repo.InviteCode, error) {
	if err := s.ensure(); err != nil {
		return repo.InviteCode{}, err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetInviteCode(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.InviteCode{}, notFound("邀请码")
		}
		return repo.InviteCode{}, wrap("查询邀请码", id, err)
	}
	row, err := q.SetInviteCodeDisabled(ctx, repo.SetInviteCodeDisabledParams{ID: id, Disabled: disabled})
	if err != nil {
		return repo.InviteCode{}, wrap("更新邀请码", id, err)
	}
	return row, nil
}

// ListInviteCodeUsages 返回邀请码的使用记录（需求 §14.5「使用记录」）。
func (s *Service) ListInviteCodeUsages(ctx context.Context, id int64, page, pageSize int32) (Page[repo.AdminListInviteCodeUsagesRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListInviteCodeUsagesRow]{}, err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetInviteCode(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Page[repo.AdminListInviteCodeUsagesRow]{}, notFound("邀请码")
		}
		return Page[repo.AdminListInviteCodeUsagesRow]{}, wrap("查询邀请码", id, err)
	}
	rows, err := q.AdminListInviteCodeUsages(ctx, repo.AdminListInviteCodeUsagesParams{
		InviteCodeID: id, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListInviteCodeUsagesRow]{}, fmt.Errorf("查询邀请码使用记录: %w", err)
	}
	total, err := q.AdminCountInviteCodeUsages(ctx, id)
	if err != nil {
		return Page[repo.AdminListInviteCodeUsagesRow]{}, fmt.Errorf("统计使用记录: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// RegistrationPolicy 读取当前注册策略（需求 §14.3）。
func (s *Service) RegistrationPolicy(ctx context.Context) (domain.RegistrationPolicy, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	raw, err := s.config.GetString(ctx, "registration_policy")
	if err != nil {
		return "", fmt.Errorf("读取注册策略配置: %w", err)
	}
	policy, err := domain.ParseRegistrationPolicy(raw)
	if err != nil {
		return "", fmt.Errorf("注册策略配置取值非法: %w", err)
	}
	return policy, nil
}

// inviteCodeRandomBytes 是邀请码的随机字节数。
const inviteCodeRandomBytes = 10

// generateInviteCode 生成邀请码明文。邀请码是一次性凭据而非长期密钥，
// 库中直接存储明文以便管理员与用户核对，随机量取 10 字节（80 位）足以抵御枚举。
func generateInviteCode() (string, error) {
	buf := make([]byte, inviteCodeRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成邀请码随机数: %w", err)
	}
	return "PB-" + strings.ToUpper(hex.EncodeToString(buf)), nil
}
