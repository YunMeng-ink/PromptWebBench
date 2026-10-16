package admin_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/admin"
)

// ---------- 模块 3：模型管理 ----------

func TestModelCreateUpdateAndDelete(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	created, err := h.service.CreateModel(ctx, operator, admin.ModelInput{
		Name: "手工模型", ModelKey: "manual-model", ProviderKey: "manual-lab",
		ProviderName: "手工厂商", InputModalities: []string{"text", "text", "  image "},
		Aliases: []string{"别名一"}, Description: "说明", ContextLength: ptrInt64(32000),
		IsEnabled: true, AbilityTags: []string{"coding"}, Note: "备注",
	}, "")
	if err != nil {
		t.Fatalf("添加模型失败: %v", err)
	}
	if created.ImportSource != "manual" {
		t.Fatalf("导入来源 = %s，期望 manual", created.ImportSource)
	}
	// 输入模态去空白、去重复，保持首次出现的顺序。
	if len(created.InputModalities) != 2 || created.InputModalities[0] != "text" ||
		created.InputModalities[1] != "image" {
		t.Fatalf("输入模态 = %v，期望 [text image]", created.InputModalities)
	}

	updated, err := h.service.UpdateModel(ctx, operator, created.ID, admin.ModelInput{
		Name: "手工模型（改）", ModelKey: "manual-model", ProviderKey: "manual-lab",
		ProviderName: "手工厂商", IsEnabled: false,
	}, "")
	if err != nil {
		t.Fatalf("编辑模型失败: %v", err)
	}
	if updated.Name != "手工模型（改）" || updated.IsEnabled {
		t.Fatalf("编辑结果 = %+v，期望改名且停用", updated)
	}
	if updated.ID != created.ID {
		t.Fatal("编辑应保留平台模型 ID")
	}

	if _, err := h.service.SetModelEnabled(ctx, operator, created.ID, true, ""); err != nil {
		t.Fatalf("启用模型失败: %v", err)
	}
	if err := h.service.DeleteModel(ctx, operator, created.ID, ""); err != nil {
		t.Fatalf("删除模型失败: %v", err)
	}
	if h.scalar(t, `SELECT count(*) FROM models WHERE id = $1 AND deleted_at IS NOT NULL`, created.ID) != 1 {
		t.Fatal("删除应是软删除")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE object_type = 'model'`) != 4 {
		t.Fatal("模型的新增、编辑、启用与删除都应写入操作日志")
	}
}

func TestModelDuplicateIdentityIsRejected(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	input := admin.ModelInput{Name: "重复模型", ModelKey: "dup-model", ProviderKey: "dup-lab"}
	if _, err := h.service.CreateModel(ctx, operator, input, ""); err != nil {
		t.Fatalf("首次添加失败: %v", err)
	}
	_, err := h.service.CreateModel(ctx, operator, admin.ModelInput{
		Name: "另一个名字", ModelKey: "dup-model", ProviderKey: "dup-lab",
	}, "")
	expectCode(t, err, apperr.CodeDuplicateModel)
}

func TestModelDeleteRejectsReferencedModel(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "modelowner@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "带评分的提示词")
	modelID := h.seedModel(t, "被引用模型", "used-model", "used-lab")
	h.seedRating(t, ownerID, promptID, modelID, 3)

	err := h.service.DeleteModel(ctx, operator, modelID, "")
	expectCode(t, err, apperr.CodeModelInUse)
}

func TestModelInputValidationRejectsBadValues(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	_, err := h.service.CreateModel(ctx, operator, admin.ModelInput{ModelKey: "no-name"}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	_, err = h.service.CreateModel(ctx, operator, admin.ModelInput{Name: "有名字", ModelKey: ""}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	longNote := strings.Repeat("备", domain.ModelNoteMaxRunes+1)
	_, err = h.service.CreateModel(ctx, operator, admin.ModelInput{
		Name: "有名字", ModelKey: "k", Note: longNote,
	}, "")
	expectCode(t, err, apperr.CodeValidationFailed)
}

// ---------- models.dev 导入（需求 §13.3、§13.4） ----------

func TestImportModelsMapsFieldsAndStoresSnapshots(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	result, err := h.service.ImportModels(ctx, operator, admin.ModelImportOptions{Overwrite: true}, "")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if result.Total != 3 || result.Created != 3 || result.Skipped != 0 {
		t.Fatalf("导入结果 = %+v，期望 3 条全部新建", result)
	}

	// 字段按 C-45 补充裁定的路径取值。
	row, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "alpha-one", ProviderKey: "alpha",
	})
	if err != nil {
		t.Fatalf("查询导入模型失败: %v", err)
	}
	if row.Name != "Alpha One" || row.ProviderName != "Alpha Lab" {
		t.Fatalf("名称与厂商 = %s / %s", row.Name, row.ProviderName)
	}
	if len(row.InputModalities) != 2 || row.InputModalities[0] != "text" {
		t.Fatalf("输入模态 = %v，期望来自 modalities.input", row.InputModalities)
	}
	if !row.ContextLength.Valid || row.ContextLength.Int64 != 128000 {
		t.Fatalf("上下文长度 = %+v，期望 128000", row.ContextLength)
	}
	if !row.ReleaseDate.Valid || row.ReleaseDate.Time.Format("2006-01-02") != "2025-01-31" {
		t.Fatalf("发布时间 = %+v，期望 2025-01-31", row.ReleaseDate)
	}
	if row.Description != "第一个模型" {
		t.Fatalf("简介 = %s", row.Description)
	}
	if row.ImportSource != "models_dev" {
		t.Fatalf("导入来源 = %s，期望 models_dev", row.ImportSource)
	}

	// 缺少 modalities 的条目导入为空值而不是报错（需求 §13.3）。
	empty, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "alpha-two", ProviderKey: "alpha",
	})
	if err != nil {
		t.Fatalf("查询缺少模态的模型失败: %v", err)
	}
	if len(empty.InputModalities) != 0 {
		t.Fatalf("缺少 modalities 的条目输入模态 = %v，期望空", empty.InputModalities)
	}
	// 缺少 limit 的条目上下文长度为空。
	noLimit, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "beta-one", ProviderKey: "beta",
	})
	if err != nil {
		t.Fatalf("查询缺少窗口的模型失败: %v", err)
	}
	if noLimit.ContextLength.Valid {
		t.Fatalf("缺少 limit 的条目上下文长度 = %+v，期望空", noLimit.ContextLength)
	}

	// 原始条目逐条落库，供字段追溯。
	if h.scalar(t, `SELECT count(*) FROM model_snapshots`) != 3 {
		t.Fatal("每个导入条目都应保存原始快照")
	}
	if h.scalar(t, `SELECT count(*) FROM model_import_logs WHERE action = 'import'`) != 3 {
		t.Fatal("每个导入条目都应写入导入记录")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'model_import'`) != 1 {
		t.Fatal("导入应写入 model_import 操作日志")
	}
}

func TestImportModelsSkipsOrOverwritesDuplicates(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	if _, err := h.service.ImportModels(ctx, operator, admin.ModelImportOptions{}, ""); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	existing, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "alpha-one", ProviderKey: "alpha",
	})
	if err != nil {
		t.Fatalf("查询导入模型失败: %v", err)
	}

	// 第二次导入且不覆盖：全部跳过。
	skipped, err := h.service.ImportModels(ctx, operator, admin.ModelImportOptions{}, "")
	if err != nil {
		t.Fatalf("重复导入失败: %v", err)
	}
	if skipped.Created != 0 || skipped.Skipped != 3 {
		t.Fatalf("跳过的导入结果 = %+v，期望 0 新建、3 跳过", skipped)
	}
	if h.scalar(t, `SELECT count(*) FROM model_import_logs WHERE action = 'skip'`) != 3 {
		t.Fatal("跳过也应留痕（需求 §13.4）")
	}

	// 预览标出重复项。
	preview, err := h.service.PreviewModelImport(ctx)
	if err != nil {
		t.Fatalf("预览导入失败: %v", err)
	}
	if len(preview) != 3 {
		t.Fatalf("预览条目 = %d，期望 3", len(preview))
	}
	for _, entry := range preview {
		if entry.Duplicate == nil {
			t.Fatalf("条目 %s 应被标记为重复", entry.ModelID)
		}
	}

	// 第三次导入且允许覆盖：保留平台模型 ID。
	overwritten, err := h.service.ImportModels(ctx, operator, admin.ModelImportOptions{Overwrite: true}, "")
	if err != nil {
		t.Fatalf("覆盖导入失败: %v", err)
	}
	if overwritten.Overwritten != 3 || overwritten.Created != 0 {
		t.Fatalf("覆盖导入结果 = %+v，期望 3 覆盖、0 新建", overwritten)
	}
	after, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "alpha-one", ProviderKey: "alpha",
	})
	if err != nil {
		t.Fatalf("查询覆盖后的模型失败: %v", err)
	}
	if after.ID != existing.ID {
		t.Fatalf("覆盖后平台模型 ID = %d，期望保留 %d（需求 §13.4 第 5 条）", after.ID, existing.ID)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'model_overwrite'`) != 1 {
		t.Fatal("覆盖应写入 model_overwrite 操作日志")
	}
}

func TestImportModelsReportsBadGatewayWhenUpstreamFails(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	h.catalog.set(http.StatusInternalServerError, "upstream boom")
	_, err := h.service.ImportModels(ctx, operator, admin.ModelImportOptions{}, "")
	expectCode(t, err, apperr.CodeBadGateway)

	h.catalog.set(http.StatusOK, "{不是 JSON")
	_, err = h.service.ImportModels(ctx, operator, admin.ModelImportOptions{}, "")
	expectCode(t, err, apperr.CodeBadGateway)

	h.catalog.set(http.StatusOK, `{}`)
	_, err = h.service.ImportModels(ctx, operator, admin.ModelImportOptions{}, "")
	expectCode(t, err, apperr.CodeBadGateway)

	if h.catalog.requests() != 3 {
		t.Fatalf("上游访问次数 = %d，期望 3", h.catalog.requests())
	}
}

// ---------- 模块 5：标签管理 ----------

func TestTagCrudAndUsageCount(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	created, err := h.service.CreateTag(ctx, admin.TagInput{
		Slug: "phase6-tag", NameZh: "阶段六标签", NameEn: "Phase Six Tag",
		Color: "#123456", Description: "说明", Priority: 5, IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "tagged@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "带标签的提示词")
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO prompt_tags (prompt_id, tag_id) VALUES ($1, $2)`, promptID, created.ID); err != nil {
		t.Fatalf("写入提示词标签失败: %v", err)
	}

	list, err := h.service.ListTags(ctx, "阶段六", 1, 20)
	if err != nil {
		t.Fatalf("查询标签失败: %v", err)
	}
	if list.Total != 1 || list.Items[0].UsageCount != 1 {
		t.Fatalf("标签列表 = %+v，期望 1 条且使用数为 1", list.Items)
	}

	if _, err := h.service.UpdateTag(ctx, created.ID, admin.TagInput{
		Slug: "phase6-tag", NameZh: "阶段六标签", NameEn: "Phase Six Tag",
		Color: "#654321", Priority: 9, IsEnabled: false,
	}); err != nil {
		t.Fatalf("编辑标签失败: %v", err)
	}
	if color := h.scalarString(t, `SELECT color FROM tags WHERE id = $1`, created.ID); color != "#654321" {
		t.Fatalf("颜色 = %s，期望被更新", color)
	}

	if err := h.service.DeleteTag(ctx, created.ID); err != nil {
		t.Fatalf("删除标签失败: %v", err)
	}
	if h.scalar(t, `SELECT count(*) FROM tags WHERE id = $1`, created.ID) != 0 {
		t.Fatal("删除标签后行应消失")
	}
	if h.scalar(t, `SELECT count(*) FROM prompt_tags WHERE tag_id = $1`, created.ID) != 0 {
		t.Fatal("删除标签应级联清除提示词标签关联")
	}
}

func TestTagValidationRejectsBadInput(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	_, err := h.service.CreateTag(ctx, admin.TagInput{Slug: "", NameZh: "甲", NameEn: "A", Color: "#ffffff"})
	expectCode(t, err, apperr.CodeValidationFailed)

	_, err = h.service.CreateTag(ctx, admin.TagInput{Slug: "s", NameZh: "甲", NameEn: "A", Color: "red"})
	expectCode(t, err, apperr.CodeValidationFailed)

	if _, err := h.service.CreateTag(ctx, admin.TagInput{
		Slug: "dup-slug", NameZh: "甲", NameEn: "A", Color: "#ffffff",
	}); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	_, err = h.service.CreateTag(ctx, admin.TagInput{
		Slug: "dup-slug", NameZh: "乙", NameEn: "B", Color: "#ffffff",
	})
	expectCode(t, err, apperr.CodeConflict)
}

func ptrInt64(v int64) *int64 { return &v }
