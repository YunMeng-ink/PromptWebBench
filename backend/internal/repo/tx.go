package repo

import "github.com/jackc/pgx/v5"

// TxQuerier 在 sqlc 生成的 Querier 之上补回 WithTx。
// sqlc 已知限制（上游 issue #383）：WithTx 不在生成接口的 method set 中，
// 需要在事务内复用查询集时必须经由本接口取得。
type TxQuerier interface {
	Querier
	WithTx(tx pgx.Tx) *Queries
}
