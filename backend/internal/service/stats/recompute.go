package stats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/repo"
)

// Recompute 重算一条提示词的统计快照并生成推荐标签。
//
// 全部写入处于单个事务内（开发方案 §6.4 验收标准「统计快照写入在单事务内，无半写状态」）：
// 快照两行、逐模型快照、标签流转与审计日志要么全部生效，要么全部不生效。
//
// 事务开头取该提示词的重算锁（pg_advisory_xact_lock）。评分提交路径在写评分前
// 同样先取这把锁，因此重算读到的评分集合不会在计算过程中被并发写入改变。
func (s *Service) Recompute(ctx context.Context, promptID int64) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if promptID <= 0 {
		return fmt.Errorf("重算统计的提示词 ID 必须为正数，得到 %d", promptID)
	}

	// 事务提交之后再作废缓存：中途作废会让另一个读取在事务提交前把旧快照写回缓存，
	// 重算的结果被覆盖。提交后作废则保证下一次读取一定重新落库取值。
	defer s.cache.invalidate(promptID)

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := q.LockPromptStatsRecompute(ctx, promptID); err != nil {
			return fmt.Errorf("获取提示词 %d 的统计重算锁: %w", promptID, err)
		}

		versionID, err := currentVersionID(ctx, q, promptID)
		if err != nil {
			return err
		}

		result, err := s.compute(ctx, q, promptID, versionID)
		if err != nil {
			return err
		}

		if _, err := q.UpsertPromptStats(ctx, result.snapshotRow(promptID)); err != nil {
			return fmt.Errorf("写入提示词 %d 的统计快照: %w", promptID, err)
		}

		historyByID := modelByID(result.history.Models)
		recentByID := modelByID(result.recent.Models)
		kept := make([]int64, 0, len(result.history.Models))
		for _, item := range result.history.Models {
			kept = append(kept, item.ModelID)
			recent, ok := recentByID[item.ModelID]
			if !ok {
				// 历史与近 N 日的聚合行来自同一份模型清单，缺失说明查询结果不一致。
				return fmt.Errorf("提示词 %d 的近 N 日窗口缺少模型 %d 的聚合行", promptID, item.ModelID)
			}
			history := historyByID[item.ModelID]
			if _, err := q.UpsertPromptModelStat(ctx, result.modelRow(promptID, item.ModelID, history, recent)); err != nil {
				return fmt.Errorf("写入提示词 %d 模型 %d 的统计快照: %w", promptID, item.ModelID, err)
			}
		}
		// 清理已停用模型的残留快照，否则列表页的完成度均值会把已下线模型算进去。
		if len(kept) > 0 {
			if err := q.DeletePromptModelStatsExcept(ctx, repo.DeletePromptModelStatsExceptParams{
				PromptID: promptID,
				ModelIds: kept,
			}); err != nil {
				return fmt.Errorf("清理提示词 %d 的失效模型统计: %w", promptID, err)
			}
		}

		if err := s.syncRecommendTags(ctx, q, promptID, result); err != nil {
			return err
		}
		return nil
	})
}

// currentVersionID 读取提示词的当前版本。统计快照记录版本号，使快照与它反映的
// 评分所属版本可对照（需求 §7.1）。
func currentVersionID(ctx context.Context, q *repo.Queries, promptID int64) (int64, error) {
	row, err := q.GetPromptStatsVersion(ctx, promptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("提示词 %d 不存在或已删除", promptID)
		}
		return 0, fmt.Errorf("查询提示词 %d 的当前版本: %w", promptID, err)
	}
	if !row.Valid || row.Int64 == 0 {
		return 0, fmt.Errorf("提示词 %d 缺少当前版本，统计快照无法记录版本", promptID)
	}
	return row.Int64, nil
}

// LockRecompute 在当前事务上取该提示词的统计重算锁。
//
// 评分提交路径在写评分前调用它，Recompute 在事务开头调用同一个函数，两侧互斥：
// 重算读到的评分集合不会在计算过程中被并发写入改变，快照与评分因此始终对应。
func (s *Service) LockRecompute(ctx context.Context, q *repo.Queries, promptID int64) error {
	if err := q.LockPromptStatsRecompute(ctx, promptID); err != nil {
		return fmt.Errorf("获取提示词 %d 的统计重算锁: %w", promptID, err)
	}
	return nil
}

// MarkStale 使某提示词的进程内缓存失效。评分写入、提示词修改、重大修改之后调用，
// 使下一次读取重新计算而不是返回旧快照。
func (s *Service) MarkStale(promptID int64) {
	s.cache.invalidate(promptID)
}

// Fresh 返回缓存条目；不存在或已作废时返回 false。
// 存活时长按当前配置取值，配置改动无需重启即可生效。
func (s *Service) Fresh(ctx context.Context, promptID int64) (*Snapshot, bool) {
	return s.cache.get(promptID, s.cacheTTL(ctx))
}

// put 写入缓存条目。
func (s *Service) put(snapshot Snapshot) {
	s.cache.put(snapshot)
}

// cacheDefaultTTL 是统计缓存存活时长的缺省取值（技术可行性报告 §3.1：60 秒），
// 实际取值来自配置项 stats_cache_ttl_seconds。
//
// 条目在评分写入时被主动作废，存活时长只用于兜底进程重启后遗留的条目。
const cacheDefaultTTL = 60 * time.Second

// cacheTTL 读取缓存存活时长配置。缺项或取值非法时按缺省值运行：
// 缓存时长是性能参数，配置表被写坏不应让统计读取整体不可用。
func (s *Service) cacheTTL(ctx context.Context) time.Duration {
	seconds := s.intOf(ctx, "stats_cache_ttl_seconds", int64(cacheDefaultTTL/time.Second))
	if seconds <= 0 {
		return cacheDefaultTTL
	}
	return time.Duration(seconds) * time.Second
}
