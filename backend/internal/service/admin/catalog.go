package admin

import (
	"context"
	"time"

	"github.com/promptbench/backend/internal/modelsdev"
)

// catalogTTL 是目录快照的复用时长。预览的分页、检索与筛选都会重复读取目录，
// 每次都回源会把一次导入变成数十次上游请求。
const catalogTTL = 5 * time.Minute

// catalogSnapshot 是一次成功拉取的 models.dev 目录。
type catalogSnapshot struct {
	entries   []modelsdev.Entry
	fetchedAt time.Time
}

// catalog 返回可用的目录快照。未过期时直接复用；refresh 为真时强制回源。
// 拉取失败不写入缓存，下一次调用会重新尝试。
func (s *Service) catalog(ctx context.Context, refresh bool) (catalogSnapshot, error) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	now := s.now()
	if !refresh && len(s.catalogEntries) > 0 && now.Sub(s.catalogFetchedAt) < catalogTTL {
		return catalogSnapshot{entries: s.catalogEntries, fetchedAt: s.catalogFetchedAt}, nil
	}

	entries, err := s.fetchCatalog(ctx)
	if err != nil {
		return catalogSnapshot{}, err
	}
	s.catalogEntries = entries
	s.catalogFetchedAt = now
	return catalogSnapshot{entries: entries, fetchedAt: now}, nil
}
