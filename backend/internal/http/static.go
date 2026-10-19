package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// staticAssets 为静态文件提供 ETag。
//
// 内嵌文件系统（embed.FS）的修改时刻恒为零值，ServeContent 不会生成
// Last-Modified，浏览器与 CDN 每次校验都只能拿到完整响应。内嵌模式在启动时
// 按内容计算摘要；磁盘模式（STATIC_DIR）的文件会随构建变化，改用修改时刻与
// 大小生成摘要，避免重新构建后仍返回旧的 304。
type staticAssets struct {
	root  fs.FS
	fixed map[string]string
}

// newStaticAssets 建立静态产物索引，并确认入口文件存在。
func newStaticAssets(root fs.FS) (*staticAssets, error) {
	info, err := fs.Stat(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("静态产物缺少 index.html: %w", err)
	}
	out := &staticAssets{root: root}
	if !info.ModTime().IsZero() {
		return out, nil
	}

	out.fixed = make(map[string]string)
	err = fs.WalkDir(root, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		out.fixed[p] = `"` + hex.EncodeToString(sum[:]) + `"`
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历静态产物: %w", err)
	}
	return out, nil
}

// etag 返回文件的摘要；路径不存在或为目录时返回 false。
func (a *staticAssets) etag(name string) (string, bool) {
	if a.fixed != nil {
		value, ok := a.fixed[name]
		return value, ok
	}
	info, err := fs.Stat(a.root, name)
	if err != nil || info.IsDir() {
		return "", false
	}
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()), true
}

// webCacheControl 为页面入口与静态资源区分缓存策略。
//
// 带内容哈希的构建产物可以永久缓存；页面入口与 SPA 回落路径每次使用前都要
// 向源站校验，校验命中时由 ETag 返回 304。非哈希的根目录文件（如 robots.txt）
// 给一个较短的缓存期，避免部署后长时间读到旧版本。
func webCacheControl(p string) string {
	if strings.HasPrefix(p, "/assets/") {
		return "public, max-age=31536000, immutable"
	}
	if p == "/" || p == "/index.html" || !strings.Contains(lastSegment(p), ".") {
		return "no-cache"
	}
	return "public, max-age=3600"
}

// apiCacheControl 为接口响应给出缓存策略。
//
// 只有响应与访问者身份无关的只读端点进入共享缓存；其余接口一律 no-store，
// 防止 CDN 把某个用户的私有提示词、评分或通知缓存后发给其他访问者。
func apiCacheControl(method, p string) string {
	if !isSafeMethod(method) || !isCacheableAPIPath(p) {
		return "no-store"
	}
	switch p {
	case APIPrefix + "/leaderboard", APIPrefix + "/showcase/groups", APIPrefix + "/tags":
		return "public, max-age=60, stale-while-revalidate=300"
	}
	if strings.HasPrefix(p, APIPrefix+"/showcase/models/") {
		return "public, max-age=60, stale-while-revalidate=300"
	}
	return "public, max-age=300, stale-while-revalidate=3600"
}

// isCacheableAPIPath 判定接口响应是否与访问者无关。
//
// 字典里的考察目的（/purposes）会按身份隐藏高风险条目，提示词列表与详情会按
// 可见性过滤，公告带个人已读状态，因此这些只读端点同样不进入共享缓存。
func isCacheableAPIPath(p string) bool {
	switch p {
	case APIPrefix + "/capabilities",
		APIPrefix + "/difficulties",
		APIPrefix + "/tags",
		APIPrefix + "/models",
		APIPrefix + "/leaderboard",
		APIPrefix + "/showcase/groups":
		return true
	}
	return strings.HasPrefix(p, APIPrefix+"/showcase/models/") &&
		strings.HasSuffix(p, "/entries")
}

// isSafeMethod 对应 RFC 7231 定义的只读方法。
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// EchoAPICache 为接口响应写入缓存策略。静态资源由 webHandler 单独设置。
func EchoAPICache() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			p := c.Request().URL.Path
			if p == APIPrefix || strings.HasPrefix(p, APIPrefix+"/") {
				c.Response().Header().Set(
					echo.HeaderCacheControl,
					apiCacheControl(c.Request().Method, p),
				)
			}
			return next(c)
		}
	}
}

// skipCSRFForSharedCache 让静态资源与可共享缓存的只读接口不签发 CSRF Cookie。
//
// 中间件会在响应上写 Set-Cookie 与 Vary: Cookie，两者都会让 CDN 无法复用缓存，
// 而 Set-Cookie 被缓存后还会把同一个令牌发给不同访问者。浏览器首次访问应用时
// 的登录态探测（/auth/session）仍会签发 Cookie，写请求不受影响。
func skipCSRFForSharedCache(c *echo.Context) bool {
	if !isSafeMethod(c.Request().Method) {
		return false
	}
	p := c.Request().URL.Path
	if !strings.HasPrefix(p, APIPrefix) {
		return true
	}
	return isCacheableAPIPath(p)
}

func lastSegment(p string) string {
	if index := strings.LastIndexByte(p, '/'); index >= 0 {
		return p[index+1:]
	}
	return p
}
