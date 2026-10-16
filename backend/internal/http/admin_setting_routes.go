package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
)

// 模块 11：系统设置（需求 §20）。保存密钥类配置项需要主密钥，
// 未配置时返回 secret_unavailable。
func (s *Server) registerAdminSettingRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/settings",
		OperationID: "adminListSettings", Summary: "配置项列表", Tags: []string{"后台"},
	}, s.handleAdminListSettings)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/admin/settings/{key}",
		OperationID: "adminSaveSetting", Summary: "保存配置项", Tags: []string{"后台"},
	}, s.handleAdminSaveSetting)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/settings/smtp-test",
		OperationID: "adminSendTestEmail", Summary: "发送 SMTP 测试邮件", Tags: []string{"后台"},
	}, s.handleAdminSendTestEmail)
}

func (s *Server) handleAdminListSettings(ctx context.Context, _ *struct{}) (*adminConfigListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	items, err := s.deps.Admin.ListSettings(ctx)
	if err != nil {
		return nil, err
	}

	out := &adminConfigListResponse{}
	out.Body.SecretWritable = s.deps.Admin.SecretsAvailable()
	out.Body.Items = make([]adminConfigDTO, 0, len(items))
	for _, item := range items {
		dto := adminConfigDTO{
			Key:         item.Key,
			Kind:        string(item.Kind),
			Description: item.Description,
			Secret:      item.Secret,
			Source:      item.Source,
			UpdatedAt:   timePtrOfTime(item.UpdatedAt),
		}
		dto.Value = encodeConfigValue(item.Value)
		dto.Effective = encodeConfigValue(item.Effective)
		out.Body.Items = append(out.Body.Items, dto)
	}
	return out, nil
}

// encodeConfigValue 把配置取值编码为 JSON 原文。取值缺失时留空，
// 使前端能区分「未设置」与「设置为空字符串」。
func encodeConfigValue(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

type adminSaveSettingInput struct {
	Key  string `path:"key" doc:"配置项键名"`
	Body struct {
		// Value 是配置取值，形态由配置项的 Kind 决定。
		Value json.RawMessage `json:"value" doc:"配置取值"`
	}
}

func (s *Server) handleAdminSaveSetting(ctx context.Context, in *adminSaveSettingInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.Body.Value) == 0 {
		return nil, NewError(apperr.CodeValidationFailed, "配置取值不可为空")
	}
	if err := s.deps.Admin.SaveSetting(ctx, operator, in.Key, in.Body.Value, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminTestEmailInput struct {
	Body struct {
		To string `json:"to" minLength:"3" maxLength:"200" doc:"收件地址"`
	}
}

func (s *Server) handleAdminSendTestEmail(ctx context.Context, in *adminTestEmailInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.SendTestEmail(ctx, operator, in.Body.To, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
