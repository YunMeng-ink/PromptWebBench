package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// ReportStatus 是举报的处理状态（需求 §17.6）。
type ReportStatus string

const (
	ReportPending  ReportStatus = "pending"
	ReportIgnored  ReportStatus = "ignored"
	ReportResolved ReportStatus = "resolved"
)

// AllReportStatuses 返回全部举报状态，供筛选参数校验使用。
func AllReportStatuses() []ReportStatus {
	return []ReportStatus{ReportPending, ReportIgnored, ReportResolved}
}

// ParseReportStatus 解析举报状态，非法取值返回 false。
func ParseReportStatus(raw string) (ReportStatus, bool) {
	for _, s := range AllReportStatuses() {
		if ReportStatus(raw) == s {
			return s, true
		}
	}
	return "", false
}

// ModelModalityCountLimit 是模型输入模态、能力标签与别名的数量上限。
const ModelModalityCountLimit = 20

// ModelModalityMaxRunes 是单个模态、能力标签或别名的字数上限。
const ModelModalityMaxRunes = 50

// NormalizeStringList 规范化模型的字符串数组字段（输入模态、能力标签、别名）：
// 去空白、丢空项、去重复，保持首次出现的顺序。
// 取值本身不做白名单校验：需求 §13.3 允许 models.dev 的原始取值原样保留，
// 由管理员在导入后编辑映射。
func NormalizeStringList(raw []string, label string) ([]string, error) {
	if len(raw) > ModelModalityCountLimit {
		return nil, fmt.Errorf("%s最多 %d 项", label, ModelModalityCountLimit)
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, item := range raw {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if len([]rune(value)) > ModelModalityMaxRunes {
			return nil, fmt.Errorf("%s「%s」超出 %d 个字", label, value, ModelModalityMaxRunes)
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}

// ModelImportAction 是模型导入的一次动作（需求 §13.4）。取值与
// model_import_logs.action 的 CHECK 约束一致。
type ModelImportAction string

const (
	ModelImportCreated   ModelImportAction = "import"
	ModelImportOverwrite ModelImportAction = "overwrite"
	ModelImportSkipped   ModelImportAction = "skip"
)

// ModelNoteMaxRunes 是模型备注的字数上限。
const ModelNoteMaxRunes = 500

// ModelAliasCountLimit 是模型别名的数量上限。
const ModelAliasCountLimit = 20

// ConfigKind 是配置项的取值类型（技术可行性报告 §2.14 末条）。
type ConfigKind string

const (
	ConfigInt        ConfigKind = "int"
	ConfigFloat      ConfigKind = "float"
	ConfigBool       ConfigKind = "bool"
	ConfigString     ConfigKind = "string"
	ConfigEnum       ConfigKind = "enum"
	ConfigStringList ConfigKind = "string_list"
)

// ConfigSpec 声明一个配置项的取值类型与合法范围。管理端保存时按本表校验，
// 非法取值拒绝写入，使被写坏的配置不可能进入 config 表。
type ConfigSpec struct {
	Key         string
	Kind        ConfigKind
	Description string
	Min         float64
	Max         float64
	Enum        []string
	Secret      bool
}

// Validate 校验并规范化一个配置取值。
//
// 入参是 JSON 解码后的值（数字为 float64），返回值是写入 config 表的规范取值。
// 整型项在这里拒绝小数，避免出现「上限 100 却存了 99.5」这类无法比较的取值。
func (s ConfigSpec) Validate(value any) (any, error) {
	switch s.Kind {
	case ConfigInt, ConfigFloat:
		number, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("配置项 %s 需要数值", s.Key)
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("配置项 %s 需要有限数值", s.Key)
		}
		if number < s.Min || number > s.Max {
			return nil, fmt.Errorf("配置项 %s 必须落在 %v 到 %v 之间", s.Key, s.Min, s.Max)
		}
		if s.Kind == ConfigInt {
			if number != math.Trunc(number) {
				return nil, fmt.Errorf("配置项 %s 需要整数", s.Key)
			}
			return int64(number), nil
		}
		return number, nil
	case ConfigBool:
		flag, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("配置项 %s 需要布尔值", s.Key)
		}
		return flag, nil
	case ConfigString:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("配置项 %s 需要文本", s.Key)
		}
		return strings.TrimSpace(text), nil
	case ConfigEnum:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("配置项 %s 需要文本", s.Key)
		}
		for _, allowed := range s.Enum {
			if text == allowed {
				return text, nil
			}
		}
		return nil, fmt.Errorf("配置项 %s 的取值必须属于 %s", s.Key, strings.Join(s.Enum, " / "))
	case ConfigStringList:
		raw, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("配置项 %s 需要字符串数组", s.Key)
		}
		out := make([]string, 0, len(raw))
		seen := make(map[string]bool, len(raw))
		for _, item := range raw {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("配置项 %s 的数组元素必须是文本", s.Key)
			}
			normalized, err := s.validateListItem(text)
			if err != nil {
				return nil, err
			}
			if seen[normalized] {
				continue
			}
			seen[normalized] = true
			out = append(out, normalized)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("配置项 %s 的类型 %q 未登记", s.Key, s.Kind)
	}
}

// domainPattern 是邮箱后缀白名单项的形态：可选前导 @，后接域名。
var domainPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)

// validateListItem 校验字符串数组元素。当前只有邮箱后缀白名单使用该类型，
// 元素必须是域名形态，否则注册校验会把明显写错的项当成永不匹配的白名单。
func (s ConfigSpec) validateListItem(text string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(text))
	value = strings.TrimPrefix(value, "@")
	if !domainPattern.MatchString(value) {
		return "", fmt.Errorf("配置项 %s 的取值 %q 不是合法域名", s.Key, text)
	}
	return value, nil
}

// smtpPortMin 与 smtpPortMax 是 SMTP 端口取值区间。
const (
	smtpPortMin = 1
	smtpPortMax = 65535
)

// smtpSpecs 是 SMTP 分组的配置项声明。
//
// 这五项不进入阶段 0 的 config 种子（种子只含技术可行性报告 §2.14 的 26 项参数），
// 由管理员在系统设置中填写后写入；未填写时发送邮件使用环境变量取值。
// 密码以密文存储（技术可行性报告 §3.3「密钥配置」），保存时必须已配置主密钥。
func smtpSpecs() []ConfigSpec {
	return []ConfigSpec{
		{Key: "smtp_host", Kind: ConfigString, Description: "SMTP 服务器地址"},
		{Key: "smtp_port", Kind: ConfigInt, Min: smtpPortMin, Max: smtpPortMax, Description: "SMTP 服务器端口"},
		{Key: "smtp_user", Kind: ConfigString, Description: "SMTP 登录账户"},
		{Key: "smtp_password", Kind: ConfigString, Description: "SMTP 登录口令", Secret: true},
		{Key: "smtp_from", Kind: ConfigString, Description: "验证码邮件的发件人地址"},
	}
}

// ConfigSpecs 返回全部可配置项的声明。顺序即管理端的展示顺序：
// 先按业务分组的参数（技术可行性报告 §2.14 的 26 项，加 §3.1 的统计缓存时长），再是 SMTP 分组。
func ConfigSpecs() []ConfigSpec {
	return append(configSpecs(), smtpSpecs()...)
}

// configSpecs 是技术可行性报告 §2.14 的 26 项参数，加上阶段 7 按 §3.1 补充的统计缓存时长。
func configSpecs() []ConfigSpec {
	return []ConfigSpec{
		{Key: "bayes_prior", Kind: ConfigFloat, Min: 0, Max: 5, Description: "贝叶斯先验分"},
		{Key: "bayes_k", Kind: ConfigFloat, Min: 0.1, Max: 1000, Description: "贝叶斯先验权重"},
		{Key: "saturation_threshold", Kind: ConfigFloat, Min: 0, Max: 5, Description: "训练饱和阈值"},
		{Key: "obsolete_discrimination_threshold", Kind: ConfigFloat, Min: 0, Max: 1000, Description: "过时区分度阈值"},
		{Key: "discrim_min_models", Kind: ConfigInt, Min: 1, Max: 100, Description: "区分度有效模型数阈值"},
		{Key: "discrim_min_ratings_per_model", Kind: ConfigInt, Min: 1, Max: 100, Description: "每个模型最低评分数阈值"},
		{Key: "discrim_min_total_ratings", Kind: ConfigInt, Min: 1, Max: 10000, Description: "区分度总评分数阈值"},
		{Key: "discrim_model_count_target", Kind: ConfigFloat, Min: 1, Max: 100, Description: "数据充分系数中的模型数目标"},
		{Key: "discrim_rating_count_target", Kind: ConfigFloat, Min: 1, Max: 10000, Description: "数据充分系数中的评分数目标"},
		{Key: "rating_max_edits_per_record", Kind: ConfigInt, Min: 0, Max: 100, Description: "单条评分最多修改次数"},
		{Key: "rating_daily_edit_limit", Kind: ConfigInt, Min: 0, Max: 10000, Description: "每用户每日最多修改评分次数"},
		{Key: "rating_daily_delete_limit", Kind: ConfigInt, Min: 0, Max: 10000, Description: "每用户每日最多删除评分次数"},
		{Key: "recycle_retention_days", Kind: ConfigInt, Min: 1, Max: 365, Description: "回收站保留天数"},
		{Key: "recent_window_days", Kind: ConfigInt, Min: 1, Max: 90, Description: "近 N 日窗口天数"},
		{Key: "private_prompt_quota_user", Kind: ConfigInt, Min: 0, Max: 100000, Description: "普通用户私有提示词上限"},
		{Key: "private_prompt_quota_developer", Kind: ConfigInt, Min: 0, Max: 100000, Description: "开发者用户私有提示词上限"},
		{Key: "library_quota_user", Kind: ConfigInt, Min: 0, Max: 1000, Description: "普通用户私有题库上限"},
		{Key: "library_quota_developer", Kind: ConfigInt, Min: 0, Max: 1000, Description: "开发者用户私有题库上限"},
		{Key: "library_question_limit", Kind: ConfigInt, Min: 1, Max: 100000, Description: "单个私有题库题目上限"},
		{Key: "favorite_item_limit", Kind: ConfigInt, Min: 1, Max: 100000, Description: "单个收藏夹提示词上限"},
		{Key: "favorite_count_limit_user", Kind: ConfigInt, Min: 0, Max: 10000, Description: "普通用户收藏夹数量上限"},
		{Key: "favorite_count_limit_developer", Kind: ConfigInt, Min: 0, Max: 10000, Description: "开发者用户收藏夹数量上限"},
		{Key: "bootstrap_iterations", Kind: ConfigInt, Min: 1, Max: 100000, Description: "Bootstrap 重复次数"},
		{Key: "stats_cache_ttl_seconds", Kind: ConfigInt, Min: 1, Max: 86400, Description: "统计缓存存活时长（秒）"},
		{Key: "registration_policy", Kind: ConfigEnum, Description: "注册策略",
			Enum: []string{string(RegistrationOpen), string(RegistrationClosed), string(RegistrationInviteOnly)}},
		{Key: "email_domain_whitelist", Kind: ConfigStringList, Description: "邮箱后缀白名单，空数组表示不限制"},
		{Key: "collaborator_restore_allowed", Kind: ConfigBool, Description: "协作者可否恢复题库回收站题目"},
	}
}

// ConfigSpecByKey 按键查找配置项声明。
func ConfigSpecByKey(key string) (ConfigSpec, bool) {
	for _, spec := range ConfigSpecs() {
		if spec.Key == key {
			return spec, true
		}
	}
	return ConfigSpec{}, false
}

// InviteNoteMaxRunes 是邀请码备注的字数上限。
const InviteNoteMaxRunes = 200

// InviteBatchLimit 是一次批量生成邀请码的数量上限。
const InviteBatchLimit = 50

// InviteMaxUsesLimit 是单个邀请码可设置的最大使用次数上限。
const InviteMaxUsesLimit = 1000

// InviteExpiryDaysLimit 是邀请码有效期的天数上限。
const InviteExpiryDaysLimit = 365

// DashboardCounters 是统计仪表盘的 17 项计数指标（需求 §17.1）。
// 响应字段与声明一一对应，测试按本表逐项核对。
func DashboardCounters() []string {
	return []string{
		"user_total", "user_today", "user_role_user", "user_role_developer", "user_banned",
		"prompt_total", "prompt_public", "prompt_private", "prompt_developer", "prompt_today",
		"rating_total", "comment_total", "report_pending", "recycle_total",
		"model_total", "invite_total", "recommend_pending",
	}
}

// DashboardLists 是统计仪表盘的 2 个榜单（需求 §17.1）。
func DashboardLists() []string {
	return []string{"hot_prompts", "recent_prompts"}
}

// DashboardHotPromptLimit 与 DashboardRecentPromptLimit 是两个榜单的条目数量。
const (
	DashboardHotPromptLimit    = 5
	DashboardRecentPromptLimit = 5
)
