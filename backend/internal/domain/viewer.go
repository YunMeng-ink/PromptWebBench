package domain

// Visibility 是提示词的可见性档位（需求 §8.1）。
type Visibility string

const (
	// VisibilityPublic 公开：游客与所有登录用户可见。
	VisibilityPublic Visibility = "public"
	// VisibilityPrivate 私有：仅上传者本人与管理员可见（C-18）。
	VisibilityPrivate Visibility = "private"
	// VisibilityDeveloper 开发者可见：仅开发者与管理员可见（需求 §8.4）。
	VisibilityDeveloper Visibility = "developer"
)

// SensitiveLevel 是提示词的敏感级别（需求 §8.5）。
type SensitiveLevel string

const (
	SensitiveLow    SensitiveLevel = "low"
	SensitiveMedium SensitiveLevel = "medium"
	SensitiveHigh   SensitiveLevel = "high"
)

// ParseVisibility 解析可见性字符串。
func ParseVisibility(raw string) (Visibility, bool) {
	switch Visibility(raw) {
	case VisibilityPublic, VisibilityPrivate, VisibilityDeveloper:
		return Visibility(raw), true
	default:
		return "", false
	}
}

// ParseSensitiveLevel 解析敏感级别字符串。
func ParseSensitiveLevel(raw string) (SensitiveLevel, bool) {
	switch SensitiveLevel(raw) {
	case SensitiveLow, SensitiveMedium, SensitiveHigh:
		return SensitiveLevel(raw), true
	default:
		return "", false
	}
}

// Viewable 是可判定可见性的对象。提示词以 Access 记录实现它；
// 题库、题库题目、收藏夹条目在阶段 4、5 接入同一组谓词。
type Viewable interface {
	// AccessLevel 返回可见性档位。
	AccessLevel() Visibility
	// AccessHighRisk 返回高风险标记（提示词取自类别）。
	AccessHighRisk() bool
	// AccessOwner 返回所有者 ID。
	AccessOwner() int64
}

// Access 实现 Viewable。
func (a Access) AccessLevel() Visibility { return a.Level }
func (a Access) AccessHighRisk() bool    { return a.HighRisk }
func (a Access) AccessOwner() int64      { return a.UploaderID }

// Viewer 是一次请求的身份，是全系统唯一的可见性判定入口（开发方案 §3.1）。
type Viewer struct {
	UserID int64
	Role   Role
	Logged bool

	// Writable 反映账户是否处于正常态。未验证与封禁账户都能浏览，
	// 但不能产生内容与凭据（C-23、C-24）。
	Writable bool
}

// Guest 构造游客身份。
func Guest() Viewer { return Viewer{} }

// ViewerFor 由用户实体构造身份。封禁账户不可登录、会话已失效（C-24），
// 其可见性判定退化为游客，不授予任何授权。
func ViewerFor(u User) Viewer {
	return Viewer{
		UserID:   u.ID,
		Role:     u.Role,
		Logged:   u.Status != StatusBanned,
		Writable: u.Status == StatusNormal,
	}
}

// CanWrite 内容写入权限（C-23）：账户须处于正常态。
func (v Viewer) CanWrite() bool {
	return v.Logged && v.Writable && v.Role.AllowWrite()
}

// CanUploadDeveloper 仅开发者与管理员可选择「开发者可见」（需求 §8.4、§8.7）。
func (v Viewer) CanUploadDeveloper() bool {
	return v.Logged && v.Role.IsDeveloper()
}

// CanUseHighRiskCategory 仅开发者与管理员可选择高风险类别（C-19、需求 §8.7）。
func (v Viewer) CanUseHighRiskCategory() bool {
	return v.Logged && v.Role.IsDeveloper()
}

// IsAdmin 判定管理员身份。
func (v Viewer) IsAdmin() bool { return v.Logged && v.Role.IsAdmin() }

// IsDeveloper 判定开发者身份，管理员隐含满足（C-17）。
func (v Viewer) IsDeveloper() bool { return v.Logged && v.Role.IsDeveloper() }

// CanView 单条对象判定。无权限一律判为不可见，配合接口层的 404 约定，
// 不通过状态码差异暴露对象存在性（开发方案 §5.3）。
//
// 高风险类别与「开发者可见」同等处理（C-20）：即便可见性为 public，
// 对普通用户与游客一律不可见。
func (v Viewer) CanView(o Viewable) bool {
	if o.AccessHighRisk() && !v.CanUseHighRiskCategory() {
		return false
	}
	switch o.AccessLevel() {
	case VisibilityPublic:
		return true
	case VisibilityPrivate:
		return v.Owns(o) || v.IsAdmin()
	case VisibilityDeveloper:
		return v.IsDeveloper()
	default:
		return false
	}
}

// Owns 判定是否为本对象的所有者。管理员不隐含所有权：
// 管理权限由 CanManage 单独授予，避免后台与前台规则互相渗透。
func (v Viewer) Owns(o Viewable) bool {
	return v.Logged && v.UserID == o.AccessOwner()
}

// CanManage 判定是否可修改或删除本对象：所有者或管理员（C-18）。
func (v Viewer) CanManage(o Viewable) bool {
	return v.Owns(o) || v.IsAdmin()
}

// ListScope 公开列表判定，返回 SQL 谓词与参数。
//
// 公开列表只收录 public 档位。开发者及以上不加高风险过滤（其本就能看到高风险内容）；
// 其余身份追加排除高风险类别，这是 C-20 的列表侧。
// 私有与开发者可见档位不进入公开列表，由详情与「我的提示词」分别处理。
func (v Viewer) ListScope() (string, []any) {
	if v.IsDeveloper() {
		return "p.visibility = 'public'", nil
	}
	return "p.visibility = 'public' AND COALESCE(c.is_high_risk, false) = false", nil
}

// OwnScope 上传者本人列表判定，供「我的提示词」使用：任意可见性与高风险标记，
// 只按归属过滤。
func (v Viewer) OwnScope() (string, []any) {
	if !v.Logged {
		return "false", nil
	}
	return "p.uploader_id = $1 AND p.deleted_at IS NULL", []any{v.UserID}
}

// PermissionError 是权限判定的失败原因。领域层不依赖 apperr，
// 由服务层把原因翻译成错误码，保证同一判定只存在于此处。
type PermissionError string

func (e PermissionError) Error() string { return string(e) }

const (
	// ErrHighRiskCategoryForbidden 普通用户选择高风险类别（C-19）。
	ErrHighRiskCategoryForbidden PermissionError = "high_risk_category_forbidden"
	// ErrDeveloperVisibilityForbidden 普通用户选择开发者可见（需求 §8.4）。
	ErrDeveloperVisibilityForbidden PermissionError = "developer_visibility_forbidden"
	// ErrNotLogged 未登录用户请求需要身份的操作。
	ErrNotLogged PermissionError = "not_logged"
)

// AuthorizeCategory 校验类别与可见性的组合。创建与更新两条路径共用此函数，
// 使「只能由开发者使用」的约束在两个入口上同时生效（开发方案 §3.3）。
func (v Viewer) AuthorizeCategory(categoryIsHighRisk bool, visibility Visibility) error {
	if categoryIsHighRisk && !v.CanUseHighRiskCategory() {
		return ErrHighRiskCategoryForbidden
	}
	if visibility == VisibilityDeveloper && !v.CanUploadDeveloper() {
		return ErrDeveloperVisibilityForbidden
	}
	return nil
}
