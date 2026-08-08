package pool

import (
	"context"
	"log"
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

// Start 启动后台补池 goroutine,ctx 取消时优雅退出。
func (f *Filler) Start(ctx context.Context) {
	if !f.Enabled() {
		log.Printf("pool filler 未启用 (target=%d hourly_max=%d interval=%s)", f.target, f.hourlyMax, f.interval)
		return
	}
	log.Printf("pool filler 启动 target=%d interval=%s hourly_max=%d", f.target, f.interval, f.hourlyMax)
	go func() {
		// 启动后立即触发一次
		f.tick()
		t := time.NewTicker(f.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("pool filler 退出")
				return
			case <-t.C:
				f.tick()
			}
		}
	}()
}

// tick 遍历所有账号,尝试补池。
func (f *Filler) tick() {
	accounts := f.mgr.ListAccounts()
	for _, acc := range accounts {
		if acc.Status != "active" {
			continue
		}
		f.refillOne(acc.ID)
	}
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
		f.store.ReleaseQuota(accountID)
		return false
	}
	// 池预建 label 固定为 "pool",用户 create 时会记归属库的实际 label
	result, err := client.CreateAlias("pool", 1)
	_ = f.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
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
	log.Printf("pool filler %s +1 → depth=%d", accountID, f.store.Depth(accountID))
	return true
}
