package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/modelsdev"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// DuplicateModel 描述一条与已有模型重复的平台记录（需求 §13.4）。
type DuplicateModel struct {
	ID           int64
	Name         string
	ProviderName string
}

// ModelImportEntry 是导入预览中的一条条目。重复项带上平台已有记录，
// 供管理员二次确认是否覆盖。
type ModelImportEntry struct {
	ProviderID      string
	ProviderName    string
	ModelID         string
	Name            string
	Description     string
	ReleaseDate     string
	InputModalities []string
	ContextLength   *int64
	Duplicate       *DuplicateModel
}

// ModelImportOptions 是执行导入的选项。Overwrite 为真表示覆盖重复模型，
// 为假表示跳过重复模型（需求 §13.4 第 3、4 条）。
type ModelImportOptions struct {
	Overwrite bool
}

// ModelImportResult 是一次导入的结果统计。
type ModelImportResult struct {
	Total       int
	Created     int
	Overwritten int
	Skipped     int
}

// PreviewModelImport 读取 models.dev 目录并标出重复项（需求 §13.4「二次确认」）。
//
// 预览不写入任何数据：管理员看到重复清单后再决定是否覆盖。
func (s *Service) PreviewModelImport(ctx context.Context) ([]ModelImportEntry, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	entries, err := s.fetchCatalog(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := s.modelIdentities(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ModelImportEntry, 0, len(entries))
	for _, entry := range entries {
		item := ModelImportEntry{
			ProviderID:      entry.ProviderID,
			ProviderName:    entry.ProviderName,
			ModelID:         entry.ModelID,
			Name:            entry.Name,
			Description:     entry.Description,
			ReleaseDate:     entry.ReleaseDate,
			InputModalities: entry.InputModalities,
			ContextLength:   entry.ContextLength,
		}
		if found, ok := existing[identityKey(entry.ModelID, entry.ProviderID)]; ok {
			item.Duplicate = &DuplicateModel{
				ID: found.ID, Name: found.Name, ProviderName: found.ProviderName,
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// ImportModels 执行导入（需求 §13.3、§13.4）。
//
// 逐条判定：无同名模型则新建；有同名模型且允许覆盖则覆盖并保留平台模型 ID；
// 有同名模型且不允许覆盖则跳过。每条动作都写入 model_import_logs 与模型快照。
// 任何一条失败整体回滚，不留下「导入了一半」的目录。
func (s *Service) ImportModels(ctx context.Context, operator domain.User, opts ModelImportOptions, ip string) (ModelImportResult, error) {
	if err := s.ensure(); err != nil {
		return ModelImportResult{}, err
	}
	entries, err := s.fetchCatalog(ctx)
	if err != nil {
		return ModelImportResult{}, err
	}

	result := ModelImportResult{Total: len(entries)}
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		for _, entry := range entries {
			action, err := s.importEntry(ctx, q, operator, entry, opts)
			if err != nil {
				return err
			}
			switch action {
			case domain.ModelImportCreated:
				result.Created++
			case domain.ModelImportOverwrite:
				result.Overwritten++
			default:
				result.Skipped++
			}
		}
		if result.Created > 0 {
			if err := s.record(ctx, q, oplog.Entry{
				OperatorID: operator.ID,
				Action:     oplog.ActionModelImport,
				ObjectType: "model",
				After:      map[string]any{"created": result.Created, "total": result.Total},
				Note:       "从 models.dev 导入模型",
				IP:         ip,
			}); err != nil {
				return err
			}
		}
		if result.Overwritten > 0 {
			if err := s.record(ctx, q, oplog.Entry{
				OperatorID: operator.ID,
				Action:     oplog.ActionModelOverwrite,
				ObjectType: "model",
				After:      map[string]any{"overwritten": result.Overwritten},
				Note:       "覆盖已有模型",
				IP:         ip,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ModelImportResult{}, err
	}
	return result, nil
}

// importEntry 处理一条条目，返回实际发生的动作。
func (s *Service) importEntry(
	ctx context.Context, q *repo.Queries, operator domain.User, entry modelsdev.Entry, opts ModelImportOptions,
) (domain.ModelImportAction, error) {
	modalities, err := domain.NormalizeStringList(entry.InputModalities, "输入模态")
	if err != nil {
		return "", apperr.New(apperr.CodeValidationFailed,
			fmt.Sprintf("模型 %s 的输入模态非法：%s", entry.ModelID, err.Error()))
	}

	existing, err := q.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: entry.ModelID, ProviderKey: entry.ProviderID,
	})
	switch {
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return "", wrap("查询同名模型", 0, err)
	case err == nil && !opts.Overwrite:
		// 需求 §13.4 第 4 条：管理员选择跳过导入，仍然留痕。
		return domain.ModelImportSkipped, s.insertImportLog(ctx, q, operator, domain.ModelImportSkipped, entry, &existing.ID)
	case err == nil:
		row, err := q.UpdateModel(ctx, repo.UpdateModelParams{
			ID:              existing.ID,
			Name:            entry.Name,
			ModelKey:        entry.ModelID,
			ProviderKey:     entry.ProviderID,
			ProviderName:    entry.ProviderName,
			InputModalities: modalities,
			Aliases:         existing.Aliases,
			Description:     entry.Description,
			ReleaseDate:     repoDate(parseReleaseDate(entry.ReleaseDate)),
			ContextLength:   repoInt8(entry.ContextLength),
			IsEnabled:       existing.IsEnabled,
			AbilityTags:     existing.AbilityTags,
			Note:            existing.Note,
		})
		if err != nil {
			return "", wrap("覆盖模型", existing.ID, err)
		}
		if err := s.insertSnapshot(ctx, q, row.ID, entry.Raw); err != nil {
			return "", err
		}
		return domain.ModelImportOverwrite, s.insertImportLog(ctx, q, operator, domain.ModelImportOverwrite, entry, &row.ID)
	default:
		row, err := q.InsertModel(ctx, repo.InsertModelParams{
			Name:            entry.Name,
			ModelKey:        entry.ModelID,
			ProviderKey:     entry.ProviderID,
			ProviderName:    entry.ProviderName,
			InputModalities: modalities,
			Aliases:         []string{},
			Description:     entry.Description,
			ReleaseDate:     repoDate(parseReleaseDate(entry.ReleaseDate)),
			ContextLength:   repoInt8(entry.ContextLength),
			IsEnabled:       true,
			AbilityTags:     []string{},
			Note:            "",
			ImportSource:    "models_dev",
		})
		if err != nil {
			if isUniqueViolation(err) {
				return "", apperr.New(apperr.CodeDuplicateModel,
					fmt.Sprintf("模型 %s / %s 已被并发导入", entry.ModelID, entry.ProviderID))
			}
			return "", wrap("写入导入模型", 0, err)
		}
		if err := s.insertSnapshot(ctx, q, row.ID, entry.Raw); err != nil {
			return "", err
		}
		return domain.ModelImportCreated, s.insertImportLog(ctx, q, operator, domain.ModelImportCreated, entry, &row.ID)
	}
}

// insertSnapshot 保存模型的完整原始条目，用于字段追溯（需求 §13.3）。
func (s *Service) insertSnapshot(ctx context.Context, q *repo.Queries, modelID int64, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	if _, err := q.InsertModelSnapshot(ctx, repo.InsertModelSnapshotParams{
		ModelID: modelID, Source: "models_dev", Raw: raw,
	}); err != nil {
		return wrap("保存模型快照", modelID, err)
	}
	return nil
}

// insertImportLog 写入一条导入记录。
func (s *Service) insertImportLog(
	ctx context.Context, q *repo.Queries, operator domain.User,
	action domain.ModelImportAction, entry modelsdev.Entry, modelID *int64,
) error {
	payload, err := encodePayload(map[string]any{"provider_id": entry.ProviderID,
		"provider_name":    entry.ProviderName,
		"model_id":         entry.ModelID,
		"name":             entry.Name,
		"release_date":     entry.ReleaseDate,
		"input_modalities": entry.InputModalities,
		"context_length":   entry.ContextLength,
	})
	if err != nil {
		return err
	}
	if _, err := q.InsertModelImportLog(ctx, repo.InsertModelImportLogParams{
		ModelID:     repoInt8(modelID),
		Action:      string(action),
		ProviderKey: entry.ProviderID,
		ModelKey:    entry.ModelID,
		Payload:     payload,
		OperatorID:  repo.Int8(operator.ID),
	}); err != nil {
		return wrap("写入导入记录", 0, err)
	}
	return nil
}

// fetchCatalog 读取 models.dev 目录。任何失败都翻译为 bad_gateway：
// 对后台而言上游不可用与上游返回异常是同一种结果。
func (s *Service) fetchCatalog(ctx context.Context) ([]modelsdev.Entry, error) {
	entries, err := s.modelsDev.Fetch(ctx)
	if err != nil {
		s.logger.Error("读取 models.dev 目录失败", slog.String("error", err.Error()))
		return nil, apperr.New(apperr.CodeBadGateway, "读取 models.dev 目录失败："+err.Error())
	}
	return entries, nil
}

// modelIdentity 是重复判断用到的平台模型记录。
type modelIdentity struct {
	ID           int64
	Name         string
	ProviderName string
}

// modelIdentities 一次取出全部在用模型的身份列，避免逐条查询。
func (s *Service) modelIdentities(ctx context.Context) (map[string]modelIdentity, error) {
	rows, err := repo.New(s.pool).ListModelIdentities(ctx)
	if err != nil {
		return nil, wrap("查询模型身份", 0, err)
	}
	out := make(map[string]modelIdentity, len(rows))
	for _, row := range rows {
		out[identityKey(row.ModelKey, row.ProviderKey)] = modelIdentity{
			ID: row.ID, Name: row.Name, ProviderName: row.ProviderName,
		}
	}
	return out, nil
}

// identityKey 是重复判断的键：模型标识 + 模型厂商（需求 §13.4）。
func identityKey(modelKey, providerKey string) string {
	return modelKey + "\x00" + providerKey
}

// parseReleaseDate 解析 models.dev 的 release_date。取值不是日期形态时返回 nil，
// 发布时间留空由管理员补全，原始取值仍保留在模型快照中。
func parseReleaseDate(raw string) *time.Time {
	value := trimSpace(raw)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}
