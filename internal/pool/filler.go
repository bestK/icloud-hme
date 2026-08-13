package pool

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
)

// AliasHardCap 是 Apple 给单个 iCloud 账号的 Hide My Email 别名总数上限。
// 停用的别名同样占额度,所以要拿它跟 AliasTotal 比,不是 AliasActive。
const AliasHardCap = 750

// Filler 是后台补池 goroutine。
type Filler struct {
	mgr       *account.Manager
	store     *Store
	interval  time.Duration
	target    int
	hourlyMax int
	spacing   time.Duration
	cooldown  time.Duration

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
	Running bool `json:"running"`
	// Target 是最低保障水位,不是补池的终点 —— 见 NewFiller
	Target          int `json:"target"`
	HourlyMax       int `json:"hourly_max"`
	IntervalSeconds int `json:"interval_seconds"`
	// SpacingSeconds 是同一轮内两次创建之间的间隔
	SpacingSeconds int `json:"spacing_seconds"`
	// CooldownSeconds 是撞上限流后暂停的时长
	CooldownSeconds int `json:"cooldown_seconds"`
	// HardCap 是每个账号能囤到的天花板(Apple 侧限制)
	HardCap int `json:"hard_cap"`
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
//
// target 是「最低保障水位」,只用于面板上判断池子是不是快见底了,并不是补池的
// 终点:每小时配额是按合规节奏定死的,能不能扛住高峰取决于高峰前囤了多少。
// 空闲时段的配额不用就是白白过期,等需求真上来了再现建根本来不及,所以补池
// 会一直把配额吃满、囤到账号触及 AliasHardCap 为止。
//
// spacing 是同一轮内两次创建之间的间隔。一轮把剩余配额一次吃完意味着连着打
// 十几个创建请求,这个密集程度本身就是风控信号,所以要把它们摊开。
//
// cooldown 是撞上 iCloud 限流后这个账号停多久。hourlyMax 是我们自己猜的
// 安全节奏,限流是对方给的明确答复 —— 收到之后就该整个停下来等,而不是等
// 下一轮再去试探一次。
func NewFiller(mgr *account.Manager, store *Store, target, hourlyMax int, interval, spacing, cooldown time.Duration) *Filler {
	return &Filler{
		mgr:       mgr,
		store:     store,
		interval:  interval,
		target:    target,
		hourlyMax: hourlyMax,
		spacing:   spacing,
		cooldown:  cooldown,
	}
}

// Enabled 返回 Filler 是否启用。
func (f *Filler) Enabled() bool {
	return f.target > 0 && f.hourlyMax > 0 && f.interval > 0
}

// Target 返回最低保障水位。
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
		SpacingSeconds:  int(f.spacing.Seconds()),
		CooldownSeconds: int(f.cooldown.Seconds()),
		HardCap:         AliasHardCap,
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
	log.Printf("pool filler 启动 target=%d interval=%s hourly_max=%d spacing=%s cooldown=%s cap=%d",
		f.target, f.interval, f.hourlyMax, f.spacing, f.cooldown, AliasHardCap)
	f.mu.Lock()
	f.running = true
	f.nextRun = time.Now()
	f.mu.Unlock()
	go func() {
		// 启动后立即触发一次
		f.tick(ctx)
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
				f.tick(ctx)
			}
		}
	}()
}

// tick 遍历所有账号,把这一轮能补的都补上。
//
// 一轮可能跑得比 interval 还久(配额多、spacing 大时)。Ticker 的缓冲只有 1,
// 期间错过的滴答会被丢掉而不是堆积,所以这里不用额外防重入。
func (f *Filler) tick(ctx context.Context) {
	added := 0
	accounts := f.mgr.ListAccounts()
	for _, acc := range accounts {
		if ctx.Err() != nil {
			break
		}
		if acc.Status != "active" {
			continue
		}
		added += f.refillAccount(ctx, acc.ID)
	}
	now := time.Now()
	f.mu.Lock()
	f.lastRun = now
	f.nextRun = now.Add(f.interval)
	f.lastAdded = added
	f.totalAdded += added
	f.mu.Unlock()
}

// refillAccount 对单个账号连续补池,直到本小时配额耗尽、账号触及别名上限、
// 创建出错或 ctx 取消。返回这一轮实际补进去的个数。
func (f *Filler) refillAccount(ctx context.Context, accountID string) int {
	added := 0
	for {
		if ctx.Err() != nil {
			return added
		}

		// 上游说过"现在不行",就等到它说的时候再来
		if until := f.store.CooldownUntil(accountID); !until.IsZero() {
			if added == 0 {
				log.Printf("pool filler skip %s: 限流冷却中,%s 后恢复(%s)",
					accountID, time.Until(until).Round(time.Second), until.Format(time.RFC3339))
			}
			return added
		}

		room, err := f.headroom(accountID)
		if err != nil {
			log.Printf("pool filler %s 无法确认别名余量,本轮跳过: %v", accountID, err)
			f.noteErr(accountID, err)
			return added
		}
		if room <= 0 {
			if added == 0 {
				log.Printf("pool filler skip %s: 别名总数已达上限 %d", accountID, AliasHardCap)
			}
			return added
		}

		// 配额是追赶循环唯一的刹车。hourlyMax<=0 表示不限配额,那就没东西能
		// 让它停下来,一轮会一路打到 750。Enabled 目前挡掉了这种配置,但不该
		// 把"不会失控"寄托在别处的判断上,这里退回每轮一个。
		if f.hourlyMax <= 0 {
			if added > 0 {
				return added
			}
		} else if f.store.HourUsage(accountID) >= f.hourlyMax {
			// 先看一眼再决定要不要等,免得白等一个 spacing 才发现没配额了。
			// 真正的占用以下面的 TryConsumeQuota 为准。
			if added == 0 {
				log.Printf("pool filler skip %s: 本小时配额已用完", accountID)
			}
			return added
		}
		if added > 0 && !f.sleep(ctx, f.jitteredSpacing()) {
			return added
		}
		if !f.store.TryConsumeQuota(accountID, f.hourlyMax) {
			return added
		}
		if !f.createOne(accountID) {
			return added
		}
		added++
	}
}

// headroom 返回这个账号触及 Apple 别名上限前还能建多少个。
//
// AliasTotal 只有核对过才作数 —— 没核对时它是 0,而那个 0 的意思是"不知道"。
// 直接拿去跟上限比会得出"还能再建 750 个"这种危险结论,所以先向上游拉一次
// 把基数建起来;建不起来就宁可不补。
func (f *Filler) headroom(accountID string) (int, error) {
	acc, ok := f.mgr.GetAccount(accountID)
	if !ok {
		return 0, fmt.Errorf("账号不存在")
	}
	total := acc.AliasTotal
	if acc.AliasCountedAt == "" {
		var err error
		total, _, err = f.mgr.RefreshAliasCounts(accountID, nil)
		if err != nil {
			return 0, fmt.Errorf("别名计数从未核对过,校准失败: %w", err)
		}
	}
	return AliasHardCap - total, nil
}

// createOne 向上游建一个别名并压进池子。返回是否成功。
// 调用前必须已经占住一份配额,失败时按情况决定退不退。
func (f *Filler) createOne(accountID string) bool {
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
			// 触发限流的这次也算掉一次尝试(不 release),并且整个账号停到
			// 冷却结束 —— 否则下一轮配额还有余量,又会去撞一次。反复撞限流
			// 本身就是最该避免的信号。
			until := time.Now().Add(f.cooldown)
			f.store.SetCooldown(accountID, until)
			log.Printf("pool filler %s 触发限流,暂停到 %s: %v", accountID, until.Format(time.RFC3339), err)
		} else {
			log.Printf("pool filler %s create 失败: %v", accountID, err)
			f.store.ReleaseQuota(accountID)
		}
		return false
	}
	// 别名此刻已在 iCloud 侧真实存在,先记账再入池:哪怕下面 push 失败,
	// 它也占掉了一个名额,漏记会让上限检查以为还有余量。
	// 后续用户 Pop 这条时不再计数,否则会重复。
	f.mgr.ApplyAliasDelta(accountID, account.AliasCreated)
	entry := Entry{
		Email:       result.Email,
		AnonymousID: result.AnonymousID,
		Label:       result.Label,
		CreatedAt:   time.Now(),
	}
	if err := f.store.Push(accountID, entry); err != nil {
		log.Printf("pool filler %s push 失败,别名已建但没进池: %v", accountID, err)
		f.noteErr(accountID, err)
		return false
	}
	log.Printf("pool filler %s +1 → depth=%d", accountID, f.store.Depth(accountID))
	return true
}

// sleep 等待 d,ctx 取消时提前返回 false。
func (f *Filler) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// jitteredSpacing 给间隔加 ±25% 抖动。多个账号是在同一轮里顺序处理的,
// 固定间隔会让它们的请求排成一个规整的节拍,那本身就很像机器行为。
func (f *Filler) jitteredSpacing() time.Duration {
	if f.spacing <= 0 {
		return 0
	}
	delta := float64(f.spacing) * 0.25
	return time.Duration(float64(f.spacing) - delta + rand.Float64()*2*delta)
}
