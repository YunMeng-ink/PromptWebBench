package domain

import "fmt"

// VerificationPurpose 是邮箱验证码的用途。
type VerificationPurpose string

const (
	PurposeRegister      VerificationPurpose = "register"
	PurposeResetPassword VerificationPurpose = "reset_password"
)

// ParseVerificationPurpose 校验用途取值。
func ParseVerificationPurpose(raw string) (VerificationPurpose, error) {
	switch VerificationPurpose(raw) {
	case PurposeRegister, PurposeResetPassword:
		return VerificationPurpose(raw), nil
	default:
		return "", fmt.Errorf("未知验证码用途 %q", raw)
	}
}

// RegistrationPolicy 是注册策略三态（需求 §14.3）。
type RegistrationPolicy string

const (
	RegistrationOpen       RegistrationPolicy = "open"
	RegistrationClosed     RegistrationPolicy = "closed"
	RegistrationInviteOnly RegistrationPolicy = "invite_only"
)

func ParseRegistrationPolicy(raw string) (RegistrationPolicy, error) {
	switch RegistrationPolicy(raw) {
	case RegistrationOpen, RegistrationClosed, RegistrationInviteOnly:
		return RegistrationPolicy(raw), nil
	default:
		return "", fmt.Errorf("未知注册策略 %q", raw)
	}
}

// AllowsPublicSignup 判定该策略是否允许不凭邀请码注册。
func (p RegistrationPolicy) AllowsPublicSignup() bool {
	return p == RegistrationOpen
}

// InviteKind 是邀请码类型（需求 §14.4）。
type InviteKind string

const (
	InviteNormal    InviteKind = "normal"
	InviteDeveloper InviteKind = "developer"
)

func ParseInviteKind(raw string) (InviteKind, error) {
	switch InviteKind(raw) {
	case InviteNormal, InviteDeveloper:
		return InviteKind(raw), nil
	default:
		return "", fmt.Errorf("未知邀请码类型 %q", raw)
	}
}

// RoleForInvite 返回邀请码对应的账户角色。
func (k InviteKind) RoleForInvite() Role {
	if k == InviteDeveloper {
		return RoleDeveloper
	}
	return RoleUser
}
