package repo

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// 少量可空列在 None 语义上的转换辅助。0 与 "" 在本项目模型中不代表合法主键或文本，
// 因此统一作为“无值”处理。

func Int8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func Int8OrNull(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func Text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }

func TextOrNull(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func Timestamptz(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v, Valid: true}
}
