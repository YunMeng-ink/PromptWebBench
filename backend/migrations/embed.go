// Package migrations 提供打进二进制的迁移脚本。
// 顶层 *.sql 为顺序正向迁移；down/ 为人工回滚脚本，引擎不执行。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
