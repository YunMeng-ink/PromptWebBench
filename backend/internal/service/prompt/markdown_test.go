package prompt

import (
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
)

func TestParseMarkdownExtractsTitleAndBody(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantTitle string
		wantBody  string
		wantDesc  string
	}{
		{
			name:      "首个一级标题作为标题",
			content:   "# 逻辑推理题目\n\n请判断以下命题的真假。\n\n## 说明\n\n细则在此。\n",
			wantTitle: "逻辑推理题目",
			wantBody:  "# 逻辑推理题目\n\n请判断以下命题的真假。\n\n## 说明\n\n细则在此。",
		},
		{
			name:      "Front Matter 的 title 优先于一级标题",
			content:   "---\ntitle: 来自 Front Matter 的标题\ndescription: 来自 Front Matter 的描述\n---\n\n# 一级标题\n\n正文内容。\n",
			wantTitle: "来自 Front Matter 的标题",
			wantBody:  "# 一级标题\n\n正文内容。",
			wantDesc:  "来自 Front Matter 的描述",
		},
		{
			name:      "没有一级标题时回退到 Front Matter 的 title",
			content:   "---\ntitle: 仅 Front Matter 有标题\n---\n\n正文第一段。\n",
			wantTitle: "仅 Front Matter 有标题",
			wantBody:  "正文第一段。",
		},
		{
			name:      "标题含行内标记时保留原文",
			content:   "# 使用 `go` 与 **加粗** 的标题\n\n正文。\n",
			wantTitle: "使用 go 与 加粗 的标题",
			wantBody:  "# 使用 `go` 与 **加粗** 的标题\n\n正文。",
		},
		{
			name:      "二级标题不作为标题",
			content:   "## 只是二级标题\n\n正文。\n",
			wantTitle: "",
			wantBody:  "## 只是二级标题\n\n正文。",
		},
	}

	for _, tc := range cases {
		got, err := ParseMarkdown(tc.content)
		if tc.wantTitle == "" {
			assertAppErrCode(t, err, apperr.CodeValidationFailed)
			continue
		}
		if err != nil {
			t.Errorf("%s：解析失败 %v", tc.name, err)
			continue
		}
		if got.Title != tc.wantTitle {
			t.Errorf("%s：标题 = %q，期望 %q", tc.name, got.Title, tc.wantTitle)
		}
		if got.Body != tc.wantBody {
			t.Errorf("%s：正文 = %q，期望 %q", tc.name, got.Body, tc.wantBody)
		}
		if got.Description != tc.wantDesc {
			t.Errorf("%s：描述 = %q，期望 %q", tc.name, got.Description, tc.wantDesc)
		}
	}
}

func TestParseMarkdownRejectsEmptyAndOversized(t *testing.T) {
	if _, err := ParseMarkdown("   \n\n  "); err == nil {
		t.Error("空白文件应被拒绝")
	}
	if _, err := ParseMarkdown(""); err == nil {
		t.Error("空文件应被拒绝")
	}

	// 没有标题、也没有 Front Matter 的文件无法定位标题，交由用户手工填写。
	if _, err := ParseMarkdown("只有正文没有标题。"); err == nil {
		t.Error("无法识别标题时应被拒绝")
	}

	oversized := "# 标题\n" + strings.Repeat("x", ImportLimit+1)
	if _, err := ParseMarkdown(oversized); err == nil {
		t.Error("超过大小上限的文件应被拒绝")
	}
}

func TestParseMarkdownHandlesWindowsLineEndings(t *testing.T) {
	// 上传的文件可能带 CRLF，解析结果不应残留 \r。
	got, err := ParseMarkdown("# 标题\r\n\r\n正文第一段。\r\n")
	if err != nil {
		t.Fatalf("解析 CRLF 文件失败: %v", err)
	}
	if got.Title != "标题" {
		t.Errorf("标题 = %q，期望 标题", got.Title)
	}
	if strings.ContainsRune(got.Body, '\r') {
		t.Errorf("正文不应包含回车符：%q", got.Body)
	}
}

func TestParseMarkdownIgnoresUnclosedFrontMatter(t *testing.T) {
	// 未闭合的 Front Matter 不应被当作区块吞掉，整份内容按 Markdown 处理。
	got, err := ParseMarkdown("---\ntitle: 未闭合\n\n# 标题\n\n正文。\n")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.Title != "标题" {
		t.Errorf("标题 = %q，期望从一级标题取得 标题", got.Title)
	}
	if !strings.HasPrefix(got.Body, "---") {
		t.Errorf("未闭合的 Front Matter 应留在正文中，实际 %q", got.Body)
	}
}
