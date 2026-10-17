//go:build !embedweb

package web

import (
	"io/fs"
	"testing/fstest"
)

// HasIndex 报告内嵌产物中是否存在前端入口 index.html。
// 未开启 embedweb 构建标签时恒为 false：接口层跳过静态路由注册。
func HasIndex() bool { return false }

// FS 返回空文件系统；HasIndex 为 false 时接口层不会调用。
func FS() fs.FS { return fstest.MapFS{} }
