package pool

import (
	"context"
	"log"
	"sync"
	"time"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
)

// Filler 是后台补池 goroutine。
type Filler struct {
	mgr       *account.Manager
	store     *Store
	interval  time.Duration
	target    int
	hourlyMax int

	// 运行快照。补池是个看不见的后台动作,不把这些记下来,面板上就只能看到
	// 池深度在变,回答不了"到底有没有在定时跑"。
	mu         sync.Mutex
	running    bool
	lastRun    time.Time
	nextRun    time.Time
	lastAdded  int
	totalAdded int
	lastErr    string
	lastErrAt  time.Time
}

// Status 是补池调度器的运行快照。
type Status struct {
	Enabled bool `json:"enabled"`
	// Running 为 false 且 Enabled 为 true,说明进程刚起还没进循环
	Running         bool `json:"running"`
	Target          int  `json:"target"`
	HourlyMax       int  `json:"hourly_max"`
	IntervalSeconds int  `json:"interval_seconds"`
	// LastRunAt / NextRunAt 为空表示还没跑过第一轮
	LastRunAt  string `json:"last_run_at,omitempty"`
	NextRunAt  string `json:"next_run_at,omitempty"`
	LastAdded  int    `json:"last_added"`
	TotalAdded int    `json:"total_added"`
	// LastError 是最近一次补池失败的原因,带上时间才能判断它是不是旧账
	LastError   string `json:"last_error,omitempty"`
	LastErrorAt string `json:"last_error_at,omitempty"`
}

// NewFiller 构造 Filler。target<=0 或 hourlyMax<=0 表示禁用池,Start 直接返回。
func NewFiller(mgr *account.Manager, store *Store, target, hourlyMax int, interval time.Duration) *Filler {
	return &Filler{
		mgr:       mgr,
		store:     store,
		interval:  interval,
		target:    target,
		hourlyMax: hourlyMax,
	}
}

// Enabled 返回 Filler 是否启用。
func (f *Filler) Enabled() bool {
	return f.target > 0 && f.hourlyMax > 0 && f.interval > 0
}

// Target 返回目标深度。
func (f *Filler) Target() int { return f.target }

// HourlyMax 返回每小时最多创建数。
func (f *Filler) HourlyMax() int { return f.hourlyMax }

// Interval 返回补池轮询间隔。
func (f *Filler) Interval() time.Duration { return f.interval }

// Status 返回运行快照。
func (f *Filler) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := Status{
		Enabled:         f.Enabled(),
		Running:         f.running,
		Target:          f.target,
		HourlyMax:       f.hourlyMax,
		IntervalSeconds: int(f.interval.Seconds()),
		LastAdded:       f.lastAdded,
		TotalAdded:      f.totalAdded,
		LastError:       f.lastErr,
	}
	if !f.lastRun.IsZero() {
		st.LastRunAt = f.lastRun.Format(time.RFC3339)
	}
	if !f.nextRun.IsZero() {
		st.NextRunAt = f.nextRun.Format(time.RFC3339)
	}
	if !f.lastErrAt.IsZero() {
		st.LastErrorAt = f.lastErrAt.Format(time.RFC3339)
	}
	return st
}

// noteErr 记下最近一次补池失败,供面板显示。
func (f *Filler) noteErr(accountID string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastErr = accountID + ": " + err.Error()
	f.lastErrAt = time.Now()
}

// Start 启动后台补池 goroutine,ctx 取消时优雅退出。
func (f *Filler) Start(ctx context.Context) {
	if !f.Enabled() {
		log.Printf("pool filler 未启用 (target=%d hourly_max=%d interval=%s)", f.target, f.hourlyMax, f.interval)
		return
	}
	log.Printf("pool filler 启动 target=%d interval=%s hourly_max=%d", f.target, f.interval, f.hourlyMax)
	f.mu.Lock()
	f.running = true
	f.nextRun = time.Now()
	f.mu.Unlock()
	go func() {
		// 启动后立即触发一次
		f.tick()
		t := time.NewTicker(f.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("pool filler 退出")
				f.mu.Lock()
				f.running = false
				f.mu.Unlock()
				return
			case <-t.C:
				f.tick()
			}
		}
	}()
}

// tick 遍历所有账号,尝试补池。
func (f *Filler) tick() {
	added := 0
	accounts := f.mgr.ListAccounts()
	for _, acc := range accounts {
		if acc.Status != "active" {
			continue
		}
		if f.refillOne(acc.ID) {
			added++
		}
	}
	now := time.Now()
	f.mu.Lock()
	f.lastRun = now
	f.nextRun = now.Add(f.interval)
	f.lastAdded = added
	f.totalAdded += added
	f.mu.Unlock()
}

// refillOne 对单个账号做一次补池。返回是否实际补了 1 个。
func (f *Filler) refillOne(accountID string) bool {
	depth := f.store.Depth(accountID)
	if depth >= f.target {
		return false
	}
	if !f.store.TryConsumeQuota(accountID, f.hourlyMax) {
		log.Printf("pool filler skip %s: 本小时配额已用完", accountID)
		return false
	}
	client, err := f.mgr.HMEClient(accountID, false)
	if err != nil {
		log.Printf("pool filler %s HMEClient 失败: %v", accountID, err)
		f.noteErr(accountID, err)
		f.store.ReleaseQuota(accountID)
		return false
	}
	// 池预建 label 固定为 "pool",用户 create 时会记归属库的实际 label
	result, err := client.CreateAlias("pool", 1)
	_ = f.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		f.noteErr(accountID, err)
		if hme.IsRateLimit(err.Error()) {
			log.Printf("pool filler %s 触发限流,配额保留: %v", accountID, err)
			// 触发限流的这次也算掉一次尝试(不 release),等下一小时
		} else {
			log.Printf("pool filler %s create 失败: %v", accountID, err)
			f.store.ReleaseQuota(accountID)
		}
		return false
	}
	entry := Entry{
		Email:       result.Email,
		AnonymousID: result.AnonymousID,
		Label:       result.Label,
		CreatedAt:   time.Now(),
	}
	if err := f.store.Push(accountID, entry); err != nil {
		log.Printf("pool filler %s push 失败: %v", accountID, err)
		return false
	}
	// 别名此刻已在 iCloud 侧真实存在,计入账号统计。
	// 后续用户 Pop 这条时不再计数,否则会重复。
	f.mgr.ApplyAliasDelta(accountID, account.AliasCreated)
	log.Printf("pool filler %s +1 → depth=%d", accountID, f.store.Depth(accountID))
	return true
}
