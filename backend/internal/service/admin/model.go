package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// ModelInput 是模型的写入字段（需求 §13.2 的 14 个字段）。
//
// 导入来源由服务端决定，不接受调用方指定：手工添加固定为 manual，
// 从 models.dev 导入固定为 models_dev。
type ModelInput struct {
	Name            string
	ModelKey        string
	ProviderKey     string
	ProviderName    string
	InputModalities []string
	Aliases         []string
	Description     string
	ReleaseDate     *time.Time
	ContextLength   *int64
	IsEnabled       bool
	CapabilityIDs   []int64
	Note            string
}

// normalize 校验并规范化模型字段。名称与模型标识必填，
// 其余字段按类型规范化为可写取值。
func (in ModelInput) normalize() (ModelInput, error) {
	out := in
	out.Name = trimSpace(in.Name)
	out.ModelKey = trimSpace(in.ModelKey)
	out.ProviderKey = trimSpace(in.ProviderKey)
	out.ProviderName = trimSpace(in.ProviderName)
	out.Description = trimSpace(in.Description)
	out.Note = trimSpace(in.Note)

	if out.Name == "" {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, "模型名称不可为空")
	}
	if out.ModelKey == "" {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, "模型标识不可为空")
	}
	if len([]rune(out.Note)) > domain.ModelNoteMaxRunes {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, "模型备注超出长度上限")
	}
	if out.ContextLength != nil && *out.ContextLength < 0 {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, "上下文长度不可为负数")
	}

	var err error
	if out.InputModalities, err = domain.NormalizeStringList(in.InputModalities, "输入模态"); err != nil {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, err.Error())
	}
	seen := map[int64]bool{}
	out.CapabilityIDs = make([]int64, 0, len(in.CapabilityIDs))
	for _, capabilityID := range in.CapabilityIDs {
		if capabilityID <= 0 {
			return ModelInput{}, apperr.New(apperr.CodeValidationFailed, "考察维度取值非法")
		}
		if seen[capabilityID] {
			continue
		}
		seen[capabilityID] = true
		out.CapabilityIDs = append(out.CapabilityIDs, capabilityID)
	}
	if out.Aliases, err = domain.NormalizeStringList(in.Aliases, "模型别名"); err != nil {
		return ModelInput{}, apperr.New(apperr.CodeValidationFailed, err.Error())
	}
	return out, nil
}

// ListModels 返回模型列表（需求 §17.3「查看模型」）。
func (s *Service) ListModels(ctx context.Context, keyword string, includeDeleted bool, page, pageSize int32) (Page[repo.AdminListModelsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListModelsRow]{}, err
	}
	q := repo.New(s.pool)
	params := repo.AdminListModelsParams{
		Keyword:        trimSpace(keyword),
		IncludeDeleted: includeDeleted,
		LimitCount:     pageSize,
		OffsetCount:    (page - 1) * pageSize,
	}
	rows, err := q.AdminListModels(ctx, params)
	if err != nil {
		return Page[repo.AdminListModelsRow]{}, fmt.Errorf("查询模型列表: %w", err)
	}
	total, err := q.AdminCountModels(ctx, repo.AdminCountModelsParams{
		Keyword: params.Keyword, IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return Page[repo.AdminListModelsRow]{}, fmt.Errorf("统计模型数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// GetModel 读取单个模型（需求 §17.3「编辑模型」前的回显）。
func (s *Service) GetModel(ctx context.Context, id int64) (repo.Model, error) {
	if err := s.ensure(); err != nil {
		return repo.Model{}, err
	}
	row, err := repo.New(s.pool).AdminGetModel(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Model{}, apperr.New(apperr.CodeNotFound, "")
		}
		return repo.Model{}, fmt.Errorf("查询模型: %w", err)
	}
	return row, nil
}

// CreateModel 手工添加模型（需求 §13.1 第 1 种方式）。
func (s *Service) CreateModel(ctx context.Context, operator domain.User, in ModelInput, ip string) (repo.Model, error) {
	if err := s.ensure(); err != nil {
		return repo.Model{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Model{}, err
	}

	var created repo.Model
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := s.assertModelIdentityFree(ctx, q, normalized.ModelKey, normalized.ProviderKey, 0); err != nil {
			return err
		}
		if err := validateModelCapabilities(ctx, q, normalized.CapabilityIDs); err != nil {
			return err
		}
		row, err := q.InsertModel(ctx, repo.InsertModelParams{
			Name:            normalized.Name,
			ModelKey:        normalized.ModelKey,
			ProviderKey:     normalized.ProviderKey,
			ProviderName:    normalized.ProviderName,
			InputModalities: normalized.InputModalities,
			Aliases:         normalized.Aliases,
			Description:     normalized.Description,
			ReleaseDate:     repoDate(normalized.ReleaseDate),
			ContextLength:   repoInt8(normalized.ContextLength),
			IsEnabled:       normalized.IsEnabled,
			Note:            normalized.Note,
			ImportSource:    "manual",
		})
		if err != nil {
			if isUniqueViolation(err) {
				return apperr.New(apperr.CodeDuplicateModel, "")
			}
			return fmt.Errorf("写入模型: %w", err)
		}
		if err := replaceModelCapabilities(ctx, q, row.ID, normalized.CapabilityIDs); err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return repo.Model{}, err
	}

	if err := s.logModelChange(ctx, operator, oplog.ActionModelImport, created, nil, ip, "手工添加模型"); err != nil {
		return repo.Model{}, err
	}
	return created, nil
}

// UpdateModel 编辑模型（需求 §17.3「编辑模型」）。平台模型 ID 不变。
func (s *Service) UpdateModel(ctx context.Context, operator domain.User, id int64, in ModelInput, ip string) (repo.Model, error) {
	if err := s.ensure(); err != nil {
		return repo.Model{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Model{}, err
	}

	var updated repo.Model
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		current, err := q.AdminGetModel(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询模型: %w", err)
		}
		if current.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "回收站中的模型不可编辑")
		}
		if err := s.assertModelIdentityFree(ctx, q, normalized.ModelKey, normalized.ProviderKey, id); err != nil {
			return err
		}
		if err := validateModelCapabilities(ctx, q, normalized.CapabilityIDs); err != nil {
			return err
		}

		row, err := q.UpdateModel(ctx, repo.UpdateModelParams{
			ID:              id,
			Name:            normalized.Name,
			ModelKey:        normalized.ModelKey,
			ProviderKey:     normalized.ProviderKey,
			ProviderName:    normalized.ProviderName,
			InputModalities: normalized.InputModalities,
			Aliases:         normalized.Aliases,
			Description:     normalized.Description,
			ReleaseDate:     repoDate(normalized.ReleaseDate),
			ContextLength:   repoInt8(normalized.ContextLength),
			IsEnabled:       normalized.IsEnabled,
			Note:            normalized.Note,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return apperr.New(apperr.CodeDuplicateModel, "")
			}
			return fmt.Errorf("更新模型: %w", err)
		}
		if err := replaceModelCapabilities(ctx, q, row.ID, normalized.CapabilityIDs); err != nil {
			return err
		}
		updated = row
		return s.logModelChangeInTx(ctx, q, operator, oplog.ActionModelOverwrite, row, &current, ip, "编辑模型")
	})
	if err != nil {
		return repo.Model{}, err
	}
	return updated, nil
}

// validateModelCapabilities 校验模型选择的考察维度存在且启用。
func validateModelCapabilities(ctx context.Context, q *repo.Queries, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.GetCapabilitiesByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("查询模型考察维度: %w", err)
	}
	if len(rows) != len(ids) {
		return apperr.New(apperr.CodeInvalidClassification, "模型选择的考察维度不存在")
	}
	for _, row := range rows {
		if !row.IsEnabled {
			return apperr.New(apperr.CodeInvalidClassification, "模型选择的考察维度已停用")
		}
	}
	return nil
}

// replaceModelCapabilities 整体替换模型的考察维度集合。
func replaceModelCapabilities(ctx context.Context, q *repo.Queries, modelID int64, ids []int64) error {
	if err := q.ReplaceModelCapabilities(ctx, modelID); err != nil {
		return fmt.Errorf("清空模型考察维度: %w", err)
	}
	for _, capabilityID := range ids {
		if err := q.AttachModelCapability(ctx, repo.AttachModelCapabilityParams{
			ModelID:      modelID,
			CapabilityID: capabilityID,
		}); err != nil {
			return fmt.Errorf("写入模型考察维度: %w", err)
		}
	}
	return nil
}

// ModelCapabilityIDs 返回模型选择的考察维度，供后台详情与写入响应回显。
func (s *Service) ModelCapabilityIDs(ctx context.Context, modelID int64) ([]int64, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	ids, err := repo.New(s.pool).ListModelCapabilityIDs(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("查询模型考察维度: %w", err)
	}
	return ids, nil
}

// ModelCapabilityIDsByModel 批量返回一页模型选择的考察维度，避免逐条查询。
func (s *Service) ModelCapabilityIDsByModel(ctx context.Context, modelIDs []int64) (map[int64][]int64, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	out := make(map[int64][]int64, len(modelIDs))
	if len(modelIDs) == 0 {
		return out, nil
	}
	rows, err := repo.New(s.pool).ListModelCapabilitiesForModels(ctx, modelIDs)
	if err != nil {
		return nil, fmt.Errorf("查询模型考察维度: %w", err)
	}
	for _, row := range rows {
		out[row.ModelID] = append(out[row.ModelID], row.CapabilityID)
	}
	return out, nil
}

// SetModelEnabled 启用或禁用模型（需求 §17.3）。禁用后不再出现在前台字典中。
func (s *Service) SetModelEnabled(ctx context.Context, operator domain.User, id int64, enabled bool, ip string) (repo.Model, error) {
	if err := s.ensure(); err != nil {
		return repo.Model{}, err
	}

	var updated repo.Model
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		current, err := q.AdminGetModel(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询模型: %w", err)
		}
		if current.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "回收站中的模型不可变更启用状态")
		}
		row, err := q.SetModelEnabled(ctx, repo.SetModelEnabledParams{ID: id, IsEnabled: enabled})
		if err != nil {
			return fmt.Errorf("更新模型启用状态: %w", err)
		}
		updated = row
		return s.logModelChangeInTx(ctx, q, operator, oplog.ActionModelOverwrite, row, &current, ip, "变更模型启用状态")
	})
	if err != nil {
		return repo.Model{}, err
	}
	return updated, nil
}

// DeleteModel 删除模型（需求 §17.3）。存在有效评分即视为被引用，拒绝删除（model_in_use）。
func (s *Service) DeleteModel(ctx context.Context, operator domain.User, id int64, ip string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		current, err := q.AdminGetModel(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询模型: %w", err)
		}
		if current.DeletedAt.Valid {
			return apperr.New(apperr.CodeInvalidState, "模型已在回收站中")
		}
		count, err := q.CountModelRatings(ctx, id)
		if err != nil {
			return fmt.Errorf("统计模型引用: %w", err)
		}
		if count > 0 {
			return apperr.New(apperr.CodeModelInUse, fmt.Sprintf("该模型已有 %d 条评分，不可删除", count))
		}
		if _, err := q.SoftDeleteModel(ctx, id); err != nil {
			return fmt.Errorf("删除模型: %w", err)
		}
		return s.logModelChangeInTx(ctx, q, operator, oplog.ActionModelOverwrite, current, nil, ip, "删除模型")
	})
}

// ListModelPrompts 返回引用了该模型的提示词（需求 §17.3「查看模型关联提示词」）。
func (s *Service) ListModelPrompts(ctx context.Context, modelID int64, page, pageSize int32) (Page[repo.AdminListModelPromptsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListModelPromptsRow]{}, err
	}
	if _, err := s.GetModel(ctx, modelID); err != nil {
		return Page[repo.AdminListModelPromptsRow]{}, err
	}
	q := repo.New(s.pool)
	rows, err := q.AdminListModelPrompts(ctx, repo.AdminListModelPromptsParams{
		ModelID: modelID, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListModelPromptsRow]{}, fmt.Errorf("查询模型关联提示词: %w", err)
	}
	total, err := q.AdminCountModelPrompts(ctx, modelID)
	if err != nil {
		return Page[repo.AdminListModelPromptsRow]{}, fmt.Errorf("统计模型关联提示词: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// ListModelStats 返回该模型在各提示词上的完成度统计（需求 §17.3「查看模型完成度统计」）。
func (s *Service) ListModelStats(ctx context.Context, modelID int64, page, pageSize int32) (Page[repo.AdminListModelStatsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListModelStatsRow]{}, err
	}
	if _, err := s.GetModel(ctx, modelID); err != nil {
		return Page[repo.AdminListModelStatsRow]{}, err
	}
	q := repo.New(s.pool)
	rows, err := q.AdminListModelStats(ctx, repo.AdminListModelStatsParams{
		ModelID: modelID, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListModelStatsRow]{}, fmt.Errorf("查询模型完成度统计: %w", err)
	}
	total, err := q.AdminCountModelStats(ctx, modelID)
	if err != nil {
		return Page[repo.AdminListModelStatsRow]{}, fmt.Errorf("统计模型完成度条目: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// ListModelSnapshots 返回模型的原始字段快照，供字段追溯（需求 §13.3）。
func (s *Service) ListModelSnapshots(ctx context.Context, modelID int64, page, pageSize int32) (Page[repo.ModelSnapshot], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.ModelSnapshot]{}, err
	}
	q := repo.New(s.pool)
	rows, err := q.ListModelSnapshots(ctx, repo.ListModelSnapshotsParams{
		ModelID: modelID, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.ModelSnapshot]{}, fmt.Errorf("查询模型快照: %w", err)
	}
	return NewPage(rows, page, pageSize, int64(len(rows))), nil
}

// ListModelImportLogs 返回导入与覆盖记录（需求 §17.3）。
func (s *Service) ListModelImportLogs(ctx context.Context, action string, page, pageSize int32) (Page[repo.AdminListModelImportLogsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListModelImportLogsRow]{}, err
	}
	if action != "" && !isKnownImportAction(action) {
		return Page[repo.AdminListModelImportLogsRow]{}, apperr.New(apperr.CodeValidationFailed, "导入动作取值非法")
	}
	q := repo.New(s.pool)
	rows, err := q.AdminListModelImportLogs(ctx, repo.AdminListModelImportLogsParams{
		Action: action, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListModelImportLogsRow]{}, fmt.Errorf("查询模型导入记录: %w", err)
	}
	total, err := q.AdminCountModelImportLogs(ctx, action)
	if err != nil {
		return Page[repo.AdminListModelImportLogsRow]{}, fmt.Errorf("统计模型导入记录: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// assertModelIdentityFree 校验「模型标识 + 模型厂商」未被其他在用模型占用（需求 §13.4）。
// excludeID 为当前正在编辑的模型，避免把自身判为重复。
func (s *Service) assertModelIdentityFree(ctx context.Context, q *repo.Queries, modelKey, providerKey string, excludeID int64) error {
	existing, err := q.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: modelKey, ProviderKey: providerKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("查询同名模型: %w", err)
	}
	if existing.ID == excludeID {
		return nil
	}
	return apperr.New(apperr.CodeDuplicateModel,
		fmt.Sprintf("已存在相同模型标识与厂商的模型：%s / %s", existing.Name, existing.ProviderName))
}

// logModelChange 在独立事务中写模型变更日志。
func (s *Service) logModelChange(
	ctx context.Context, operator domain.User, action string, after repo.Model, before *repo.Model, ip, note string,
) error {
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		return s.logModelChangeInTx(ctx, q, operator, action, after, before, ip, note)
	})
}

// logModelChangeInTx 写一条模型变更日志。对象标识取平台模型 ID，
// 覆盖时不更换 ID（需求 §13.4 第 5 条）。
func (s *Service) logModelChangeInTx(
	ctx context.Context, q *repo.Queries, operator domain.User, action string,
	after repo.Model, before *repo.Model, ip, note string,
) error {
	entry := oplog.Entry{
		OperatorID: operator.ID,
		Action:     action,
		ObjectType: "model",
		ObjectID:   after.ID,
		After:      modelSnapshot(after),
		Note:       note,
		IP:         ip,
	}
	if before != nil {
		entry.Before = modelSnapshot(*before)
	}
	return s.record(ctx, q, entry)
}

// modelSnapshot 是写入日志的模型摘要。原始条目另有 model_snapshots 表承载。
func modelSnapshot(m repo.Model) map[string]any {
	return map[string]any{
		"id":            m.ID,
		"name":          m.Name,
		"model_key":     m.ModelKey,
		"provider_key":  m.ProviderKey,
		"provider_name": m.ProviderName,
		"is_enabled":    m.IsEnabled,
		"import_source": m.ImportSource,
	}
}

// repoDate 把可选日期转为可空列取值。SQL 侧已按日期截断，时间部分不参与存储。
func repoDate(v *time.Time) pgtype.Date {
	if v == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *v, Valid: true}
}

// repoInt8 把可选整数转为可空列取值。
func repoInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// isUniqueViolation 判定错误是否为唯一约束冲突。
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}

func isKnownImportAction(action string) bool {
	switch domain.ModelImportAction(action) {
	case domain.ModelImportCreated, domain.ModelImportOverwrite, domain.ModelImportSkipped:
		return true
	default:
		return false
	}
}
