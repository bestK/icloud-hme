// Package pool 提供 HME 别名"暖池",按合规节奏预建、命中直接 Pop。
//
// 每个 iCloud 账号一个共享池,所有 user token 从同一池取用;归属绑定
// 在 pop 时写入 token store。
package pool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry 是池里的一条预建别名。
type Entry struct {
	Email       string    `json:"email"`
	AnonymousID string    `json:"anonymous_id"`
	Label       string    `json:"label"`
	CreatedAt   time.Time `json:"created_at"`
}

// hourCounter 记录当前整点小时内已创建的次数。
type hourCounter struct {
	Hour  time.Time `json:"hour"`
	Count int       `json:"count"`
}

// Store 是线程安全的池存储。
type Store struct {
	mu       sync.Mutex
	pools    map[string][]Entry
	counters map[string]hourCounter
	dataFile string
}

// NewStore 打开或创建 pool.json。
func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	s := &Store{
		pools:    make(map[string][]Entry),
		counters: make(map[string]hourCounter),
		dataFile: filepath.Join(dataDir, "pool.json"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var wrapper struct {
		Pools    map[string][]Entry     `json:"pools"`
		Counters map[string]hourCounter `json:"hour_counters"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return err
	}
	if wrapper.Pools != nil {
		s.pools = wrapper.Pools
	}
	if wrapper.Counters != nil {
		s.counters = wrapper.Counters
	}
	return nil
}

func (s *Store) save() error {
	wrapper := struct {
		Pools     map[string][]Entry     `json:"pools"`
		Counters  map[string]hourCounter `json:"hour_counters"`
		UpdatedAt string                 `json:"updated_at"`
	}{
		Pools:     s.pools,
		Counters:  s.counters,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.dataFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.dataFile)
}

// Push 追加一条预建条目。
func (s *Store) Push(accountID string, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pools[accountID] = append(s.pools[accountID], e)
	return s.save()
}

// Pop 弹出最早的一条(FIFO)。返回 ok=false 表示池空。
func (s *Store) Pop(accountID string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.pools[accountID]
	if len(list) == 0 {
		return Entry{}, false
	}
	head := list[0]
	s.pools[accountID] = list[1:]
	_ = s.save()
	return head, true
}

// Depth 返回某账号的池深度。
func (s *Store) Depth(accountID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pools[accountID])
}

// AllDepths 返回所有账号的池深度快照。
func (s *Store) AllDepths() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.pools))
	for k, v := range s.pools {
		out[k] = len(v)
	}
	return out
}

// currentHour 返回当前整点。配额按整点小时分桶,跨桶即清零。
func currentHour() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
}

// counterFor 取某账号当前小时的计数器。留在上一个小时的旧计数一律当 0,
// 这样跨小时清零只需在读的时候判断,不用起后台任务去扫。
// 调用方必须已持有 s.mu。
func (s *Store) counterFor(accountID string) hourCounter {
	hourKey := currentHour()
	cur := s.counters[accountID]
	if !cur.Hour.Equal(hourKey) {
		return hourCounter{Hour: hourKey, Count: 0}
	}
	return cur
}

// TryConsumeQuota 检查当前小时是否还有配额,有则计数 +1 返回 true。
// hourlyMax<=0 表示禁用配额限制(总是返回 true)。
func (s *Store) TryConsumeQuota(accountID string, hourlyMax int) bool {
	if hourlyMax <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.counterFor(accountID)
	if cur.Count >= hourlyMax {
		s.counters[accountID] = cur
		return false
	}
	cur.Count++
	s.counters[accountID] = cur
	_ = s.save()
	return true
}

// RecordUsage 无条件记一次上游创建,不做配额检查也不会失败。
//
// 给池空时的实时创建用:那条路径不能因为配额耗尽就把用户挡回去,但它确实
// 在上游建了一个别名。不记进同一个账本的话,补池会以为这一小时很闲、继续
// 按满速补,两条路径叠加出来的真实创建频率就失控了 —— 而这恰恰发生在
// 需求高峰,风控最容易盯上的时候。
func (s *Store) RecordUsage(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.counterFor(accountID)
	cur.Count++
	s.counters[accountID] = cur
	_ = s.save()
}

// ReleaseQuota 回滚一次 TryConsumeQuota(例如后续 create 失败但非限流,不消耗配额)。
// 跨小时后旧计数已作废,此时回滚是空操作。
func (s *Store) ReleaseQuota(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.counterFor(accountID)
	if cur.Count > 0 {
		cur.Count--
		s.counters[accountID] = cur
		_ = s.save()
	}
}

// HourUsage 返回某账号当前小时已用的配额数(仅本小时内有效,跨小时会清零)。
func (s *Store) HourUsage(accountID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counterFor(accountID).Count
}
