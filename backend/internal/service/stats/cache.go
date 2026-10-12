package stats

import (
	"sync"
	"time"
)

// cache 是统计快照的进程内缓存（开发方案 §6.4 交付内容「统计快照落库与进程内缓存」）。
//
// 单实例部署，无需一致性协议：条目在评分写入、提示词修改与重大修改时被主动作废，
// 存活时长只用于兜底进程重启后遗留的条目。
type cache struct {
	mu      sync.RWMutex
	entries map[int64]cacheEntry
}

type cacheEntry struct {
	snapshot Snapshot
	storedAt time.Time
}

func newCache() *cache {
	return &cache{entries: map[int64]cacheEntry{}}
}

// get 返回缓存条目。存在但已超过存活时长时视为未命中并顺手清除。
func (c *cache) get(promptID int64) (*Snapshot, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[promptID]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.storedAt) > snapshotTTL {
		c.invalidate(promptID)
		return nil, false
	}
	snapshot := entry.snapshot
	return &snapshot, true
}

// put 写入条目。快照按值复制：调用方持有的结构体后续修改不会影响缓存内容。
func (c *cache) put(snapshot Snapshot) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries[snapshot.PromptID] = cacheEntry{snapshot: snapshot, storedAt: time.Now()}
	c.mu.Unlock()
}

// invalidate 作废一条条目。
func (c *cache) invalidate(promptID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, promptID)
	c.mu.Unlock()
}

// size 返回条目数，供测试与后台自检使用。
func (c *cache) size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// flight 让同一提示词的统计重算在进程内只有一个在执行，其余调用等待它的结果。
//
// 快照过期时读路径需要同步重算。没有这层合并，一批并发读取会在缓存被写入之前
// 各自完整算一遍：500 次 Bootstrap 乘以并发数，白白放大负载。数据库侧的
// 咨询锁只保证写不冲突，不减少重复计算，两者解决的问题不同。
type flight struct {
	mu    sync.Mutex
	calls map[int64]*flightCall
}

type flightCall struct {
	done chan struct{}
	err  error
}

func newFlight() *flight {
	return &flight{calls: map[int64]*flightCall{}}
}

// do 执行 fn，同一 key 上并发的调用共享第一次执行的结果。
func (f *flight) do(key int64, fn func() error) error {
	f.mu.Lock()
	if existing, ok := f.calls[key]; ok {
		f.mu.Unlock()
		<-existing.done
		return existing.err
	}
	current := &flightCall{done: make(chan struct{})}
	f.calls[key] = current
	f.mu.Unlock()

	current.err = fn()
	close(current.done)

	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()
	return current.err
}
