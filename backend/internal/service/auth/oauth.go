package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

// gitHubProvider 是 oauth_accounts.provider 的取值。
const gitHubProvider = "github"

// GitHubAuthorizeURL 生成授权跳转地址，并附带签名后的 state。
func (s *Service) GitHubAuthorizeURL(ctx context.Context) (string, error) {
	if err := s.requireGitHub(); err != nil {
		return "", err
	}

	nonce, err := security.GenerateOAuthState()
	if err != nil {
		return "", err
	}
	return s.github.AuthorizeURL(s.stateSigner.Sign(nonce), s.gitHubCallbackURL()), nil
}

// GitHubLandingURL 是授权完成或失败后浏览器应落在的前端地址。
func (s *Service) GitHubLandingURL() string {
	return s.publicBaseURL + "/oauth/github/callback"
}

// CompleteGitHubLogin 处理授权回调：校验 state、换取令牌、读取用户、
// 绑定或创建本地账户并签发会话。
func (s *Service) CompleteGitHubLogin(ctx context.Context, code, state string) (Session, error) {
	if err := s.requireGitHub(); err != nil {
		return Session{}, err
	}
	if err := s.stateSigner.Verify(state); err != nil {
		return Session{}, apperr.New(apperr.CodeInvalidCode, "OAuth 校验未通过")
	}

	accessToken, err := s.github.Exchange(ctx, code, s.gitHubCallbackURL())
	if err != nil {
		if errors.Is(err, oauth.ErrExchangeFailed) {
			return Session{}, apperr.New(apperr.CodeInvalidCode, "GitHub 授权码无效或已过期")
		}
		return Session{}, apperr.New(apperr.CodeBadGateway, "")
	}

	ghUser, err := s.github.FetchUser(ctx, accessToken)
	if err != nil {
		return Session{}, apperr.New(apperr.CodeBadGateway, "")
	}
	if ghUser.Email == "" {
		return Session{}, apperr.New(apperr.CodeValidationFailed, "GitHub 未返回可用邮箱")
	}

	email := normalizeEmail(ghUser.Email)
	if err := s.checkEmailAllowed(ctx, email); err != nil {
		return Session{}, err
	}

	q := repo.New(s.pool)

	bound, err := q.GetOAuthAccount(ctx, repo.GetOAuthAccountParams{
		Provider:       gitHubProvider,
		ProviderUserID: fmt.Sprintf("%d", ghUser.ID),
	})
	switch {
	case err == nil:
		return s.sessionForBoundAccount(ctx, bound.UserID)
	case !errors.Is(err, pgx.ErrNoRows):
		return Session{}, fmt.Errorf("查询第三方绑定: %w", err)
	}

	existing, err := q.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		return s.linkExistingAccount(ctx, existing, ghUser)
	case !errors.Is(err, pgx.ErrNoRows):
		return Session{}, fmt.Errorf("查询账户: %w", err)
	}

	return s.createOAuthAccount(ctx, email, ghUser)
}

func (s *Service) sessionForBoundAccount(ctx context.Context, userID int64) (Session, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	if user.IsBanned() {
		return Session{}, apperr.New(apperr.CodeBanned, "")
	}
	return s.issueSession(user)
}

// linkExistingAccount 把 GitHub 身份绑定到同邮箱的存量账户。
// 仅当 GitHub 侧邮箱已验证时绑定，避免用未验证邮箱冒领账户。
func (s *Service) linkExistingAccount(ctx context.Context, existing repo.User, ghUser oauth.GitHubUser) (Session, error) {
	if !ghUser.EmailVerified {
		return Session{}, apperr.New(apperr.CodeEmailTaken, "")
	}
	if existing.Status == string(domain.StatusBanned) {
		return Session{}, apperr.New(apperr.CodeBanned, "")
	}

	var linked repo.User
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := s.createOAuthBinding(ctx, q, existing.ID, ghUser); err != nil {
			return err
		}
		user := existing
		if existing.Status == string(domain.StatusUnverified) {
			verified, err := q.VerifyUserEmail(ctx, existing.ID)
			if err != nil {
				return fmt.Errorf("标记邮箱已验证: %w", err)
			}
			user = verified
		}
		linked = user
		return nil
	})
	if err != nil {
		return Session{}, err
	}

	user, err := repo.UserFromRow(linked)
	if err != nil {
		return Session{}, err
	}
	return s.issueSession(user)
}

// createOAuthAccount 为未注册过的 GitHub 用户建号。
// 关闭或仅邀请码注册时拒绝新用户注册，已绑定用户的登录不受影响（C-37）。
func (s *Service) createOAuthAccount(ctx context.Context, email string, ghUser oauth.GitHubUser) (Session, error) {
	policy, err := s.registrationPolicy(ctx)
	if err != nil {
		return Session{}, err
	}
	if !policy.AllowsPublicSignup() {
		return Session{}, apperr.New(apperr.CodeRegistrationClosed, "")
	}

	status := domain.StatusUnverified
	var verificationCode string
	if ghUser.EmailVerified {
		status = domain.StatusNormal
	} else {
		// 未验证邮箱的账户进入未验证态，并走正常邮箱验证流程（C-36）。
		verificationCode, err = security.GenerateVerificationCode()
		if err != nil {
			return Session{}, err
		}
		if err := s.mail.SendVerificationCode(ctx, email, verificationCode, domain.PurposeRegister); err != nil {
			s.logger.Error("GitHub 注册验证码邮件发送失败", slog.String("error", err.Error()))
			return Session{}, apperr.New(apperr.CodeMailSendFailed, "")
		}
	}

	base := ghUser.Login
	if base == "" {
		base = email
	}

	var created repo.User
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		username, err := deriveUniqueUsername(ctx, q, usernameBase(base))
		if err != nil {
			return err
		}

		var verifiedAt pgtype.Timestamptz
		if status == domain.StatusNormal {
			verifiedAt = repo.Timestamptz(s.now())
		}

		user, err := q.CreateUser(ctx, repo.CreateUserParams{
			Username:        username,
			Email:           email,
			PasswordHash:    repo.TextOrNull(""),
			Role:            string(domain.RoleUser),
			Status:          string(status),
			EmailVerifiedAt: verifiedAt,
		})
		if err != nil {
			return fmt.Errorf("创建账户: %w", err)
		}
		if err := s.createOAuthBinding(ctx, q, user.ID, ghUser); err != nil {
			return err
		}
		if verificationCode != "" {
			if err := q.SupersedeEmailVerifications(ctx, supersedeParams(email, domain.PurposeRegister)); err != nil {
				return fmt.Errorf("作废旧验证码: %w", err)
			}
			if _, err := q.CreateEmailVerification(ctx, repo.CreateEmailVerificationParams{
				UserID:    repo.Int8(user.ID),
				Email:     email,
				Code:      verificationCode,
				Purpose:   string(domain.PurposeRegister),
				ExpiresAt: repo.Timestamptz(s.now().Add(VerificationCodeTTL)),
			}); err != nil {
				return fmt.Errorf("保存验证码: %w", err)
			}
		}
		created = user
		return nil
	})
	if err != nil {
		return Session{}, err
	}

	user, err := repo.UserFromRow(created)
	if err != nil {
		return Session{}, err
	}
	return s.issueSession(user)
}

func (s *Service) createOAuthBinding(ctx context.Context, q *repo.Queries, userID int64, ghUser oauth.GitHubUser) error {
	if _, err := q.CreateOAuthAccount(ctx, repo.CreateOAuthAccountParams{
		UserID:         userID,
		Provider:       gitHubProvider,
		ProviderUserID: fmt.Sprintf("%d", ghUser.ID),
		ProviderLogin:  ghUser.Login,
	}); err != nil {
		return fmt.Errorf("创建第三方绑定: %w", err)
	}
	return nil
}

func (s *Service) requireGitHub() error {
	if s.github == nil || !s.github.Configured() {
		return apperr.New(apperr.CodeOAuthNotConfigured, "")
	}
	return nil
}

func (s *Service) gitHubCallbackURL() string {
	return s.publicBaseURL + "/api/v1/auth/oauth/github/callback"
}
