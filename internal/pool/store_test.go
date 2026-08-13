package pool

import (
	"testing"
	"time"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPushPopDepth_FIFO(t *testing.T) {
	s := newTempStore(t)
	if s.Depth("a") != 0 {
		t.Fatal("初始应为空")
	}
	_ = s.Push("a", Entry{Email: "e1", AnonymousID: "a1", CreatedAt: time.Now()})
	_ = s.Push("a", Entry{Email: "e2", AnonymousID: "a2", CreatedAt: time.Now()})
	if s.Depth("a") != 2 {
		t.Fatalf("depth expected 2 got %d", s.Depth("a"))
	}
	got, ok := s.Pop("a")
	if !ok || got.Email != "e1" {
		t.Fatalf("FIFO 应先弹 e1,got %+v ok=%v", got, ok)
	}
	got2, _ := s.Pop("a")
	if got2.Email != "e2" {
		t.Fatalf("次弹 e2,got %s", got2.Email)
	}
	if _, ok := s.Pop("a"); ok {
		t.Fatal("池空 Pop 应返回 ok=false")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s1, _ := NewStore(dir)
	_ = s1.Push("a", Entry{Email: "e", AnonymousID: "id", CreatedAt: time.Now()})
	s1.TryConsumeQuota("a", 4)

	s2, _ := NewStore(dir)
	if s2.Depth("a") != 1 {
		t.Fatal("重新加载后 push 数据消失")
	}
	if s2.HourUsage("a") != 1 {
		t.Fatalf("重新加载后 hour usage 消失,got %d", s2.HourUsage("a"))
	}
}

func TestQuota_HourlyMax(t *testing.T) {
	s := newTempStore(t)
	for i := 0; i < 4; i++ {
		if !s.TryConsumeQuota("a", 4) {
			t.Fatalf("第 %d 次应成功", i+1)
		}
	}
	if s.TryConsumeQuota("a", 4) {
		t.Fatal("超过 hourlyMax 应返回 false")
	}
	s.ReleaseQuota("a")
	if !s.TryConsumeQuota("a", 4) {
		t.Fatal("Release 后应可以再消耗")
	}
}

func TestQuota_ZeroMeansUnlimited(t *testing.T) {
	s := newTempStore(t)
	for i := 0; i < 100; i++ {
		if !s.TryConsumeQuota("a", 0) {
			t.Fatal("hourlyMax=0 应视为无限制")
		}
	}
}

// 实时创建那条路径不能被配额挡住,但必须记进同一本账,
// 记完之后补池就该发现自己没配额了。
func TestRecordUsage_CountsWithoutBlocking(t *testing.T) {
	s := newTempStore(t)
	for i := 0; i < 10; i++ {
		s.RecordUsage("a")
	}
	if s.HourUsage("a") != 10 {
		t.Fatalf("RecordUsage 应无视上限累计,got %d", s.HourUsage("a"))
	}
	if s.TryConsumeQuota("a", 4) {
		t.Fatal("实时创建已用超配额,补池不该再拿到额度")
	}
}

func TestQuota_StaleHourResets(t *testing.T) {
	s := newTempStore(t)
	// 伪造一个上个小时的计数,它应该在读的时候就被当成 0
	s.counters["a"] = hourCounter{Hour: currentHour().Add(-time.Hour), Count: 4}

	if s.HourUsage("a") != 0 {
		t.Fatalf("跨小时后用量应清零,got %d", s.HourUsage("a"))
	}
	s.ReleaseQuota("a")
	if s.HourUsage("a") != 0 {
		t.Fatalf("回滚一个已作废的小时不该把用量变成负数或复活旧计数,got %d", s.HourUsage("a"))
	}
	if !s.TryConsumeQuota("a", 4) {
		t.Fatal("新的小时应该重新有配额")
	}
	if s.HourUsage("a") != 1 {
		t.Fatalf("新小时的第一次消费应记为 1,got %d", s.HourUsage("a"))
	}
}
