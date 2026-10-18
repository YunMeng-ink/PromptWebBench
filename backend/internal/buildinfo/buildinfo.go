// Package buildinfo 承载编译期注入的版本信息。
//
// Version 由构建命令通过 -ldflags 写入；未注入时取 dev，
// 使本地 go run 与测试不需要任何构建参数。
package buildinfo

// Version 是当前二进制的版本号，取发布标签（如 v0.2.0）或提交标识。
var Version = "dev"
