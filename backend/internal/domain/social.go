package domain

import "fmt"

// FavoriteVisibility 是收藏夹的可见性（需求 §9.3）。只有公开与私有两档，
// 没有开发者专用档：收藏夹不是内容本身，不承载开发者专属信息。
type FavoriteVisibility string

const (
	FavoritePublic  FavoriteVisibility = "public"
	FavoritePrivate FavoriteVisibility = "private"
)

// ParseFavoriteVisibility 解析收藏夹可见性字符串。
func ParseFavoriteVisibility(raw string) (FavoriteVisibility, bool) {
	switch FavoriteVisibility(raw) {
	case FavoritePublic, FavoritePrivate:
		return FavoriteVisibility(raw), true
	default:
		return "", false
	}
}

// 收藏夹配额（需求 §9.2、可行性报告 §2.14）。条目上限对所有身份相同，
// 收藏夹数量按身份区分。
const (
	// FavoriteItemLimit 是单个收藏夹最多容纳的提示词数量。
	FavoriteItemLimit = 300
	// FavoriteCountLimitUser 是普通用户最多可创建的收藏夹数量。
	FavoriteCountLimitUser = 20
	// FavoriteCountLimitDeveloper 是开发者用户最多可创建的收藏夹数量。
	FavoriteCountLimitDeveloper = 30
)

// Favorite 是收藏夹实体。
type Favorite struct {
	ID          int64
	UserID      int64
	Name        string
	Description string
	Visibility  FavoriteVisibility
	ItemCount   int32
}

// Access 返回收藏夹的可见性判定要素。
//
// HighRisk 恒为 false：高风险是提示词类别的属性，不是收藏夹的属性。
// 收藏夹里被改为高风险的提示词在条目层逐条复查（需求 §9.3 与开发方案 §6.5），
// 而不是把整个收藏夹判为高风险。
func (f Favorite) Access() Access {
	return Access{Level: FavoriteAccessLevel(f.Visibility), HighRisk: false, UploaderID: f.UserID}
}

// FavoriteAccessLevel 把收藏夹可见性映射为通用的可见性档位。
//
// 公开收藏夹映射为 public，私有映射为 private，两者复用与提示词相同的谓词：
// 私有档位只对所有者与管理员可见（C-18 对后台可见性的一致处理）。
func FavoriteAccessLevel(v FavoriteVisibility) Visibility {
	if v == FavoritePublic {
		return VisibilityPublic
	}
	return VisibilityPrivate
}

// FavoriteCountLimit 返回给定身份最多可创建的收藏夹数量（需求 §9.2）。
func FavoriteCountLimit(isDeveloper bool) int {
	if isDeveloper {
		return FavoriteCountLimitDeveloper
	}
	return FavoriteCountLimitUser
}

// Follow 是关注关系（需求 §9.4）。
type Follow struct {
	FollowerID int64
	FolloweeID int64
}

// ValidateFollow 校验关注对象。自关注由数据库的 CHECK 约束兜底，
// 此处提前给出可读的错误，避免把约束违反当成服务端错误。
func ValidateFollow(followerID, followeeID int64) error {
	if followerID == followeeID {
		return fmt.Errorf("不能关注自己")
	}
	return nil
}

// Comment 是提示词评论（需求 §12.1）。ParentID 非空表示这是一条回复。
type Comment struct {
	ID        int64
	PromptID  int64
	UserID    int64
	ParentID  *int64
	Body      string
	LikeCount int32
}

// IsReply 判定该评论是否为回复。
func (c Comment) IsReply() bool { return c.ParentID != nil }

// ReportReason 是举报原因（C-31）。API 与数据库都使用英文枚举键，
// 中文文案保留在前端语言包，使英文界面能独立本地化。
type ReportReason string

const (
	ReportBadContent ReportReason = "bad_content"
	ReportIllegal    ReportReason = "illegal"
	ReportPorn       ReportReason = "porn"
	ReportAttack     ReportReason = "attack"
	ReportPrivacy    ReportReason = "privacy"
	ReportSpam       ReportReason = "spam"
	ReportDangerous  ReportReason = "dangerous"
	ReportOther      ReportReason = "other"
)

// AllReportReasons 返回全部举报原因，供参数校验与字典声明使用。
func AllReportReasons() []ReportReason {
	return []ReportReason{
		ReportBadContent, ReportIllegal, ReportPorn, ReportAttack,
		ReportPrivacy, ReportSpam, ReportDangerous, ReportOther,
	}
}

// ParseReportReason 解析举报原因，非法取值返回 false。
func ParseReportReason(raw string) (ReportReason, bool) {
	for _, r := range AllReportReasons() {
		if ReportReason(raw) == r {
			return r, true
		}
	}
	return "", false
}

// ReportObjectType 是被举报对象的类型（需求 §12.2）。
type ReportObjectType string

const (
	ReportObjectPrompt  ReportObjectType = "prompt"
	ReportObjectComment ReportObjectType = "comment"
)

// ParseReportObjectType 解析被举报对象类型。
func ParseReportObjectType(raw string) (ReportObjectType, bool) {
	switch ReportObjectType(raw) {
	case ReportObjectPrompt, ReportObjectComment:
		return ReportObjectType(raw), true
	default:
		return "", false
	}
}

// NotificationKind 是站内通知的类型（需求 §12.3）。
type NotificationKind string

const (
	NotifyAnnouncement           NotificationKind = "announcement"
	NotifyCommentReply           NotificationKind = "comment_reply"
	NotifyLibraryQuestionDeleted NotificationKind = "library_question_deleted"
	NotifySystem                 NotificationKind = "system"
)

// AllNotificationKinds 返回全部通知类型，供筛选参数校验使用。
func AllNotificationKinds() []NotificationKind {
	return []NotificationKind{
		NotifyAnnouncement, NotifyCommentReply, NotifyLibraryQuestionDeleted, NotifySystem,
	}
}

// ParseNotificationKind 解析通知类型，非法取值返回 false。
func ParseNotificationKind(raw string) (NotificationKind, bool) {
	for _, k := range AllNotificationKinds() {
		if NotificationKind(raw) == k {
			return k, true
		}
	}
	return "", false
}

// ValidateComment 校验评论文本。
func ValidateComment(body string) error {
	if len([]rune(body)) == 0 {
		return fmt.Errorf("评论内容不能为空")
	}
	if len([]rune(body)) > CommentMaxRunes {
		return fmt.Errorf("评论内容不能超过 %d 个字", CommentMaxRunes)
	}
	return nil
}

// CommentMaxRunes 是评论的字数上限。
const CommentMaxRunes = 2000

// ReportDetailMaxRunes 是举报补充说明的字数上限。
const ReportDetailMaxRunes = 500

// ValidateReportDetail 校验举报补充说明。
func ValidateReportDetail(detail string) error {
	if len([]rune(detail)) > ReportDetailMaxRunes {
		return fmt.Errorf("补充说明不能超过 %d 个字", ReportDetailMaxRunes)
	}
	return nil
}
