//go:build embedweb

// Package web 内嵌前端构建产物，使打包部署只需一个后端二进制。
//
// embedweb 构建标签只在 deploy/pb.sh build 的打包编译中开启：内嵌产物体积不小，
// 常规构建（go run、go test、make gate）不需要携带。未开启标签时本包退化为空实现
// （web_none.go），接口层据此跳过静态路由注册。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// HasIndex 报告内嵌产物中是否存在前端入口 index.html。
func HasIndex() bool {
	_, err := fs.Stat(files, "dist/index.html")
	return err == nil
}

// FS 返回以构建产物根为根的文件系统。
func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		// dist 目录随 go:embed 声明必然存在，此处不可达。
		panic(err)
	}
	return sub
}
