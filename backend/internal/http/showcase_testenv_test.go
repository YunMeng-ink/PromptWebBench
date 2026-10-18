package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeImageBed 是图床接口的本地替身：按 multipart 字段 image 接收文件，
// 返回固定形态的成功响应。上传用例通过它验证真实的服务端上传代码路径。
type fakeImageBed struct {
	server *httptest.Server

	mu       sync.Mutex
	failNext bool
	hits     int
	lastSize int64
}

func newFakeImageBed(t *testing.T) *fakeImageBed {
	t.Helper()
	fake := &fakeImageBed{}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeImageBed) URL() string { return f.server.URL }

func (f *fakeImageBed) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	if f.failNext {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"图床内部错误","message":"图床内部错误"}`))
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"error":"缺少图片","message":"缺少图片"}`))
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"error":"缺少图片","message":"缺少图片"}`))
		return
	}
	defer func() { _ = file.Close() }()
	f.lastSize = header.Size

	name := fmt.Sprintf("stored-%d.webp", f.hits)
	payload := map[string]any{
		"success": true,
		"url":     f.server.URL + "/i/" + name,
		"data": map[string]any{
			"url":      f.server.URL + "/i/" + name,
			"filename": name,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// setFailure 让下一次上传返回失败响应。
func (f *fakeImageBed) setFailure(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = fail
}

// requestCount 返回图床收到的请求数。
func (f *fakeImageBed) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}
