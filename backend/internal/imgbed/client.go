// Package imgbed 把用户提交的模型结果图片上传到外部图床（img.scdn.io）。
//
// 平台数据库只保存图床返回的外链与文件名，不保存图片本体；
// 因此上传成功与入库写入是两个阶段：先外呼图床，再把外链写入 rating_images。
package imgbed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// DefaultEndpoint 是图床上传接口地址（https://img.scdn.io/api_docs.php）。
const DefaultEndpoint = "https://img.scdn.io/api/v1.php"

// maxResponseBytes 是图床响应的读取上限，防止异常上游把进程内存吃满。
const maxResponseBytes = 1 << 20

// Result 是一次成功上传的外链结果。
type Result struct {
	// URL 是可直接用于 <img src> 的图片外链。
	URL string
	// Filename 是图床生成的文件名，入库用于追溯。
	Filename string
}

// Client 上传图片。Endpoint 与 HTTPClient 可替换，测试用本地 HTTP 服务驱动真实代码。
type Client struct {
	// Endpoint 是图床上传地址，空串表示 DefaultEndpoint。
	Endpoint   string
	HTTPClient *http.Client
}

// NewClient 构造图床客户端。endpoint 为空时使用正式地址。
func NewClient(endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{Endpoint: endpoint, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

// uploadResponse 是图床成功响应中会被读取的部分。
type uploadResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Data    struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	} `json:"data"`
	// 失败响应的 error 与 message 内容一致，任取其一即可。
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Upload 把一张图片上传到图床，返回外链与文件名。
// 文件内容经 multipart 字段 image 提交，符合图床接口的请求契约。
func (c *Client) Upload(ctx context.Context, filename, contentType string, content io.Reader) (Result, error) {
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="image"; filename="%s"`, escapeQuotes(filename)))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return Result{}, fmt.Errorf("构造图床上传表单: %w", err)
	}
	if _, err := io.Copy(part, content); err != nil {
		return Result{}, fmt.Errorf("写入图床上传表单: %w", err)
	}
	if err := writer.Close(); err != nil {
		return Result{}, fmt.Errorf("收尾图床上传表单: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, strings.NewReader(body.String()))
	if err != nil {
		return Result{}, fmt.Errorf("构造图床上传请求: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("请求图床: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Result{}, fmt.Errorf("读取图床响应: %w", err)
	}

	var parsed uploadResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Result{}, fmt.Errorf("解析图床响应（状态 %d）: %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		reason := parsed.Message
		if reason == "" {
			reason = parsed.Error
		}
		if reason == "" {
			reason = fmt.Sprintf("图床返回状态 %d", resp.StatusCode)
		}
		return Result{}, fmt.Errorf("图床上传失败: %s", reason)
	}

	result := Result{URL: parsed.URL, Filename: parsed.Data.Filename}
	if result.URL == "" {
		result.URL = parsed.Data.URL
	}
	if result.URL == "" {
		return Result{}, fmt.Errorf("图床响应缺少图片外链（状态 %d）", resp.StatusCode)
	}
	return result, nil
}

// escapeQuotes 转义 multipart 头中的文件名引号与反斜杠。
func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value)
}
