package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/domain"
)

// 字典端点对游客开放（开发方案 §5.5 字典组级别为 G）。
// 考察目的列表需要区分高风险：普通用户不得看到高风险条目，否则筛选器会列出无法使用的选项。
func (s *Server) registerDictionaryRoutes() {
	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/purposes",
		OperationID: "listPurposes", Summary: "考察目的列表", Tags: []string{"字典"},
	}, s.handleListPurposes)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/capabilities",
		OperationID: "listCapabilities", Summary: "考察维度列表", Tags: []string{"字典"},
	}, s.handleListCapabilities)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/difficulties",
		OperationID: "listDifficulties", Summary: "难度列表", Tags: []string{"字典"},
	}, s.handleListDifficulties)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/tags",
		OperationID: "listTags", Summary: "主题标签列表", Tags: []string{"字典"},
	}, s.handleListTags)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/models",
		OperationID: "listModels", Summary: "模型列表", Tags: []string{"字典"},
	}, s.handleListModels)
}

type purposeDTO struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	NameZh        string `json:"name_zh"`
	NameEn        string `json:"name_en"`
	DescriptionZh string `json:"description_zh"`
	DescriptionEn string `json:"description_en"`
	IsHighRisk    bool   `json:"is_high_risk"`
}

type capabilityDTO struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	NameZh        string `json:"name_zh"`
	NameEn        string `json:"name_en"`
	GroupKey      string `json:"group_key"`
	DescriptionZh string `json:"description_zh"`
	DescriptionEn string `json:"description_en"`
}

func toPurposeDTO(p domain.Purpose) purposeDTO {
	return purposeDTO{
		ID: p.ID, Slug: p.Slug, NameZh: p.NameZh, NameEn: p.NameEn,
		DescriptionZh: p.DescriptionZh, DescriptionEn: p.DescriptionEn,
		IsHighRisk: p.IsHighRisk,
	}
}

func toCapabilityDTO(c domain.Capability) capabilityDTO {
	return capabilityDTO{
		ID: c.ID, Slug: c.Slug, NameZh: c.NameZh, NameEn: c.NameEn,
		GroupKey: c.GroupKey, DescriptionZh: c.DescriptionZh, DescriptionEn: c.DescriptionEn,
	}
}

type purposeListResponse struct {
	Body struct {
		Items []purposeDTO `json:"items"`
	}
}

func (s *Server) handleListPurposes(ctx context.Context, _ *struct{}) (*purposeListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Repo.ListPurposes(ctx)
	if err != nil {
		return nil, err
	}

	out := &purposeListResponse{}
	out.Body.Items = make([]purposeDTO, 0, len(rows))
	for _, row := range rows {
		// 高风险考察目的只对开发者展示（C-19）。判定使用与提示词可见性同一组谓词。
		if row.IsHighRisk && !v.CanUseHighRiskPurpose() {
			continue
		}
		out.Body.Items = append(out.Body.Items, toPurposeDTO(domain.Purpose{
			ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
			DescriptionZh: row.DescriptionZh, DescriptionEn: row.DescriptionEn,
			IsHighRisk: row.IsHighRisk, IsEnabled: row.IsEnabled, SortOrder: row.SortOrder,
		}))
	}
	return out, nil
}

type capabilityListResponse struct {
	Body struct {
		Items []capabilityDTO `json:"items"`
	}
}

func (s *Server) handleListCapabilities(ctx context.Context, _ *struct{}) (*capabilityListResponse, error) {
	rows, err := s.deps.Repo.ListCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	out := &capabilityListResponse{}
	out.Body.Items = make([]capabilityDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Items = append(out.Body.Items, toCapabilityDTO(domain.Capability{
			ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
			GroupKey: row.GroupKey, DescriptionZh: row.DescriptionZh, DescriptionEn: row.DescriptionEn,
			IsEnabled: row.IsEnabled, SortOrder: row.SortOrder,
		}))
	}
	return out, nil
}

type difficultyDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
	Level  int16  `json:"level"`
}

func toDifficultyDTO(d domain.Difficulty) difficultyDTO {
	return difficultyDTO{
		ID: d.ID, Slug: d.Slug, NameZh: d.NameZh, NameEn: d.NameEn, Level: d.Level,
	}
}

type difficultyListResponse struct {
	Body struct {
		Items []difficultyDTO `json:"items"`
	}
}

func (s *Server) handleListDifficulties(ctx context.Context, _ *struct{}) (*difficultyListResponse, error) {
	rows, err := s.deps.Repo.ListDifficulties(ctx)
	if err != nil {
		return nil, err
	}
	out := &difficultyListResponse{}
	out.Body.Items = make([]difficultyDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Items = append(out.Body.Items, difficultyDTO{
			ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn, Level: row.Level,
		})
	}
	return out, nil
}

type tagListResponse struct {
	Body struct {
		Items []tagDTO `json:"items"`
	}
}

func (s *Server) handleListTags(ctx context.Context, _ *struct{}) (*tagListResponse, error) {
	rows, err := s.deps.Repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := &tagListResponse{}
	out.Body.Items = make([]tagDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Items = append(out.Body.Items, tagDTO{
			ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
			Color: row.Color, Kind: row.Kind,
		})
	}
	return out, nil
}

type modelDTO struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ModelKey     string `json:"model_key"`
	ProviderKey  string `json:"provider_key"`
	ProviderName string `json:"provider_name"`
}

type modelListResponse struct {
	Body struct {
		Items []modelDTO `json:"items"`
	}
}

func (s *Server) handleListModels(ctx context.Context, _ *struct{}) (*modelListResponse, error) {
	rows, err := s.deps.Repo.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := &modelListResponse{}
	out.Body.Items = make([]modelDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Items = append(out.Body.Items, modelDTO{
			ID: row.ID, Name: row.Name, ModelKey: row.ModelKey,
			ProviderKey: row.ProviderKey, ProviderName: row.ProviderName,
		})
	}
	return out, nil
}

// visibilityOptions 返回当前身份可选的可见性档位。上传表单据此隐藏「开发者可见」，
// 使普通用户无法选中（C-19、需求 §8.7）。
func visibilityOptions(v domain.Viewer) []string {
	out := []string{string(domain.VisibilityPublic), string(domain.VisibilityPrivate)}
	if v.CanUploadDeveloper() {
		out = append(out, string(domain.VisibilityDeveloper))
	}
	return out
}
