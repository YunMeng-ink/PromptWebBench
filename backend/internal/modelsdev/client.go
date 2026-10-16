// Package modelsdev 读取 models.dev 的模型目录（需求 §13.3）。
//
// 取值路径由 C-45 补充裁定：根对象以提供商 ID 为键，提供商条目含 id、name 与 models；
// 模型条目位于 models 映射内。导入只取其中七个字段，其余字段随原始条目一并
// 存入 model_snapshots 供字段追溯。
package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DefaultURL 是 models.dev 的模型目录地址。
const DefaultURL = "https://models.dev/api.json"

// maxResponseBytes 是响应的读取上限，防止异常上游把进程内存吃满。
const maxResponseBytes = 16 << 20

// Entry 是一条待导入的模型条目，字段已按 C-45 的映射路径压平。
type Entry struct {
	// ProviderID 是根对象的键，即提供商标识。
	ProviderID string
	// ProviderName 是 provider.name，即模型厂商名称。
	ProviderName string
	// ModelID 是 models[id].id，即模型标识。
	ModelID string
	// Name 是 models[id].name。
	Name string
	// Description 是 models[id].description。
	Description string
	// ReleaseDate 是 models[id].release_date，形如 2025-01-31；缺失时为空串。
	ReleaseDate string
	// InputModalities 是 models[id].modalities.input。条目缺少 modalities 时为空切片。
	InputModalities []string
	// ContextLength 是 models[id].limit.context；缺失时为 nil。
	ContextLength *int64
	// Raw 是该模型条目的原始 JSON，原样存入 model_snapshots。
	Raw json.RawMessage
}

// Client 读取模型目录。URL 可替换，测试用本地 HTTP 服务驱动真实解析代码。
type Client struct {
	URL        string
	HTTPClient *http.Client
}

// NewClient 构造读取器。url 为空时使用 models.dev 的正式地址。
func NewClient(url string) *Client {
	if url == "" {
		url = DefaultURL
	}
	return &Client{URL: url, HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

// providerPayload 是提供商条目的解析形态。
type providerPayload struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	Models map[string]json.RawMessage `json:"models"`
}

// modelPayload 是模型条目的解析形态。modalities 与 limit 用指针区分「字段缺失」
// 与「字段存在但为空」，前者导入为空值（需求 §13.3）。
type modelPayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
	Limit       *struct {
		Context *int64 `json:"context"`
	} `json:"limit"`
	Modalities *struct {
		Input []string `json:"input"`
	} `json:"modalities"`
}

// Fetch 读取并解析模型目录。任何网络或解析失败都返回错误，由服务层翻译为 bad_gateway。
func (c *Client) Fetch(ctx context.Context) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 models.dev 请求: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 models.dev: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev 返回状态 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("读取 models.dev 响应: %w", err)
	}

	entries, err := Parse(body)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("models.dev 响应中没有任何模型条目")
	}
	return entries, nil
}

// Parse 解析模型目录响应。提供商与模型按键排序，使同一份响应每次导入的顺序一致。
func Parse(body []byte) ([]Entry, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析 models.dev 响应: %w", err)
	}

	providerIDs := make([]string, 0, len(root))
	for id := range root {
		providerIDs = append(providerIDs, id)
	}
	sort.Strings(providerIDs)

	entries := make([]Entry, 0, len(root))
	for _, providerID := range providerIDs {
		var provider providerPayload
		if err := json.Unmarshal(root[providerID], &provider); err != nil {
			return nil, fmt.Errorf("解析提供商 %s: %w", providerID, err)
		}

		modelIDs := make([]string, 0, len(provider.Models))
		for id := range provider.Models {
			modelIDs = append(modelIDs, id)
		}
		sort.Strings(modelIDs)

		for _, modelID := range modelIDs {
			raw := provider.Models[modelID]
			entry, err := parseModelEntry(provider, providerID, modelID, raw)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// parseModelEntry 把一条模型原始条目压平为 Entry。
//
// 模型标识与名称以响应中的取值优先，字段缺失时退回映射键，
// 使缺少 id 或 name 的条目仍能被唯一标识。
func parseModelEntry(provider providerPayload, providerID, modelID string, raw json.RawMessage) (Entry, error) {
	var payload modelPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Entry{}, fmt.Errorf("解析模型 %s/%s: %w", providerID, modelID, err)
	}

	entry := Entry{
		ProviderID:   providerID,
		ProviderName: strings.TrimSpace(provider.Name),
		ModelID:      firstNonEmpty(payload.ID, modelID),
		Name:         firstNonEmpty(payload.Name, modelID),
		Description:  payload.Description,
		ReleaseDate:  strings.TrimSpace(payload.ReleaseDate),
		Raw:          raw,
	}
	if payload.Modalities != nil {
		entry.InputModalities = payload.Modalities.Input
	}
	if entry.InputModalities == nil {
		entry.InputModalities = []string{}
	}
	if payload.Limit != nil {
		entry.ContextLength = payload.Limit.Context
	}
	return entry, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
