package api_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// testConfigSecret 是测试用的主密钥，用于加密密钥类配置项。
const testConfigSecret = "test-config-secret-0123456789abcdef"

// modelsDevCatalog 是 models.dev 响应的固定样本，覆盖三种条目形态：
// 字段齐全的条目、缺少 modalities 的条目、缺少 limit 的条目（需求 §13.3）。
const modelsDevCatalog = `{
  "alpha": {
    "id": "alpha",
    "name": "Alpha Lab",
    "models": {
      "alpha-one": {
        "id": "alpha-one",
        "name": "Alpha One",
        "description": "第一个模型",
        "release_date": "2025-01-31",
        "limit": {"context": 128000},
        "modalities": {"input": ["text", "image"]}
      },
      "alpha-two": {
        "id": "alpha-two",
        "name": "Alpha Two",
        "limit": {"context": 32000}
      }
    }
  },
  "beta": {
    "id": "beta",
    "name": "Beta Lab",
    "models": {
      "beta-one": {
        "id": "beta-one",
        "name": "Beta One",
        "release_date": "2024-12-01",
        "modalities": {"input": ["text", "audio"]}
      }
    }
  }
}`

// fakeModelsDev 是 models.dev 的本地替身。
//
// 后台导入用例通过它切换上游的状态码与响应体，使导入走真实的 HTTP 客户端与解析代码，
// 而不是替换掉被测试的实现；同时可以断言上游被访问的次数。
type fakeModelsDev struct {
	server *httptest.Server

	mu     sync.Mutex
	status int
	body   string
	hits   int
}

func newFakeModelsDev(t *testing.T) *fakeModelsDev {
	t.Helper()
	fake := &fakeModelsDev{status: http.StatusOK, body: modelsDevCatalog}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeModelsDev) URL() string { return f.server.URL }

func (f *fakeModelsDev) serve(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	_, _ = w.Write([]byte(f.body))
}

// setResponse 切换上游响应。
func (f *fakeModelsDev) setResponse(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
	f.body = body
}

// requestCount 返回上游收到的请求数。
func (f *fakeModelsDev) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}
