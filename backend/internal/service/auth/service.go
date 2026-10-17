// Package auth 承载注册、登录、会话、口令与凭据的用例编排。
package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

// VerificationCodeTTL 是邮箱验证码有效期。
const VerificationCodeTTL = 15 * time.Minute

// maxCodesPerHour 是同一邮箱同一用途每小时可请求的验证码数量上限。
const maxCodesPerHour = 5

// maxUsernameAttempts 是用户名后缀递增的尝试次数上限。
const maxUsernameAttempts = 20

type Deps struct {
	Pool   *pgxpool.Pool
	Config *config.Store
	Mail   mail.Sender
	Signer *security.Signer
	Logger *slog.Logger

	// GitHub 与 StateSigner、PublicBaseURL 共同支撑第三方登录；GitHub 为空表示未启用。
	GitHub        *oauth.Client
	StateSigner   *security.OAuthStateSigner
	PublicBaseURL string
}

type Service struct {
	pool   *pgxpool.Pool
	cfg    *config.Store
	mail   mail.Sender
	signer *security.Signer
	oplog  *oplog.Recorder
	logger *slog.Logger

	github        *oauth.Client
	stateSigner   *security.OAuthStateSigner
	publicBaseURL string

	now func() time.Time
}

func New(deps Deps) *Service {
	return &Service{
		pool:          deps.Pool,
		cfg:           deps.Config,
		mail:          deps.Mail,
		signer:        deps.Signer,
		oplog:         oplog.NewRecorder(),
		logger:        deps.Logger,
		github:        deps.GitHub,
		stateSigner:   deps.StateSigner,
		publicBaseURL: strings.TrimSuffix(deps.PublicBaseURL, "/"),
		now:           time.Now,
	}
}

// Session 是一次成功登录产生的会话凭据。
type Session struct {
	User      domain.User
	Token     string
	ExpiresAt time.Time
}

func (s *Service) issueSession(user domain.User) (Session, error) {
	token, err := s.signer.Sign(user.ID, user.TokenVersion)
	if err != nil {
		return Session{}, err
	}
	return Session{
		User:      user,
		Token:     token,
		ExpiresAt: s.now().Add(security.SessionTTL),
	}, nil
}

// SessionFor 为已完成身份确认的账户签发会话，用于邮箱验证码验证通过后直接登录：
// 持有验证码即证明对该邮箱的控制权。
func (s *Service) SessionFor(ctx context.Context, userID int64) (Session, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	if user.IsBanned() {
		return Session{}, apperr.New(apperr.CodeBanned, "")
	}
	return s.issueSession(user)
}

// ResolveSession 校验会话令牌并载入账户。角色、状态每次请求都从数据库读取，
// 使封禁与身份变更立即生效。
func (s *Service) ResolveSession(ctx context.Context, userID int64, tokenVersion int32) (domain.User, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if user.IsBanned() {
		return domain.User{}, apperr.New(apperr.CodeBanned, "")
	}
	if user.TokenVersion != tokenVersion {
		return domain.User{}, apperr.New(apperr.CodeUnauthorized, "")
	}
	return user, nil
}

// ResolveAPIToken 用 API Token 明文换出账户。
func (s *Service) ResolveAPIToken(ctx context.Context, plain string) (domain.User, error) {
	hash := security.HashAPIToken(plain)
	q := repo.New(s.pool)

	row, err := q.GetUserByAPITokenHash(ctx, hash)
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeUnauthorized, "")
	}
	user, err := repo.UserFromRow(row)
	if err != nil {
		return domain.User{}, err
	}
	if user.IsBanned() {
		return domain.User{}, apperr.New(apperr.CodeBanned, "")
	}
	if err := q.TouchAPIToken(ctx, hash); err != nil {
		return domain.User{}, fmt.Errorf("更新 API Token 使用时间: %w", err)
	}
	return user, nil
}

// Logout 使该账户的全部既有会话失效（G-13）。
func (s *Service) Logout(ctx context.Context, userID int64) error {
	if _, err := repo.New(s.pool).BumpUserTokenVersion(ctx, userID); err != nil {
		return fmt.Errorf("登出时提升令牌版本: %w", err)
	}
	return nil
}

func (s *Service) userByID(ctx context.Context, id int64) (domain.User, error) {
	row, err := repo.New(s.pool).GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeUnauthorized, "")
	}
	return repo.UserFromRow(row)
}

// registrationPolicy 读取注册策略配置（需求 §14.3 三态）。
func (s *Service) registrationPolicy(ctx context.Context) (domain.RegistrationPolicy, error) {
	raw, err := s.cfg.GetString(ctx, "registration_policy")
	if err != nil {
		return "", err
	}
	return domain.ParseRegistrationPolicy(raw)
}

// checkEmailAllowed 按白名单校验邮箱后缀；白名单为空表示不限制（C-35）。
func (s *Service) checkEmailAllowed(ctx context.Context, email string) error {
	whitelist, err := s.cfg.GetStringSlice(ctx, "email_domain_whitelist")
	if err != nil {
		return err
	}
	if len(whitelist) == 0 {
		return nil
	}

	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return apperr.New(apperr.CodeValidationFailed, "邮箱格式不正确")
	}
	domainPart := strings.ToLower(email[at+1:])
	for _, allowed := range whitelist {
		if domainPart == strings.ToLower(strings.TrimPrefix(allowed, "@")) {
			return nil
		}
	}
	return apperr.New(apperr.CodeEmailNotAllowed, "")
}

// issueVerificationCode 生成并发送验证码。发送成功后才落库（G-14）。
func (s *Service) issueVerificationCode(ctx context.Context, q *repo.Queries, userID int64, email string, purpose domain.VerificationPurpose) error {
	if err := s.enforceCodeRate(ctx, q, email, purpose); err != nil {
		return err
	}

	code, err := security.GenerateVerificationCode()
	if err != nil {
		return err
	}
	if err := s.mail.SendVerificationCode(ctx, email, code, purpose); err != nil {
		s.logger.Error("验证码邮件发送失败",
			slog.String("email", maskEmail(email)),
			slog.String("purpose", string(purpose)),
			slog.String("error", err.Error()),
		)
		return apperr.New(apperr.CodeMailSendFailed, "")
	}

	if err := q.SupersedeEmailVerifications(ctx, supersedeParams(email, purpose)); err != nil {
		return fmt.Errorf("作废旧验证码: %w", err)
	}

	if _, err := q.CreateEmailVerification(ctx, repo.CreateEmailVerificationParams{
		UserID:    repo.Int8OrNull(userID),
		Email:     email,
		Code:      code,
		Purpose:   string(purpose),
		ExpiresAt: repo.Timestamptz(s.now().Add(VerificationCodeTTL)),
	}); err != nil {
		return fmt.Errorf("保存验证码: %w", err)
	}
	return nil
}

func (s *Service) enforceCodeRate(ctx context.Context, q *repo.Queries, email string, purpose domain.VerificationPurpose) error {
	since := s.now().Add(-time.Hour)
	count, err := q.CountRecentEmailVerifications(ctx, repo.CountRecentEmailVerificationsParams{
		Lower:     email,
		Purpose:   string(purpose),
		CreatedAt: repo.Timestamptz(since),
	})
	if err != nil {
		return fmt.Errorf("统计近期验证码: %w", err)
	}
	if count >= maxCodesPerHour {
		return apperr.New(apperr.CodeRateLimited, "验证码请求过于频繁，请稍后再试")
	}
	return nil
}

// maxVerificationFailures 是同一验证码允许的最大错误次数（防分布式穷举）：
// 6 位数字验证码空间有限，只靠按地址限流挡不住多来源猜测，错误达到上限即作废。
const maxVerificationFailures = 5

// consumeVerificationCode 校验并一次性消费验证码。
// 校验失败按（邮箱、用途）累计失败次数，达到上限作废当前验证码：必须重新申请，
// 攻击者无法在 15 分钟有效期内对单个验证码无限穷举。
func (s *Service) consumeVerificationCode(ctx context.Context, q *repo.Queries, email string, purpose domain.VerificationPurpose, code string) error {
	record, err := q.GetActiveEmailVerification(ctx, repo.GetActiveEmailVerificationParams{
		Lower:   email,
		Purpose: string(purpose),
		Code:    strings.TrimSpace(code),
	})
	if err != nil {
		failed, failErr := q.RecordVerificationFailure(ctx, repo.RecordVerificationFailureParams{
			Lower:   email,
			Purpose: string(purpose),
		})
		if failErr == nil && failed.FailedAttempts >= maxVerificationFailures {
			if err := q.ConsumeEmailVerification(ctx, failed.ID); err != nil {
				return fmt.Errorf("作废验证码: %w", err)
			}
		}
		return apperr.New(apperr.CodeInvalidCode, "")
	}
	if record.FailedAttempts >= maxVerificationFailures {
		// 防御性兜底：计数达到上限的行本应在失败路径被作废。
		return apperr.New(apperr.CodeInvalidCode, "")
	}
	if err := q.ConsumeEmailVerification(ctx, record.ID); err != nil {
		return fmt.Errorf("消费验证码: %w", err)
	}
	return nil
}

// maskEmail 脱敏邮箱用于日志：保留首字符与域名，中间以 *** 代替。
func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***@" + email[at+1:]
}

// deriveUniqueUsername 由候选基名推导登录名，冲突时追加序号，仍冲突则追加随机后缀。
func deriveUniqueUsername(ctx context.Context, q *repo.Queries, base string) (string, error) {
	candidate := base
	for attempt := 2; attempt <= maxUsernameAttempts; attempt++ {
		exists, err := q.UsernameExists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("检查用户名: %w", err)
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, attempt)
	}

	suffix, err := security.GenerateOAuthState()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", base, suffix[:6]), nil
}

// usernameBase 由邮箱或第三方登录名推导登录名基名。
func usernameBase(source string) string {
	local := source
	if at := strings.Index(source, "@"); at >= 0 {
		local = source[:at]
	}

	var sb strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + ('a' - 'A'))
		}
	}

	base := strings.Trim(sb.String(), "._-")
	if base == "" {
		base = "user"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	return base
}

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func supersedeParams(email string, purpose domain.VerificationPurpose) repo.SupersedeEmailVerificationsParams {
	return repo.SupersedeEmailVerificationsParams{Lower: email, Purpose: string(purpose)}
}
