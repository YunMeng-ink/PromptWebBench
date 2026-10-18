package admin

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/modelsdev"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// 导入预览的筛选状态。
const (
	ModelImportStatusAll       = ""
	ModelImportStatusNew       = "new"
	ModelImportStatusDuplicate = "duplicate"
)

// maxImportSelection 是一次导入允许的条目上限。
// models.dev 当前目录约八千条，留出一倍余量足以覆盖目录增长。
const maxImportSelection = 20000

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

// ModelImportFilter 是预览与选择性导入共用的筛选条件。
type ModelImportFilter struct {
	// Keyword 匹配模型名称、模型标识、厂商名称与厂商标识。
	Keyword string
	// ProviderID 限定单个提供商。
	ProviderID string
	// Status 取 new、duplicate 或空串（全部）。
	Status string
}

// ModelImportPreviewParams 是预览请求。分页与筛选都在服务端完成，
// 浏览器只接收当前页的条目。
type ModelImportPreviewParams struct {
	Filter   ModelImportFilter
	Page     int32
	PageSize int32
	// Refresh 为真时丢弃缓存并重新读取 models.dev。
	Refresh bool
}

// ModelImportProvider 是提供商维度的统计，用于筛选下拉与摘要。
type ModelImportProvider struct {
	ID         string
	Name       string
	Total      int
	Duplicates int
}

// ModelImportPreview 是一页预览结果。
type ModelImportPreview struct {
	Items          []ModelImportEntry
	Page           int32
	PageSize       int32
	Total          int
	CatalogTotal   int
	DuplicateCount int
	Providers      []ModelImportProvider
	FetchedAt      time.Time
}

// ModelImportKey 用「模型标识 + 厂商标识」定位一条目录条目。
type ModelImportKey struct {
	ProviderID string
	ModelID    string
}

// ModelImportSelection 描述本次要导入的条目。
// mode=keys 时按 Keys 精确导入；mode=filter（或空）时导入匹配 Filter 的全部条目，
// 并可减去 Exclude 中的条目。
type ModelImportSelection struct {
	Mode    string
	Keys    []ModelImportKey
	Filter  ModelImportFilter
	Exclude []ModelImportKey
}

// ModelImportOptions 是执行导入的选项。Overwrite 为真表示覆盖重复模型，
// 为假表示跳过重复模型（需求 §13.4 第 3、4 条）。
type ModelImportOptions struct {
	Overwrite bool
	Selection ModelImportSelection
}

// ModelImportResult 是一次导入的结果统计。
type ModelImportResult struct {
	Total       int
	Created     int
	Overwritten int
	Skipped     int
}

// PreviewModelImport 读取 models.dev 目录并返回一页条目，同时标出重复项
// （需求 §13.4「二次确认」）。预览不写入任何数据。
func (s *Service) PreviewModelImport(ctx context.Context, params ModelImportPreviewParams) (ModelImportPreview, error) {
	if err := s.ensure(); err != nil {
		return ModelImportPreview{}, err
	}
	filter, err := normalizeImportFilter(params.Filter)
	if err != nil {
		return ModelImportPreview{}, err
	}
	snapshot, err := s.catalog(ctx, params.Refresh)
	if err != nil {
		return ModelImportPreview{}, err
	}
	existing, err := s.modelsForImport(ctx, repo.New(s.pool))
	if err != nil {
		return ModelImportPreview{}, err
	}

	page, pageSize := importPageValues(params.Page, params.PageSize)
	start := int64(page-1) * int64(pageSize)
	end := start + int64(pageSize)

	providers := make(map[string]*ModelImportProvider, 64)
	duplicateCount := 0
	matched := int64(0)
	items := make([]ModelImportEntry, 0, pageSize)

	for _, entry := range snapshot.entries {
		key := identityKey(entry.ModelID, entry.ProviderID)
		duplicate, isDuplicate := existing[key]
		if isDuplicate {
			duplicateCount++
		}

		agg := providers[entry.ProviderID]
		if agg == nil {
			name := entry.ProviderName
			if name == "" {
				name = entry.ProviderID
			}
			agg = &ModelImportProvider{ID: entry.ProviderID, Name: name}
			providers[entry.ProviderID] = agg
		}
		agg.Total++
		if isDuplicate {
			agg.Duplicates++
		}

		if !importEntryMatches(entry, isDuplicate, filter) {
			continue
		}
		if matched >= start && matched < end {
			items = append(items, toModelImportEntry(entry, duplicate, isDuplicate))
		}
		matched++
	}

	return ModelImportPreview{
		Items:          items,
		Page:           page,
		PageSize:       pageSize,
		Total:          int(matched),
		CatalogTotal:   len(snapshot.entries),
		DuplicateCount: duplicateCount,
		Providers:      sortedImportProviders(providers),
		FetchedAt:      snapshot.fetchedAt,
	}, nil
}

// ImportModels 执行导入（需求 §13.3、§13.4）。
//
// 只处理选择范围内的条目：无同名模型则新建；有同名模型且允许覆盖则覆盖并保留
// 平台模型 ID；有同名模型且不允许覆盖则跳过。每条动作都写入 model_import_logs
// 与模型快照。任何一条失败整体回滚，不留下「导入了一半」的目录。
func (s *Service) ImportModels(
	ctx context.Context, operator domain.User, opts ModelImportOptions, ip string,
) (ModelImportResult, error) {
	if err := s.ensure(); err != nil {
		return ModelImportResult{}, err
	}
	snapshot, err := s.catalog(ctx, false)
	if err != nil {
		return ModelImportResult{}, err
	}

	result := ModelImportResult{}
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		existing, err := s.modelsForImport(ctx, q)
		if err != nil {
			return err
		}
		entries, err := resolveImportEntries(snapshot.entries, existing, opts.Selection)
		if err != nil {
			return err
		}
		result.Total = len(entries)

		for _, entry := range entries {
			action, err := s.importEntry(ctx, q, operator, entry, opts.Overwrite, existing)
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

// resolveImportEntries 把选择范围解析为具体的目录条目。
func resolveImportEntries(
	entries []modelsdev.Entry, existing map[string]repo.Model, selection ModelImportSelection,
) ([]modelsdev.Entry, error) {
	mode := strings.ToLower(trimSpace(selection.Mode))
	switch mode {
	case "", "filter":
		filter, err := normalizeImportFilter(selection.Filter)
		if err != nil {
			return nil, err
		}
		excluded := make(map[string]struct{}, len(selection.Exclude))
		for _, key := range selection.Exclude {
			excluded[identityKey(trimSpace(key.ModelID), trimSpace(key.ProviderID))] = struct{}{}
		}

		out := make([]modelsdev.Entry, 0, 256)
		for _, entry := range entries {
			if _, skip := excluded[identityKey(entry.ModelID, entry.ProviderID)]; skip {
				continue
			}
			_, duplicate := existing[identityKey(entry.ModelID, entry.ProviderID)]
			if !importEntryMatches(entry, duplicate, filter) {
				continue
			}
			out = append(out, entry)
		}
		if len(out) == 0 {
			return nil, apperr.New(apperr.CodeValidationFailed, "当前筛选没有可导入的模型")
		}
		if len(out) > maxImportSelection {
			return nil, apperr.New(apperr.CodeValidationFailed,
				fmt.Sprintf("单次最多导入 %d 条模型，请缩小筛选范围", maxImportSelection))
		}
		return out, nil
	case "keys":
		if len(selection.Keys) == 0 {
			return nil, apperr.New(apperr.CodeValidationFailed, "请至少选择一个模型")
		}
		if len(selection.Keys) > maxImportSelection {
			return nil, apperr.New(apperr.CodeValidationFailed,
				fmt.Sprintf("单次最多导入 %d 条模型，请减少选择", maxImportSelection))
		}
		index := make(map[string]modelsdev.Entry, len(entries))
		for _, entry := range entries {
			index[identityKey(entry.ModelID, entry.ProviderID)] = entry
		}

		out := make([]modelsdev.Entry, 0, len(selection.Keys))
		seen := make(map[string]struct{}, len(selection.Keys))
		for _, key := range selection.Keys {
			providerID := trimSpace(key.ProviderID)
			modelID := trimSpace(key.ModelID)
			composite := identityKey(modelID, providerID)
			if _, duplicate := seen[composite]; duplicate {
				continue
			}
			seen[composite] = struct{}{}
			entry, ok := index[composite]
			if !ok {
				return nil, apperr.New(apperr.CodeValidationFailed,
					"目录已更新，请重新加载导入预览后再试")
			}
			out = append(out, entry)
		}
		return out, nil
	default:
		return nil, apperr.New(apperr.CodeValidationFailed, "导入选择方式非法")
	}
}

// importEntry 处理一条条目，返回实际发生的动作。
// existing 由调用方一次载入，避免逐条查询同名模型。
func (s *Service) importEntry(
	ctx context.Context, q *repo.Queries, operator domain.User,
	entry modelsdev.Entry, overwrite bool, existing map[string]repo.Model,
) (domain.ModelImportAction, error) {
	modalities, err := domain.NormalizeStringList(entry.InputModalities, "输入模态")
	if err != nil {
		return "", apperr.New(apperr.CodeValidationFailed,
			fmt.Sprintf("模型 %s 的输入模态非法：%s", entry.ModelID, err.Error()))
	}

	key := identityKey(entry.ModelID, entry.ProviderID)
	found, isDuplicate := existing[key]
	switch {
	case isDuplicate && !overwrite:
		// 需求 §13.4 第 4 条：管理员选择跳过导入，仍然留痕。
		return domain.ModelImportSkipped, s.insertImportLog(ctx, q, operator, domain.ModelImportSkipped, entry, &found.ID)
	case isDuplicate:
		row, err := q.UpdateModel(ctx, repo.UpdateModelParams{
			ID:              found.ID,
			Name:            entry.Name,
			ModelKey:        entry.ModelID,
			ProviderKey:     entry.ProviderID,
			ProviderName:    entry.ProviderName,
			InputModalities: modalities,
			Aliases:         found.Aliases,
			Description:     entry.Description,
			ReleaseDate:     repoDate(parseReleaseDate(entry.ReleaseDate)),
			ContextLength:   repoInt8(entry.ContextLength),
			IsEnabled:       found.IsEnabled,
			Note:            found.Note,
		})
		if err != nil {
			return "", wrap("覆盖模型", found.ID, err)
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

// modelsForImport 一次取出全部在用模型，按「模型标识 + 厂商」建立索引。
func (s *Service) modelsForImport(ctx context.Context, q *repo.Queries) (map[string]repo.Model, error) {
	rows, err := q.ListModelsForImport(ctx)
	if err != nil {
		return nil, wrap("查询模型身份", 0, err)
	}
	out := make(map[string]repo.Model, len(rows))
	for _, row := range rows {
		out[identityKey(row.ModelKey, row.ProviderKey)] = row
	}
	return out, nil
}

// identityKey 是重复判断的键：模型标识 + 模型厂商（需求 §13.4）。
func identityKey(modelKey, providerKey string) string {
	return modelKey + "\x00" + providerKey
}

// normalizeImportFilter 规范化筛选条件，并拒绝未登记的状态取值。
func normalizeImportFilter(filter ModelImportFilter) (ModelImportFilter, error) {
	out := ModelImportFilter{
		Keyword:    trimSpace(filter.Keyword),
		ProviderID: trimSpace(filter.ProviderID),
		Status:     strings.ToLower(trimSpace(filter.Status)),
	}
	switch out.Status {
	case ModelImportStatusAll, ModelImportStatusNew, ModelImportStatusDuplicate:
		return out, nil
	default:
		return ModelImportFilter{}, apperr.New(apperr.CodeValidationFailed, "导入筛选状态非法")
	}
}

// importEntryMatches 判定一条目录条目是否符合筛选条件。
func importEntryMatches(entry modelsdev.Entry, duplicate bool, filter ModelImportFilter) bool {
	if filter.ProviderID != "" && entry.ProviderID != filter.ProviderID {
		return false
	}
	switch filter.Status {
	case ModelImportStatusNew:
		if duplicate {
			return false
		}
	case ModelImportStatusDuplicate:
		if !duplicate {
			return false
		}
	}
	if filter.Keyword == "" {
		return true
	}
	needle := strings.ToLower(filter.Keyword)
	return strings.Contains(strings.ToLower(entry.Name), needle) ||
		strings.Contains(strings.ToLower(entry.ModelID), needle) ||
		strings.Contains(strings.ToLower(entry.ProviderName), needle) ||
		strings.Contains(strings.ToLower(entry.ProviderID), needle)
}

// importPageValues 把分页参数限制在可服务范围内。
func importPageValues(page, pageSize int32) (int32, int32) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func toModelImportEntry(entry modelsdev.Entry, duplicate repo.Model, isDuplicate bool) ModelImportEntry {
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
	if isDuplicate {
		item.Duplicate = &DuplicateModel{
			ID: duplicate.ID, Name: duplicate.Name, ProviderName: duplicate.ProviderName,
		}
	}
	return item
}

func sortedImportProviders(providers map[string]*ModelImportProvider) []ModelImportProvider {
	out := make([]ModelImportProvider, 0, len(providers))
	for _, provider := range providers {
		out = append(out, *provider)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Name)
		right := strings.ToLower(out[j].Name)
		if left == right {
			return out[i].ID < out[j].ID
		}
		return left < right
	})
	return out
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
