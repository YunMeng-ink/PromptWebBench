// Package domain 承载实体与业务规则，不依赖上层，也不做 I/O。
package domain

import (
	"fmt"
	"time"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleDeveloper Role = "developer"
	RoleAdmin     Role = "admin"
)

type UserStatus string

const (
	StatusNormal     UserStatus = "normal"
	StatusUnverified UserStatus = "unverified"
	StatusBanned     UserStatus = "banned"
)

// SystemAccountUsername 是承接被封禁用户提示词的默认账户（需求 §2.5）。
const SystemAccountUsername = "沧海遗粟"

// UsernameMaxRunes 是昵称的最大字符数。顶栏的用户胶囊宽度有限，
// 过长的昵称会把站点标题挤成省略号，因此昵称上限与邮箱前缀一起收紧到 20 个字符。
const UsernameMaxRunes = 20

// User 是账户实体。
type User struct {
	ID              int64
	Username        string
	Email           string
	PasswordHash    string
	HasPassword     bool
	Role            Role
	Status          UserStatus
	IsSystem        bool
	Bio             string
	AvatarURL       string
	TokenVersion    int32
	EmailVerifiedAt *time.Time
}

// IsAdmin 判定管理员。
func (r Role) IsAdmin() bool { return r == RoleAdmin }

// IsDeveloper 判定开发者权限。管理员隐含开发者权限（C-17）。
func (r Role) IsDeveloper() bool { return r == RoleDeveloper || r == RoleAdmin }

// AllowWrite 判定该身份组是否允许写入内容。
func (r Role) AllowWrite() bool { return r == RoleUser || r == RoleDeveloper || r == RoleAdmin }

// IsAdmin 判定管理员。
func (u User) IsAdmin() bool { return u.Role.IsAdmin() }

// IsDeveloper 判定开发者权限。管理员隐含开发者权限（C-17）。
func (u User) IsDeveloper() bool { return u.Role.IsDeveloper() }

func (u User) IsBanned() bool { return u.Status == StatusBanned }

// CanWriteContent 判定是否允许产生内容与凭据。
// 未验证账户可登录、可浏览，但不可评分、评论、上传、创建收藏夹或题库、生成 Token（C-23）。
func (u User) CanWriteContent() bool { return u.Status == StatusNormal }

func ParseRole(raw string) (Role, error) {
	switch Role(raw) {
	case RoleUser, RoleDeveloper, RoleAdmin:
		return Role(raw), nil
	default:
		return "", fmt.Errorf("未知角色 %q", raw)
	}
}

func ParseStatus(raw string) (UserStatus, error) {
	switch UserStatus(raw) {
	case StatusNormal, StatusUnverified, StatusBanned:
		return UserStatus(raw), nil
	default:
		return "", fmt.Errorf("未知账户状态 %q", raw)
	}
}
