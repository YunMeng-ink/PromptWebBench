package library

import (
	"context"
	"strings"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Export 是题库导出的结果。
type Export struct {
	// Filename 是建议的下载文件名。
	Filename string
	// Content 是 Markdown 全文。
	Content string
}

// ExportLibrary 把题库导出为 Markdown（需求 §10.8）。
//
// 内容含 domain.LibraryExportElements 列出的 14 项要素。导出只收录当前身份可见的
// 题目：来源提示词为开发者可见或高风险时，普通用户协作者既读不到也导不出
// （开发方案 §6.6 专项回归的导出侧）。
func (s *Service) ExportLibrary(ctx context.Context, v domain.Viewer, libraryID int64) (Export, error) {
	if err := s.ensure(); err != nil {
		return Export{}, notFound("题库")
	}

	q := repo.New(s.pool)
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
	if err != nil {
		return Export{}, err
	}
	if err := requireCapability(actor, domain.CapExportLibrary, "当前身份不可以导出题库"); err != nil {
		return Export{}, err
	}

	questions, err := s.visibleQuestions(ctx, q, actor, lib, false)
	if err != nil {
		return Export{}, err
	}
	notes, err := s.latestRevisionNotes(ctx, q, questions)
	if err != nil {
		return Export{}, err
	}
	ownerName, err := s.actorName(ctx, q, lib.OwnerID)
	if err != nil {
		return Export{}, err
	}

	var builder strings.Builder
	writeLibraryHeader(&builder, lib, ownerName, s.now().UTC().Format("2006-01-02T15:04:05Z07:00"), len(questions))
	for index, question := range questions {
		writeQuestion(&builder, index+1, question, notes[question.ID])
	}
	return Export{
		Filename: exportFilename(lib.Name),
		Content:  builder.String(),
	}, nil
}

// writeLibraryHeader 写出题库层的五项要素：名称、描述、创建者、导出时间、题目数量。
func writeLibraryHeader(builder *strings.Builder, lib domain.Library, ownerName, exportedAt string, count int) {
	builder.WriteString("# " + lib.Name + "\n\n")
	builder.WriteString("- 题库描述：" + blankIfEmpty(lib.Description) + "\n")
	builder.WriteString("- 创建者：" + blankIfEmpty(ownerName) + "\n")
	builder.WriteString("- 导出时间：" + exportedAt + "\n")
	builder.WriteString("- 题目数量：" + itoa(count) + "\n\n")
}

// writeQuestion 写出单道题目的九项要素：顺序、标题、类别、标签、正文、说明、
// 来源提示词、来源版本、修改说明。
func writeQuestion(builder *strings.Builder, order int, question Question, derivationNote string) {
	builder.WriteString("## " + itoa(order) + ". " + blankIfEmpty(question.Title) + "\n\n")
	builder.WriteString("- 题目类别：" + categoryLabel(question) + "\n")
	builder.WriteString("- 题目标签：" + tagsLabel(question.Tags) + "\n")
	builder.WriteString("- 题目类型：" + kindLabel(question.Kind) + "\n")
	builder.WriteString("- 来源提示词：" + sourceLabel(question) + "\n")
	builder.WriteString("- 来源版本：" + blankIfEmpty(question.SourceVersionNo) + "\n")
	builder.WriteString("- 修改说明：" + derivationLabel(question, derivationNote) + "\n\n")
	builder.WriteString("### 提示词正文\n\n")
	builder.WriteString(blankIfEmpty(question.Body))
	builder.WriteString("\n\n### 题目说明\n\n")
	builder.WriteString(blankIfEmpty(question.Description))
	builder.WriteString("\n\n")
}

// categoryLabel 返回题目的类别名称。两类名称都为空时给出占位符。
func categoryLabel(question Question) string {
	if question.CategoryNameZh != "" {
		return question.CategoryNameZh
	}
	if question.CategoryNameEn != "" {
		return question.CategoryNameEn
	}
	return "—"
}

// tagsLabel 把标签拼成一行。
func tagsLabel(tags []string) string {
	if len(tags) == 0 {
		return "—"
	}
	return strings.Join(tags, "、")
}

// kindLabel 返回题目类型的中文名称。
func kindLabel(kind domain.LibraryQuestionKind) string {
	if kind == domain.QuestionReference {
		return "引用题"
	}
	return "派生题"
}

// sourceLabel 返回来源提示词的标识。引用题必带来源，派生题的来源被删除后不再有标识。
func sourceLabel(question Question) string {
	if question.SourcePromptID == nil {
		return "—"
	}
	return "#" + itoa64(*question.SourcePromptID)
}

// derivationLabel 返回修改说明。引用题未修改，说明其跟随来源提示词最新版本
// （需求 §7.4），来源已被删除时改为提示该题目等待清理；派生题取最近一次修改记录的说明。
func derivationLabel(question Question, note string) string {
	if question.Kind == domain.QuestionReference {
		if !question.SourceAvailable {
			return "来源提示词已删除，可删除该题目以归还题库容量"
		}
		return "未修改，跟随来源提示词最新版本"
	}
	return blankIfEmpty(note)
}

// latestRevisionNotes 批量取每道题的最近一次修改说明。
func (s *Service) latestRevisionNotes(
	ctx context.Context, q *repo.Queries, questions []Question,
) (map[int64]string, error) {
	out := make(map[int64]string)
	ids := make([]int64, 0, len(questions))
	for _, question := range questions {
		if question.Kind == domain.QuestionDerived {
			ids = append(ids, question.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListLatestLibraryQuestionRevisionNotes(ctx, ids)
	if err != nil {
		return nil, wrap("查询题目修改说明", 0, err)
	}
	for _, row := range rows {
		out[row.QuestionID] = row.Note
	}
	return out, nil
}

// blankIfEmpty 把空值写成占位符，使导出文件的每一项要素都有内容可读。
func blankIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

// exportFilename 由题库名称生成下载文件名。文件名只保留可安全落盘的字符。
func exportFilename(name string) string {
	replaced := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return '-'
		default:
			return r
		}
	}, name)
	trimmed := strings.TrimSpace(replaced)
	if trimmed == "" {
		trimmed = "library"
	}
	return trimmed + ".md"
}
