package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

// ConfigValue 是配置项在管理端的展示形态。
type ConfigValue struct {
	Key         string            `json:"key"`
	Kind        domain.ConfigKind `json:"kind"`
	Description string            `json:"description"`
	Secret      bool              `json:"secret"`
	// Value 是配置表中的当前取值；密钥项恒为 null，未设置的非密钥项也是 null。
	Value any `json:"value"`
	// Effective 是实际生效的取值：配置表未设置时回落到环境变量提供的默认值。
	Effective any `json:"effective"`
	// Source 说明生效取值的来源：config 表或环境变量。
	Source    string     `json:"source"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// SecretsAvailable 返回密钥类配置项当前是否可写：未配置主密钥时保存会被拒绝。
// 管理端据此提示「未配置主密钥」，不把必然失败的保存入口摆给管理员。
func (s *Service) SecretsAvailable() bool { return s.secrets.Enabled() }

// ListSettings 返回全部可配置项（需求 §20、§17.8 的系统设置模块）。
//
// 取值来自两处：config 表与进程环境变量。界面需要看到「当前生效的是什么」，
// 因此每项都返回生效取值与它的来源；密钥项只返回是否已设置，不回显密文。
func (s *Service) ListSettings(ctx context.Context) ([]ConfigValue, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	rows, err := repo.New(s.pool).AdminListConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取配置表: %w", err)
	}
	stored := make(map[string]repo.AdminListConfigRow, len(rows))
	for _, row := range rows {
		stored[row.Key] = row
	}

	envValues := s.envDefaults()
	specs := domain.ConfigSpecs()
	out := make([]ConfigValue, 0, len(specs))
	for _, spec := range specs {
		item := ConfigValue{
			Key:         spec.Key,
			Kind:        spec.Kind,
			Description: spec.Description,
			Secret:      spec.Secret,
		}
		if env, ok := envValues[spec.Key]; ok {
			item.Effective = env
			item.Source = "env"
		}
		row, exists := stored[spec.Key]
		if exists {
			if row.UpdatedAt.Valid {
				updated := row.UpdatedAt.Time
				item.UpdatedAt = &updated
			}
			if !spec.Secret {
				var decoded any
				if err := json.Unmarshal(row.Value, &decoded); err != nil {
					return nil, fmt.Errorf("解析配置项 %s: %w", spec.Key, err)
				}
				item.Value = decoded
				item.Effective = decoded
				item.Source = "config"
			} else if item.Effective == nil {
				item.Source = "config"
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// envDefaults 返回环境变量提供的配置兜底取值。
//
// 只有 SMTP 分组可从环境变量提供：其余 26 项是业务参数，
// 由种子数据给出初值，不读环境变量。
func (s *Service) envDefaults() map[string]any {
	out := map[string]any{}
	if s.mail.Host != "" {
		out["smtp_host"] = s.mail.Host
	}
	if s.mail.Port != "" {
		out["smtp_port"] = s.mail.Port
	}
	if s.mail.User != "" {
		out["smtp_user"] = s.mail.User
	}
	if s.mail.From != "" {
		out["smtp_from"] = s.mail.From
	}
	if s.mail.Password != "" {
		out["smtp_password"] = "已由环境变量提供"
	}
	return out
}

// SaveSetting 保存一个配置项（需求 §20）。
//
// 校验按 domain.ConfigSpec 的声明执行：数值范围、枚举白名单、域名形态。
// 密钥项经主密钥加密后写入 secret_ciphertext；未配置主密钥时拒绝保存，
// 不会退化为明文存储（技术可行性报告 §3.3）。
func (s *Service) SaveSetting(ctx context.Context, operator domain.User, key string, raw json.RawMessage, ip string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	spec, ok := domain.ConfigSpecByKey(key)
	if !ok {
		return apperr.New(apperr.CodeValidationFailed, "配置项未登记")
	}
	if spec.Secret {
		return s.saveSecretSetting(ctx, operator, spec, raw, ip)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return apperr.New(apperr.CodeValidationFailed, "配置取值不是合法 JSON")
	}
	value, err := spec.Validate(decoded)
	if err != nil {
		return apperr.New(apperr.CodeValidationFailed, err.Error())
	}

	before, err := s.config.Get(ctx, key)
	if err != nil {
		// 配置项缺失属于预期情形（首次保存），不作为失败。
		before = nil
	}
	if err := s.config.Set(ctx, key, value, spec.Description, operator.ID); err != nil {
		return err
	}
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionConfigUpdate,
			ObjectType: "config",
			After:      map[string]any{"key": key, "value": value},
			Before:     decodeConfigSnapshot(before),
			IP:         ip,
		})
	})
}

// saveSecretSetting 保存密钥类配置项。取值必须是字符串。
func (s *Service) saveSecretSetting(ctx context.Context, operator domain.User, spec domain.ConfigSpec, raw json.RawMessage, ip string) error {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return apperr.New(apperr.CodeValidationFailed, "配置取值不是合法 JSON")
	}
	value, err := spec.Validate(decoded)
	if err != nil {
		return apperr.New(apperr.CodeValidationFailed, err.Error())
	}
	plain, ok := value.(string)
	if !ok {
		return apperr.New(apperr.CodeValidationFailed, "密钥类配置项需要文本取值")
	}

	ciphertext, err := s.secrets.Seal(plain)
	if err != nil {
		if errors.Is(err, security.ErrSecretUnavailable) {
			return apperr.New(apperr.CodeSecretUnavailable, "")
		}
		return fmt.Errorf("加密配置项 %s: %w", spec.Key, err)
	}
	if err := s.config.SetSecret(ctx, spec.Key, ciphertext, spec.Description, operator.ID); err != nil {
		return err
	}
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionConfigUpdate,
			ObjectType: "config",
			// 密钥取值不入日志，只记录它被更新过。
			After: map[string]any{"key": spec.Key, "secret_updated": true},
			IP:    ip,
		})
	})
}

// effectiveMailConfig 计算当前生效的 SMTP 配置：配置表优先，缺项回落环境变量。
func (s *Service) effectiveMailConfig(ctx context.Context) (mail.Config, error) {
	cfg := s.mail
	// override 读取配置表中的覆盖值。配置项不存在与取值为空都表示「不覆盖」。
	override := func(key string) (string, bool) {
		value, err := s.config.GetString(ctx, key)
		if err != nil {
			return "", false
		}
		return value, value != ""
	}
	if value, ok := override("smtp_host"); ok {
		cfg.Host = value
	}
	if value, ok := override("smtp_port"); ok {
		cfg.Port = value
	}
	if value, ok := override("smtp_user"); ok {
		cfg.User = value
	}
	if value, ok := override("smtp_from"); ok {
		cfg.From = value
	}

	// 未在系统设置中保存过口令时沿用环境变量提供的取值。
	if stored := s.storedSecret(ctx, "smtp_password"); stored != "" {
		password, err := s.secrets.Open(stored)
		if err != nil {
			if errors.Is(err, security.ErrSecretUnavailable) {
				return cfg, nil
			}
			return mail.Config{}, err
		}
		if password != "" {
			cfg.Password = password
		}
	}
	return cfg, nil
}

// storedSecret 读取密钥类配置项的密文。未设置或读取失败时返回空串。
func (s *Service) storedSecret(ctx context.Context, key string) string {
	row, err := repo.New(s.pool).AdminGetConfig(ctx, key)
	if err != nil {
		return ""
	}
	if !row.IsSecret || !row.SecretCiphertext.Valid {
		return ""
	}
	return row.SecretCiphertext.String
}

// SendTestEmail 发送一封测试邮件（需求 §20）。
//
// 无论成功还是失败都写入 email_test 日志：发送测试邮件是一次运维动作，
// 失败的原因同样需要留痕。
func (s *Service) SendTestEmail(ctx context.Context, operator domain.User, to, ip string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	address := trimSpace(to)
	if address == "" || !containsAt(address) {
		return apperr.New(apperr.CodeValidationFailed, "收件地址不合法")
	}

	cfg, err := s.effectiveMailConfig(ctx)
	if err != nil {
		return err
	}
	sendErr := mail.NewSMTPSender(cfg).SendTestEmail(ctx, address)

	note := "发送测试邮件"
	if sendErr != nil {
		note = "发送测试邮件失败：" + sendErr.Error()
	}
	if err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionEmailTest,
			ObjectType: "config",
			After:      map[string]any{"to": address, "success": sendErr == nil},
			Note:       note,
			IP:         ip,
		})
	}); err != nil {
		return err
	}
	if sendErr != nil {
		return apperr.New(apperr.CodeMailError, sendErr.Error())
	}
	return nil
}

// decodeConfigSnapshot 把配置表原始值转为日志快照。密钥项不记录取值。
func decodeConfigSnapshot(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

// containsAt 判定字符串是否含 @，用于收件地址的最小校验。
func containsAt(value string) bool {
	for _, r := range value {
		if r == '@' {
			return true
		}
	}
	return false
}
