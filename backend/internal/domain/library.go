package domain

import (
	"fmt"
	"time"
)

// LibraryRole 是题库角色（需求 §10.3）。私有题库只有所有者与协作者两种角色，
// 创建者即所有者。
type LibraryRole string

const (
	LibraryRoleOwner        LibraryRole = "owner"
	LibraryRoleCollaborator LibraryRole = "collaborator"
)

// ParseLibraryRole 解析题库角色字符串。
func ParseLibraryRole(raw string) (LibraryRole, bool) {
	switch LibraryRole(raw) {
	case LibraryRoleOwner, LibraryRoleCollaborator:
		return LibraryRole(raw), true
	default:
		return "", false
	}
}

// 题库配额（需求 §10.2、可行性报告 §2.14）。题目容量对所有身份相同。
const (
	// LibraryQuotaUser 是普通用户最多可创建的私有题库数量。
	LibraryQuotaUser = 1
	// LibraryQuotaDeveloper 是开发者用户最多可创建的私有题库数量。
	LibraryQuotaDeveloper = 3
	// LibraryQuestionLimit 是单个私有题库的题目容量上限。
	// 计入回收站中的引用题，不计入派生题（C-22）。
	LibraryQuestionLimit = 150
)

// LibraryQuota 返回给定身份最多可创建的私有题库数量（需求 §10.2）。
func LibraryQuota(isDeveloper bool) int {
	if isDeveloper {
		return LibraryQuotaDeveloper
	}
	return LibraryQuotaUser
}

// LibraryCapability 是题库内的一项操作能力。
//
// 需求 §10.4 与 §10.5 以两份清单给出所有者与协作者的权限，C-46 规定以这两段清单
// 为准。两份清单被翻译成本枚举，使「谁能对题库做什么」在全系统只出现一次
// （开发方案 §3.1）。查看题库题目归入 CapViewLibrary：题目不属于题库之外的对象。
type LibraryCapability string

const (
	// CapViewLibrary 查看题库及其题目。
	CapViewLibrary LibraryCapability = "view_library"
	// CapViewRecycleBin 查看题库内回收站题目，是恢复与彻底删除的前置。
	CapViewRecycleBin LibraryCapability = "view_recycle_bin"
	// CapUpdateName 修改题库名称。
	CapUpdateName LibraryCapability = "update_name"
	// CapUpdateDescription 修改题库描述。
	CapUpdateDescription LibraryCapability = "update_description"
	// CapDeleteLibrary 删除整个题库。
	CapDeleteLibrary LibraryCapability = "delete_library"
	// CapInviteMember 邀请协作者。
	CapInviteMember LibraryCapability = "invite_member"
	// CapRemoveMember 移除协作者。
	CapRemoveMember LibraryCapability = "remove_member"
	// CapAddQuestion 添加题目。
	CapAddQuestion LibraryCapability = "add_question"
	// CapDeleteQuestion 删除任何题目。
	CapDeleteQuestion LibraryCapability = "delete_question"
	// CapUpdateQuestion 修改题目内容。
	CapUpdateQuestion LibraryCapability = "update_question"
	// CapTagQuestion 给题目贴标签。
	CapTagQuestion LibraryCapability = "tag_question"
	// CapUntagQuestion 移除题目标签。
	CapUntagQuestion LibraryCapability = "untag_question"
	// CapReorderQuestion 调整题目顺序。
	CapReorderQuestion LibraryCapability = "reorder_question"
	// CapExportLibrary 导出题库。
	CapExportLibrary LibraryCapability = "export_library"
	// CapRestoreQuestion 恢复题库内回收站题目。
	CapRestoreQuestion LibraryCapability = "restore_question"
	// CapPurgeQuestion 彻底删除回收站题目。
	CapPurgeQuestion LibraryCapability = "purge_question"
	// CapTransferOwnership 转移题库所有权。
	CapTransferOwnership LibraryCapability = "transfer_ownership"
	// CapUpdatePermissions 修改题库基础权限设置。
	CapUpdatePermissions LibraryCapability = "update_permissions"
)

// AllLibraryCapabilities 返回全部题库能力。所有者一览为真（需求 §10.4 的 15 项
// 加上由 §10.5 反推的转移所有权与修改基础权限设置，以及回收站查看）。
func AllLibraryCapabilities() []LibraryCapability {
	return []LibraryCapability{
		CapViewLibrary, CapViewRecycleBin, CapUpdateName, CapUpdateDescription,
		CapDeleteLibrary, CapInviteMember, CapRemoveMember, CapAddQuestion,
		CapDeleteQuestion, CapUpdateQuestion, CapTagQuestion, CapUntagQuestion,
		CapReorderQuestion, CapExportLibrary, CapRestoreQuestion, CapPurgeQuestion,
		CapTransferOwnership, CapUpdatePermissions,
	}
}

// collaboratorCapabilities 是协作者可做的 11 项（需求 §10.5 上半清单）。
//
// 清单为 10 条：查看题库、修改题库描述、添加题目、删除任何题目、修改题目内容、
// 给题目贴标签、移除题目标签、调整题目顺序、导出题库、恢复题库内回收站题目。
// 第 11 项为回收站查看：§10.4 把「恢复题库内回收站题目」与「彻底删除回收站题目」
// 列为两个独立操作，恢复的前提是能看到回收站，因此协作者在获授恢复的同时
// 获得回收站查看，但仍不可彻底删除。
var collaboratorCapabilities = map[LibraryCapability]bool{
	CapViewLibrary:       true,
	CapViewRecycleBin:    true,
	CapUpdateDescription: true,
	CapAddQuestion:       true,
	CapDeleteQuestion:    true,
	CapUpdateQuestion:    true,
	CapTagQuestion:       true,
	CapUntagQuestion:     true,
	CapReorderQuestion:   true,
	CapExportLibrary:     true,
	CapRestoreQuestion:   true,
}

// CollaboratorCapabilities 返回协作者可做的全部能力，供权限矩阵测试逐条核对。
func CollaboratorCapabilities() []LibraryCapability {
	out := make([]LibraryCapability, 0, len(collaboratorCapabilities))
	for _, cap := range AllLibraryCapabilities() {
		if collaboratorCapabilities[cap] {
			out = append(out, cap)
		}
	}
	return out
}

// CollaboratorForbiddenCapabilities 返回协作者不可做的 7 项（需求 §10.5 下半清单）：
// 删除整个题库、修改题库名称、邀请新协作者、移除其他协作者、转移题库所有权、
// 彻底删除回收站题目、修改题库基础权限设置。
func CollaboratorForbiddenCapabilities() []LibraryCapability {
	return []LibraryCapability{
		CapDeleteLibrary, CapUpdateName, CapInviteMember, CapRemoveMember,
		CapTransferOwnership, CapPurgeQuestion, CapUpdatePermissions,
	}
}

// adminReadOnlyCapabilities 是管理员在他人题库上的权限：按 C-18 可查看私有对象，
// 因此可查看与导出，但不参与协作，也不改动他人题库。
var adminReadOnlyCapabilities = map[LibraryCapability]bool{
	CapViewLibrary:    true,
	CapViewRecycleBin: true,
	CapExportLibrary:  true,
}

// LibraryActor 是「某身份在某题库中的角色」，题库权限判定的唯一入口。
type LibraryActor struct {
	Viewer Viewer
	// IsMember 为假表示该身份不是题库成员。所有者必然是成员。
	IsMember bool
	Role     LibraryRole
}

// ActorFor 由身份与成员记录构造判定主体。member 为 nil 表示不是成员。
func ActorFor(v Viewer, member *LibraryMember) LibraryActor {
	if member == nil {
		return LibraryActor{Viewer: v}
	}
	return LibraryActor{Viewer: v, IsMember: true, Role: member.Role}
}

// IsOwner 判定是否为题库所有者。
func (a LibraryActor) IsOwner() bool {
	return a.IsMember && a.Role == LibraryRoleOwner
}

// Can 判定该主体是否可执行某项能力。
func (a LibraryActor) Can(cap LibraryCapability) bool {
	if a.Viewer.IsAdmin() {
		return adminReadOnlyCapabilities[cap]
	}
	if !a.IsMember {
		return false
	}
	switch a.Role {
	case LibraryRoleOwner:
		return true
	case LibraryRoleCollaborator:
		return collaboratorCapabilities[cap]
	default:
		return false
	}
}

// Library 是私有题库实体（需求 §10.1）。
type Library struct {
	ID          int64
	OwnerID     int64
	Name        string
	Description string
	// QuestionCount 是配额计数：引用题数量，含回收站中的引用题（C-22）。
	QuestionCount int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Access 返回题库的可见性判定要素。
//
// 题库没有公开档位（需求 §10.1：私有题库），因此恒为 private 档：
// 只有成员与管理员能看到，非成员一律 404。HighRisk 恒为 false，
// 高风险是提示词类别的属性，题库内的高风险内容在题目层逐条复查。
func (l Library) Access() Access {
	return Access{Level: VisibilityPrivate, HighRisk: false, UploaderID: l.OwnerID}
}

// LibraryMember 是一条成员记录（需求 §10.3）。
type LibraryMember struct {
	ID        int64
	LibraryID int64
	UserID    int64
	Role      LibraryRole
	InvitedBy *int64
}

// LibraryQuestionKind 是题库内题目的类型（需求 §10.7）。
type LibraryQuestionKind string

const (
	// QuestionReference 引用题目：读取来源提示词的最新版本，不存副本。
	QuestionReference LibraryQuestionKind = "reference"
	// QuestionDerived 派生题目：在题库内修改后的副本，独立存储。
	QuestionDerived LibraryQuestionKind = "derived"
)

// ParseLibraryQuestionKind 解析题目类型字符串。
func ParseLibraryQuestionKind(raw string) (LibraryQuestionKind, bool) {
	switch LibraryQuestionKind(raw) {
	case QuestionReference, QuestionDerived:
		return LibraryQuestionKind(raw), true
	default:
		return "", false
	}
}

// LibraryQuestion 是题库内的一道题目。
//
// 引用题只存来源提示词标识，正文在读取时取自来源提示词的最新版本（需求 §7.4）；
// 派生题自带标题与正文，不受来源提示词重大修改影响（C-28）。
type LibraryQuestion struct {
	ID                    int64
	LibraryID             int64
	Kind                  LibraryQuestionKind
	SourcePromptID        *int64
	SourcePromptVersionID *int64
	Title                 string
	Body                  string
	Description           string
	CategoryID            *int64
	AbilityID             *int64
	DifficultyID          *int64
	Sensitive             SensitiveLevel
	SortOrder             int32
}

// SourceAccess 返回题目内容所属的可见性判定要素。
//
// 引用题的正文来自来源提示词，判定要素完全取自来源提示词（C-20：高风险类别与
// 开发者可见同等处理）；派生题的副本虽然存于题库，其内容同样源自来源提示词，
// 因此沿用来源的判定要素，否则开发者可见或高风险的内容会以副本形式流到普通用户。
// source 为 nil 表示来源提示词已不存在：引用题没有正文可读，派生题作为独立副本
// 仍然可读（C-28）。
func (q LibraryQuestion) SourceAccess(source *Access) (Access, bool) {
	if q.Kind == QuestionReference {
		if source == nil {
			return Access{}, false
		}
		return *source, true
	}
	if source == nil {
		return Access{}, true
	}
	return *source, true
}

// LibraryQuestionSourceMissing 是来源提示词不可用时引用题的标题文案。
//
// 引用题的正文只在读取时取自来源提示词，来源被删除后没有正文可展示。题目保留在
// 题库中并以此文案标注，使成员能删掉它并归还容量；来源提示词被恢复后题目自动恢复可读。
const LibraryQuestionSourceMissing = "（来源提示词已删除）"

// LibraryExportElements 是导出内容必须包含的 14 项要素（需求 §10.8 与 §10.7）。
//
// §10.8 列出 12 项；来源提示词与来源版本取自 §10.7「保留来源提示词 ID」与
// §7.4「引用题跟随最新版本」，导出须能定位到所导出版本，因此单列两项。
// 该列表是导出测试逐项核对的依据。
func LibraryExportElements() []string {
	return []string{
		"library_name",         // 题库名称
		"library_description",  // 题库描述
		"owner_name",           // 创建者
		"exported_at",          // 导出时间
		"question_count",       // 题目数量
		"question_order",       // 题目顺序
		"question_title",       // 题目标题
		"question_category",    // 题目类别
		"question_tags",        // 题目标签
		"prompt_body",          // 提示词正文
		"question_description", // 题目说明
		"source_prompt",        // 来源提示词
		"source_version",       // 来源版本
		"derivation_note",      // 派生题目修改说明
	}
}

// 题库与题目的字段长度上限。题目正文与提示词正文同源（提示词正文上限 200000），
// 派生题正文也必须能容纳同样长度。
const (
	// LibraryNameMaxRunes 是题库名称的字数上限。
	LibraryNameMaxRunes = 100
	// LibraryDescriptionMaxRunes 是题库描述与题目说明的字数上限。
	LibraryDescriptionMaxRunes = 2000
	// LibraryQuestionTitleMaxRunes 是题目标题的字数上限，与提示词标题一致。
	LibraryQuestionTitleMaxRunes = 200
	// LibraryQuestionBodyMaxRunes 是题目正文的字数上限，与提示词正文一致。
	LibraryQuestionBodyMaxRunes = 200000
	// LibraryQuestionTagMaxRunes 是题目标签的字数上限。
	LibraryQuestionTagMaxRunes = 50
	// LibraryQuestionTagCountLimit 是单题标签数量上限。
	LibraryQuestionTagCountLimit = 20
)

// ValidateLibraryName 校验题库名称。
func ValidateLibraryName(name string) error {
	if len([]rune(name)) == 0 {
		return fmt.Errorf("题库名称不能为空")
	}
	if len([]rune(name)) > LibraryNameMaxRunes {
		return fmt.Errorf("题库名称不能超过 %d 个字", LibraryNameMaxRunes)
	}
	return nil
}

// ValidateLibraryDescription 校验题库描述。
func ValidateLibraryDescription(description string) error {
	if len([]rune(description)) > LibraryDescriptionMaxRunes {
		return fmt.Errorf("题库描述不能超过 %d 个字", LibraryDescriptionMaxRunes)
	}
	return nil
}

// ValidateLibraryQuestionTitle 校验题目标题。
func ValidateLibraryQuestionTitle(title string) error {
	if len([]rune(title)) == 0 {
		return fmt.Errorf("题目标题不能为空")
	}
	if len([]rune(title)) > LibraryQuestionTitleMaxRunes {
		return fmt.Errorf("题目标题不能超过 %d 个字", LibraryQuestionTitleMaxRunes)
	}
	return nil
}

// ValidateLibraryQuestionBody 校验题目正文。
func ValidateLibraryQuestionBody(body string) error {
	if len([]rune(body)) == 0 {
		return fmt.Errorf("题目正文不能为空")
	}
	if len([]rune(body)) > LibraryQuestionBodyMaxRunes {
		return fmt.Errorf("题目正文不能超过 %d 个字", LibraryQuestionBodyMaxRunes)
	}
	return nil
}

// ValidateLibraryQuestionDescription 校验题目说明。
func ValidateLibraryQuestionDescription(description string) error {
	if len([]rune(description)) > LibraryDescriptionMaxRunes {
		return fmt.Errorf("题目说明不能超过 %d 个字", LibraryDescriptionMaxRunes)
	}
	return nil
}

// ValidateLibraryQuestionTag 校验题目标签。
func ValidateLibraryQuestionTag(tag string) error {
	if len([]rune(tag)) == 0 {
		return fmt.Errorf("题目标签不能为空")
	}
	if len([]rune(tag)) > LibraryQuestionTagMaxRunes {
		return fmt.Errorf("题目标签不能超过 %d 个字", LibraryQuestionTagMaxRunes)
	}
	return nil
}

// RecycleBinObjectType 是回收站收录的对象类型（需求 §11.1、§11.2）。
// 初版启用提示词、整个私有题库、私有题库中的题目三类。
type RecycleBinObjectType string

const (
	RecycleObjectPrompt          RecycleBinObjectType = "prompt"
	RecycleObjectLibrary         RecycleBinObjectType = "library"
	RecycleObjectLibraryQuestion RecycleBinObjectType = "library_question"
)

// ParseRecycleBinObjectType 解析回收站对象类型。
func ParseRecycleBinObjectType(raw string) (RecycleBinObjectType, bool) {
	switch RecycleBinObjectType(raw) {
	case RecycleObjectPrompt, RecycleObjectLibrary, RecycleObjectLibraryQuestion:
		return RecycleBinObjectType(raw), true
	default:
		return "", false
	}
}

// RecycleRetentionDays 是回收站的默认保留天数（需求 §11.3）。
// 实际取值由配置项 recycle_retention_days 决定。
const RecycleRetentionDays = 14

// RecycleBinExpiry 计算删除时刻对应的回收站到期时刻。
func RecycleBinExpiry(deletedAt time.Time, retentionDays int64) time.Time {
	if retentionDays <= 0 {
		retentionDays = RecycleRetentionDays
	}
	return deletedAt.AddDate(0, 0, int(retentionDays))
}

// NotificationLibraryQuestionDeletedBody 生成协作者删题通知的正文（需求 §10.6）。
//
// 通知正文只在此处生成，服务层与测试共用同一份文案，避免两处各写一遍。
func NotificationLibraryQuestionDeletedBody(actorName, libraryName, questionTitle string) string {
	return fmt.Sprintf("协作者%s删除了题库「%s」中的题目「%s」。", actorName, libraryName, questionTitle)
}

// NotificationLibraryInvitedBody 生成加入题库邀请的通知正文。
func NotificationLibraryInvitedBody(libraryName string) string {
	return fmt.Sprintf("你已被邀请加入私有题库「%s」，可在题库列表中查看。", libraryName)
}
