// schemagen 把内嵌迁移中的建表脚本按文件名顺序拼接为 db/schema.sql，供 sqlc 解析。
// 约定：文件名以 _seed.sql 结尾的迁移只含数据，不进入 schema.sql。
package main

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/promptbench/backend/migrations"
)

const header = `-- 本文件由 cmd/schemagen 从 backend/migrations/*.sql 生成，禁止手工编辑。
-- 重新生成：在 backend 目录执行 make schema

`

func main() {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		panic(fmt.Errorf("读取内嵌迁移目录: %w", err))
	}

	var ddl []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, "_seed.sql") {
			continue
		}
		ddl = append(ddl, name)
	}
	sort.Strings(ddl)

	var sb strings.Builder
	sb.WriteString(header)
	for _, name := range ddl {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			panic(fmt.Errorf("读取迁移 %s: %w", name, err))
		}
		sb.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile("db/schema.sql", []byte(sb.String()), 0o644); err != nil {
		panic(fmt.Errorf("写入 db/schema.sql: %w", err))
	}
	fmt.Printf("已生成 db/schema.sql，来源 %d 个迁移文件: %s\n", len(ddl), strings.Join(ddl, ", "))
}
