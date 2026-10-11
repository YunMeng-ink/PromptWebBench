package prompt

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/promptbench/backend/internal/apperr"
)

// Imported 是 Markdown 文件导入的解析结果（需求 §8.7）。
// Front Matter 为可选支持：存在时以其中的标题与描述补全，其余字段由用户导入后补全。
type Imported struct {
	Title       string
	Body        string
	Description string
}

// ImportLimit 是导入文件的大小上限（字节）。与请求体 1 MB 上限同源，
// 前端在选择文件时即可据此拒绝，避免把超限文件传到服务端。
const ImportLimit = 512 * 1024

// ParseMarkdown 解析 .md 文件：取首个一级标题为标题、首个一级标题之前的内容为描述，
// 整体内容为正文。
//
// Front Matter 以文件开头的 --- 区块为准，解析 YAML 的 title、description、tags 三项；
// 区块之后的内容按普通 Markdown 处理，YAML 本身不进入正文。
func ParseMarkdown(content string) (Imported, error) {
	if len(content) > ImportLimit {
		return Imported{}, apperr.New(apperr.CodeValidationFailed, "文件超过大小上限")
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	frontMatter, body := splitFrontMatter(normalized)

	out := Imported{
		Body:        strings.TrimSpace(body),
		Description: strings.TrimSpace(frontMatter.description),
	}
	if frontMatter.title != "" {
		out.Title = frontMatter.title
	} else {
		out.Title = firstHeading(body)
	}
	if out.Body == "" {
		return Imported{}, apperr.New(apperr.CodeValidationFailed, "文件内容为空")
	}
	if out.Title == "" {
		return Imported{}, apperr.New(apperr.CodeValidationFailed, "未能识别标题，请手工填写")
	}
	return out, nil
}

type frontMatterData struct {
	title       string
	description string
}

// splitFrontMatter 切出文件开头的 Front Matter 区块。
//
// 只识别顶层键：Front Matter 只是可选的补充信息，不做完整 YAML 解析，
// 避免为导入功能引入一整套 YAML 语义。
func splitFrontMatter(content string) (frontMatterData, string) {
	if !strings.HasPrefix(content, "---\n") {
		return frontMatterData{}, content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return frontMatterData{}, content
	}
	block := content[4 : 4+end]
	rest := strings.TrimPrefix(content[4+end+len("\n---"):], "\n")

	var data frontMatterData
	for _, line := range strings.Split(block, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		switch strings.TrimSpace(key) {
		case "title":
			data.title = value
		case "description":
			data.description = value
		}
	}
	return data, rest
}

// firstHeading 用 Markdown 解析器取首个一级标题的文本。
// 标题内的行内标记（代码片段、强调等）把文字放在各自的子节点上，
// 因此按子树遍历收集 Text 段，取到的是原始文字而非 Markdown 源片段。
func firstHeading(content string) string {
	source := []byte(content)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))

	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Level != 1 {
			continue
		}
		if name := strings.TrimSpace(collectText(heading, source)); name != "" {
			return name
		}
	}
	return ""
}

// collectText 收集节点子树中全部 Text 段的原文。
func collectText(node ast.Node, source []byte) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch typed := child.(type) {
		case *ast.Text:
			out.Write(typed.Segment.Value(source))
		case *ast.String:
			out.Write(typed.Value)
		default:
			out.WriteString(collectText(child, source))
		}
	}
	return out.String()
}
